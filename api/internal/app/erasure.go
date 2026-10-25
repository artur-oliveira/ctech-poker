package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/fx"
	"gopkg.aoctech.app/api-commons/awsconfig"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/dataexport"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/oauth2client"
	"gopkg.aoctech.app/api-commons/resilient"
	"gopkg.aoctech.app/api-commons/supervisor"
	"gopkg.aoctech.app/api-commons/ws"
	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/opsalert"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/responsible"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/userexport"
	"gopkg.aoctech.app/poker/api/internal/userpurge"
)

// Account-deletion saga participant wiring
// (docs/plans/2026-10-07-account-deletion-participant.md).

const (
	// erasureService is poker's participant id in erasure messages and the SNS filter.
	erasureService = "poker"
	// erasureAckScope is the only scope of poker's dedicated ack client
	// (ERASURE_ACK_CLIENT_ID), granted by an operator in ctech-account.
	erasureAckScope = "internal:account:erasure-ack"
	// exportAckScope is the extra scope the same client needs for the personal-data
	// export (granted by an operator; until then the account never asks poker).
	exportAckScope = "internal:account:export-ack"
)

// newErasureStore binds {env}_poker_erasure_state. The poker_ segment keeps it
// from colliding with another participant's state table in the same AWS
// account (plan ruling R1). Tombstones never expire (R10).
func newErasureStore(db *dynamodb.Client, cfg *config.Config) *erasure.Store {
	return erasure.NewStore(db, cfg.Env+"_poker", 0)
}

func newEligibility(sessions *sessionlog.Store, pending *reconcile.PendingStore) *userpurge.Eligibility {
	return userpurge.NewEligibility(sessions, pending)
}

func registerErasureRoutes(app *fiber.App, cfg *config.Config, verifier *jwtverify.Verifier, elig *userpurge.Eligibility) {
	v1.RegisterErasure(app.Group("/v1.0"), verifier, cfg.ErasureAccountClientID, elig.Blockers)
}

// startErasureConsumer runs the saga consumer in the background: it applies
// user.locked/user.unlocked itself and calls Purger.Purge on user.erase, then
// acks to ctech-account. Every replica runs one; SQS hands each message to
// one of them, and the purge is idempotent.
func startErasureConsumer(lc fx.Lifecycle, sup *supervisor.Supervisor, cfg *config.Config, db *dynamodb.Client, c cache.Backend, store *erasure.Store, elig *userpurge.Eligibility, reg ws.Registry, rules *responsible.Service) error {
	if cfg.ErasureQueueURL == "" {
		slog.Warn("ERASURE_QUEUE_URL empty: account-deletion consumer disabled (dev only; config.Load refuses this in prod)")
		return nil
	}
	awsCfg, err := awsconfig.Load(context.Background(), cfg.AWSRegion)
	if err != nil {
		return err
	}
	// Poker instances have IPv6 egress only (see newAvatarService).
	objects := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.EndpointOptions.UseDualStackEndpoint = aws.DualStackEndpointStateEnabled
	})
	queue := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		o.EndpointOptions.UseDualStackEndpoint = aws.DualStackEndpointStateEnabled
	})
	purger := userpurge.NewPurger(db, objects, c, userpurge.Config{
		Env: cfg.Env, AvatarBucket: cfg.AvatarBucket, ArchiveBucket: cfg.ActionLogArchiveBucket,
	}, elig.Blockers).WithCarrier(rules.CarryOnErase)
	// The account: acks and tokens. A circuit per host, so a down account is refused at once
	// and the saga step waits in the queue instead of holding a goroutine.
	httpClient := resilient.Client(10*time.Second, resilient.Config{})
	account := strings.TrimRight(cfg.CtechURL, "/")
	tokens := oauth2client.New(httpClient, c, account+"/v1.0/token", cfg.ErasureAckClientID, cfg.ErasureAckClientSecret, erasureAckScope)
	acks := erasure.NewAckClient(httpClient, account+"/v1.0/internal/erasure/ack", tokens)
	// OnLock fires only when a message really moves the sub to locked (not on
	// redeliveries); it closes the user's sockets on every instance (R6).
	// The personal-data export rides the same queue: account sends user.export with
	// a presigned upload URL and poker answers with its part (spec 2026-10-17-data-export).
	exportTokens := oauth2client.New(httpClient, c, account+"/v1.0/token", cfg.ErasureAckClientID, cfg.ErasureAckClientSecret, exportAckScope)
	// The part is data (a hand history can reach megabytes) PUT to S3 over this
	// client, so it gets a longer timeout than the saga's 10 s calls.
	uploads := &http.Client{Timeout: 60 * time.Second}
	exporter := dataexport.NewStreamHandler(erasureService, userexport.New(db, cfg.Env).Write, uploads,
		dataexport.NewAckClient(httpClient, account+"/v1.0/internal/export/ack", exportTokens))
	consumer := erasure.NewConsumer(queue, cfg.ErasureQueueURL, erasureService, store, opsalert.AlertingPurge(purger.Purge, opsalert.New(context.Background(), cfg.AWSRegion, cfg.AlertsTopicARN, cfg.Env)), acks).
		OnLock(v1.CloseLockedSockets(reg)).
		WithExporter(exporter.Handle).
		WithVisibilityHeartbeat(cfg.ErasureQueueVisibility)

	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			// A crash or error restarts the consumer with backoff; a nil return
			// (ctx cancelled) ends it.
			return sup.Run(ctx, "erasure-consumer", func(ctx context.Context) error {
				return consumer.Run(ctx)
			})
		},
		// A purge cut short here is redelivered after the visibility timeout
		// and resumes: every step re-reads from scratch. The supervisor's own
		// OnStop (registered first, so it runs last) waits for the loop to exit.
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
	return nil
}
