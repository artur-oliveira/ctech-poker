// Package app wires the poker API using Fx dependency injection.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"go.uber.org/fx"
	"gopkg.aoctech.app/api-commons/awsconfig"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/avatar"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/handshare"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/metrics"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/playernotes"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
	"gopkg.aoctech.app/poker/api/internal/walletclient"

	goproto "google.golang.org/protobuf/proto"
)

// Module is the root Fx module for the poker API.
var Module = fx.Options(
	fx.Provide(
		config.Load,
		newFiberApp,
		newCacheBackend,
		newVerifier,
		newWsRegistry,
		newTableLeaseService,
		newDynamoClient,
		newAvatarService,
		newTableStore,
		newRoomStore,
		newPlayerStore,
		newPlayerService,
		newPlayerNoteStore,
		newHandShareStore,
		newPokerStatsStore,
		newAchievementStore,
		newAchievementService,
		newLeaderboardStore,
		newLeaderboardService,
		newRouletteStore,
		newRouletteService,
		newSandboxPurchaseStore,
		newSandboxPurchaseService,
		newReactionEntitlementStore,
		newReactionPurchaseStore,
		newReactionPurchaseService,
		newReactionOwnershipCache,
		newSessionStore,
		walletclient.New,
		newBuyinService,
		newPendingStore,
		newTableManager,
	),
	fx.Invoke(wirePlayerRemovedHook),
	fx.Invoke(wireAutoRebuyHook),
	fx.Invoke(registerRoutes),
	fx.Invoke(startServer),
)

func newFiberApp(cfg *config.Config) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: fmt.Sprintf("CTech Poker - %s - %s", cfg.Env, cfg.AppVersion),
		// Fiber's default hands back strings that alias fasthttp's per-request
		// buffer: valid until the handler returns, garbage afterwards. This app
		// routinely outlives its own requests (hijacked WebSocket goroutines,
		// table Actors created from an HTTP route and kept in tablemanager), and
		// that aliasing already caused two prod incidents where a table ID
		// mutated into another request's bytes. One copy per accessor is
		// cheaper than auditing every handler for what escapes.
		Immutable: true,
		// All HTTP payloads are compact JSON/protobuf control messages. Avatar
		// bytes upload directly to S3 through a presigned form.
		BodyLimit:    1 << 20,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
		ProxyHeader:  fiber.HeaderXForwardedFor,
		TrustProxy:   len(cfg.TrustedProxies) > 0,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies: cfg.TrustedProxies,
		},
		ErrorHandler: func(c fiber.Ctx, err error) error {
			if p, ok := errors.AsType[*problem.Problem](err); ok {
				return p.Send(c)
			}
			if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
				return problem.FromError(fiberErr, c).Send(c)
			}
			slog.Error("unhandled HTTP error", "request_id", requestid.FromContext(c), "path", c.Path(), "err", err)
			return problem.InternalServer("an unexpected error occurred", c, err).Send(c)
		},
	})

	app.Use(recover.New())
	// AllowCredentials requires explicit origins. Development intentionally
	// leaves origins empty, which means wildcard/no credentials like Wallet.
	corsCfg := cors.Config{
		AllowMethods:  []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
		ExposeHeaders: []string{"X-Request-ID"},
		MaxAge:        3600,
	}
	if len(cfg.CorsAllowedOrigins) > 0 {
		corsCfg.AllowOrigins = cfg.CorsAllowedOrigins
		corsCfg.AllowCredentials = true
	}
	app.Use(cors.New(corsCfg))
	app.Use(requestid.New())
	app.Use(httpResponseMetrics(cfg, metrics.EmitTableMetric))
	app.Use(logger.New(logger.Config{
		Format: `{"time":"${time}","status":${status},"latency":"${latency}","method":"${method}","path":"${path}","request-id":"${request-id}"}` + "\n",
	}))
	return app
}

type metricEmitter func(env, name string, value float64, dims map[string]string)

// httpResponseMetrics records only the auth and throttling responses called
// out by the operational SLO. Route templates are used instead of raw paths,
// so room/player IDs cannot create unbounded CloudWatch dimensions.
func httpResponseMetrics(cfg *config.Config, emit metricEmitter) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()
		status := c.Response().StatusCode()
		if status != fiber.StatusUnauthorized && status != fiber.StatusTooManyRequests {
			return err
		}
		route := "unmatched"
		if current := c.Route(); current != nil && current.Path != "" {
			route = current.Path
		}
		emit(cfg.Env, "HTTPResponses", 1, map[string]string{
			"app_version": cfg.AppVersion,
			"route":       route,
			"status":      strconv.Itoa(status),
		})
		return err
	}
}

func newCacheBackend(cfg *config.Config) (cache.Backend, error) {
	if cfg.RedisURL == "" {
		if cfg.Env != "dev" {
			return nil, fmt.Errorf("redis required in non-dev env: RedisURL is empty")
		}
		slog.Warn("redis URL empty; using in-memory cache (dev only, NOT fleet-shared)")
		return cache.NewMemoryBackend(1024), nil
	}
	rb, err := cache.NewRedisBackend(cfg.RedisURL)
	if err != nil {
		if cfg.Env != "dev" {
			return nil, fmt.Errorf("redis backend unavailable in non-dev env: %w", err)
		}
		slog.Warn("redis backend unavailable, falling back to in-memory (dev only, NOT fleet-shared)", "err", err)
		return cache.NewMemoryBackend(1024), nil
	}
	return rb, nil
}

func newVerifier(c cache.Backend, cfg *config.Config) *jwtverify.Verifier {
	return jwtverify.NewVerifier(cfg.CtechJWKSURL, cfg.ServiceAudience, cfg.CtechURL, c)
}

func newWsRegistry(lc fx.Lifecycle, c cache.Backend, cfg *config.Config) (ws.Registry, error) {
	rb, ok := c.(*cache.RedisBackend)
	if !ok {
		if cfg.Env != "dev" {
			return nil, fmt.Errorf("ws registry requires a Redis backend in non-dev env")
		}
		slog.Warn("using in-memory ws registry (dev only, NOT fleet-shared)")
		return ws.NewMemoryRegistry(), nil
	}
	reg := ws.NewRedisRegistry(rb.Client())
	lc.Append(fx.Hook{OnStart: reg.Start, OnStop: reg.Stop})
	return reg, nil
}

func newTableLeaseService(c cache.Backend) *tablelease.Service {
	return tablelease.NewService(c)
}

func newDynamoClient(cfg *config.Config) (*dynamodb.Client, error) {
	awsCfg, err := awsconfig.Load(context.Background(), cfg.AWSRegion)
	if err != nil {
		return nil, err
	}
	return awsconfig.NewDynamoDBClient(awsCfg, cfg.DynamoDBEndpoint), nil
}

type avatarS3API struct {
	client  *s3.Client
	presign *s3.PresignClient
}

func (a *avatarS3API) PresignPostObject(ctx context.Context, input *s3.PutObjectInput, options ...func(*s3.PresignPostOptions)) (*s3.PresignedPostRequest, error) {
	return a.presign.PresignPostObject(ctx, input, options...)
}
func (a *avatarS3API) GetObject(ctx context.Context, input *s3.GetObjectInput, options ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return a.client.GetObject(ctx, input, options...)
}
func (a *avatarS3API) CopyObject(ctx context.Context, input *s3.CopyObjectInput, options ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	return a.client.CopyObject(ctx, input, options...)
}
func (a *avatarS3API) DeleteObject(ctx context.Context, input *s3.DeleteObjectInput, options ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return a.client.DeleteObject(ctx, input, options...)
}

func newAvatarService(cfg *config.Config) (*avatar.Service, error) {
	if cfg.AvatarBucket == "" {
		return avatar.New(nil, ""), nil
	}
	awsCfg, err := awsconfig.Load(context.Background(), cfg.AWSRegion)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		// Poker instances have IPv6 egress only. Force all server-side S3 calls,
		// including the presigned browser URL, onto the dualstack endpoint.
		options.EndpointOptions.UseDualStackEndpoint = aws.DualStackEndpointStateEnabled
	})
	return avatar.New(&avatarS3API{client: client, presign: s3.NewPresignClient(client)}, cfg.AvatarBucket), nil
}

func newTableStore(db *dynamodb.Client, cfg *config.Config) *tablestore.Store {
	return tablestore.NewStore(db, cfg.Env)
}
func newRoomStore(db *dynamodb.Client, cfg *config.Config) *roomstore.Store {
	return roomstore.NewStore(db, cfg.Env)
}
func newPlayerStore(db *dynamodb.Client, cfg *config.Config) *player.Store {
	return player.NewStore(db, cfg.Env)
}
func newPlayerService(store *player.Store, wallet *walletclient.Client) *player.Service {
	return player.NewService(store).WithWallet(wallet)
}
func newPlayerNoteStore(db *dynamodb.Client, cfg *config.Config) *playernotes.Store {
	return playernotes.NewStore(db, cfg.Env)
}
func newHandShareStore(db *dynamodb.Client, cfg *config.Config) *handshare.Store {
	return handshare.NewStore(db, cfg.Env)
}
func newPokerStatsStore(db *dynamodb.Client, cfg *config.Config) *pokerstats.Store {
	return pokerstats.NewStore(db, cfg.Env)
}
func newAchievementStore(db *dynamodb.Client, cfg *config.Config) *achievements.Store {
	return achievements.NewStore(db, cfg.Env)
}
func newAchievementService(store *achievements.Store) *achievements.Service {
	return achievements.NewService(store)
}
func newLeaderboardStore(db *dynamodb.Client, cfg *config.Config) *leaderboard.Store {
	return leaderboard.NewStore(db, cfg.Env)
}
func newLeaderboardService(store *leaderboard.Store) *leaderboard.Service {
	return leaderboard.NewServiceWithStore(store)
}
func newRouletteStore(db *dynamodb.Client, cfg *config.Config) *dailyreward.Store {
	return dailyreward.NewStore(db, cfg.Env)
}
func newRouletteService(wallet *walletclient.Client, store *dailyreward.Store) *dailyreward.Service {
	return dailyreward.NewService(wallet, store)
}
func newSandboxPurchaseStore(db *dynamodb.Client, cfg *config.Config) *sandboxpurchase.Store {
	return sandboxpurchase.NewStore(db, cfg.Env)
}
func newSandboxPurchaseService(wallet *walletclient.Client, store *sandboxpurchase.Store) *sandboxpurchase.Service {
	return sandboxpurchase.NewService(wallet, store)
}
func newReactionEntitlementStore(db *dynamodb.Client, cfg *config.Config) *reactionpurchase.EntitlementStore {
	return reactionpurchase.NewEntitlementStore(db, cfg.Env)
}
func newReactionPurchaseStore(db *dynamodb.Client, cfg *config.Config) *reactionpurchase.Store {
	return reactionpurchase.NewStore(db, cfg.Env)
}
func newReactionPurchaseService(wallet *walletclient.Client, entitlements *reactionpurchase.EntitlementStore, store *reactionpurchase.Store) *reactionpurchase.Service {
	return reactionpurchase.NewService(wallet, entitlements, store)
}
func newReactionOwnershipCache(svc *reactionpurchase.Service, cacheB cache.Backend) *reactionpurchase.OwnershipCache {
	return reactionpurchase.NewOwnershipCache(svc, cacheB)
}
func newSessionStore(db *dynamodb.Client, cfg *config.Config) *sessionlog.Store {
	return sessionlog.NewStore(db, cfg.Env)
}
func newPendingStore(db *dynamodb.Client, cfg *config.Config) *reconcile.PendingStore {
	return reconcile.NewPendingStore(db, cfg.Env)
}
func newBuyinService(cfg *config.Config, wallet *walletclient.Client, manager *tablemanager.Manager, rooms *roomstore.Store, players *player.Service, sessionStore *sessionlog.Store, pending *reconcile.PendingStore, pokerStatsStore *pokerstats.Store) *buyin.Service {
	if cfg.RealMoneyEnabled {
		return buyin.NewServiceWithGame(wallet, wallet, manager, rooms, wallet).WithPendingStore(pending).WithSessionStore(sessionStore).WithPlayers(players).WithAvatarBaseURL(cfg.AvatarBaseURL).WithPokerStats(pokerStatsStore)
	}
	return buyin.NewServiceWithPlayers(wallet, manager, rooms, players).WithPendingStore(pending).WithSessionStore(sessionStore).WithAvatarBaseURL(cfg.AvatarBaseURL).WithPokerStats(pokerStatsStore)
}

type roomModeReader interface {
	Get(context.Context, string) (*roomstore.Room, error)
}

func tableCurrencyMode(ctx context.Context, rooms roomModeReader, tableID string) (string, error) {
	room, err := rooms.Get(ctx, tableID)
	if err != nil {
		return "", err
	}
	if room == nil {
		return "", fmt.Errorf("room %s not found", tableID)
	}
	return room.CurrencyMode, nil
}

func newTableManager(leases *tablelease.Service, store *tablestore.Store, reg ws.Registry, achv *achievements.Service, leaderboardSvc *leaderboard.Service, rooms *roomstore.Store, sessionStore *sessionlog.Store, pokerStatsStore *pokerstats.Store, players *player.Service, cfg *config.Config) *tablemanager.Manager {
	broadcast := func(tableID, viewerID string, snap hand.Snapshot) {
		message := &pokerproto.ServerMessage{Type: "state", Snapshot: v1.ConvertSnapshot(snap)}
		data, err := goproto.Marshal(message)
		if err == nil {
			reg.Broadcast(context.Background(), tableID+"#"+viewerID, data)
		}
	}
	persistHandHistory := func(tableID, handID, mode string, outcome hand.HandOutcome, names map[string]string) {
		if sessionStore == nil {
			return
		}
		avatarURLs := make(map[string]string, len(outcome.Participants))
		for _, id := range outcome.Participants {
			if profile, err := players.GetOrCreate(context.Background(), id); err == nil {
				avatarURLs[id] = player.AvatarURL(profile, cfg.AvatarBaseURL)
			}
		}
		for _, id := range outcome.Participants {
			item := handItemForWithAvatars(outcome, id, names, avatarURLs)
			item.PK, item.TableID, item.HandID, item.CurrencyMode, item.EndedAt = id, tableID, handID, mode, time.Now().UnixMilli()
			if err := sessionStore.RecordHand(context.Background(), item); err != nil {
				slog.Error("sessionlog: record hand failed", "table", tableID, "hand", handID, "player", id, "err", err)
			}
		}
	}
	onHandComplete := func(tableID, handID string, outcome hand.HandOutcome, names map[string]string) {
		ctx := context.Background()
		mode, err := tableCurrencyMode(ctx, rooms, tableID)
		if err != nil {
			slog.Error("gamification: load room mode failed", "table", tableID, "err", err)
			return
		}
		var metrics []pokerstats.HandMetric
		actions, metricsErr := store.LoadActionsSince(ctx, tableID, handID, 0)
		if metricsErr != nil {
			slog.Error("pokerstats: load hand actions failed", "table", tableID, "hand", handID, "err", metricsErr)
		} else {
			metrics = pokerstats.Analyze(outcome.Participants, actions)
		}
		achievementMetrics := make([]achievements.HandMetric, len(metrics))
		for i, metric := range metrics {
			achievementMetrics[i] = achievements.HandMetric{
				PlayerID: metric.PlayerID,
				VPIP:     metric.VPIP,
				ThreeBet: metric.ThreeBet,
			}
		}
		unlocks, err := achv.RecordHand(ctx, tableID, mode, outcome, achievementMetrics)
		if err != nil {
			slog.Error("achievements record hand failed", "table", tableID, "err", err)
		}
		for _, unlock := range unlocks {
			data, err := goproto.Marshal(&pokerproto.ServerMessage{
				Type:  "achievement_unlocked",
				Key:   unlock.Key,
				Stars: int32(unlock.Stars),
			})
			if err == nil {
				reg.Broadcast(ctx, tableID+"#"+unlock.PlayerID, data)
			}
		}
		if err := leaderboardSvc.RecordUnlocks(ctx, mode, unlocks); err != nil {
			slog.Error("leaderboard achievement points failed", "table", tableID, "err", err)
		}
		if err := leaderboardSvc.RecordHand(ctx, mode, outcome, names); err != nil {
			slog.Error("leaderboard record hand failed", "table", tableID, "err", err)
		}
		if metricsErr == nil {
			if err := pokerStatsStore.RecordHand(ctx, mode, tableID, handID, metrics); err != nil {
				slog.Error("pokerstats: record hand failed", "table", tableID, "hand", handID, "err", err)
			}
		}
		persistHandHistory(tableID, handID, mode, outcome, names)
	}
	// roomLoader re-arms blind escalation and the per-turn action timeout from
	// the room's authoritative config on every actor creation (T6), so both
	// survive instance/lease moves.
	roomLoader := func(tableID string) (*roomstore.Room, bool, error) {
		r, err := rooms.Get(context.Background(), tableID)
		if err != nil {
			return nil, false, err
		}
		if r == nil {
			return nil, false, nil
		}
		return r, true, nil
	}
	mgr := tablemanager.NewManager(leases, store, broadcast, roomLoader, onHandComplete)
	mgr.SetOnHandUpdated(func(tableID, handID string, outcome hand.HandOutcome, names map[string]string) {
		mode, err := tableCurrencyMode(context.Background(), rooms, tableID)
		if err != nil {
			slog.Error("sessionlog: load room mode for hand update failed", "table", tableID, "hand", handID, "err", err)
			return
		}
		persistHandHistory(tableID, handID, mode, outcome, names)
	})
	mgr.SetOnSeatsChanged(func(tableID string, seatsTaken int) {
		if err := rooms.SetSeatsTaken(context.Background(), tableID, seatsTaken); err != nil {
			slog.Error("roomstore: seats taken write-through failed", "table", tableID, "err", err)
		}
		data, _ := goproto.Marshal(&pokerproto.ServerMessage{
			Type:       "room_updated",
			RoomId:     tableID,
			SeatsTaken: int32(seatsTaken),
		})
		reg.Broadcast(context.Background(), "lobby", data)
	})
	return mgr
}

// handItemFor builds one participant's poker_player_hand record from a
// completed hand.HandOutcome — pure so the outcome-to-history mapping (own
// result, opponent hole cards, opponent Won) is unit-testable without a live
// table/actor. Caller fills in PK/TableID/HandID/EndedAt.
func handItemFor(outcome hand.HandOutcome, id string, names map[string]string) sessionlog.HandItem {
	return handItemForWithAvatars(outcome, id, names, nil)
}

func handItemForWithAvatars(outcome hand.HandOutcome, id string, names, avatarURLs map[string]string) sessionlog.HandItem {
	net := outcome.Payouts[id] - outcome.Contributions[id]
	var result = "lost"
	if slices.Contains(outcome.Winners, id) {
		result = "won"
	}
	if sr, ok := outcome.ShowdownResults[id]; ok && sr.Tied {
		result = "tied"
	}
	var holeCards []string
	if own, ok := outcome.PlayerHands[id]; ok {
		holeCards = own.HoleCards[:]
	}
	var opponents []sessionlog.OpponentSummary
	for _, opp := range outcome.Participants {
		if opp == id {
			continue
		}
		info, ok := outcome.PlayerHands[opp]
		if !ok {
			continue
		}
		summary := sessionlog.OpponentSummary{PlayerID: opp, Name: names[opp], AvatarURL: avatarURLs[opp]}
		if info.Revealed {
			summary.HoleCards = info.HoleCards[:]
		} else if info.RevealedCards[0] || info.RevealedCards[1] {
			summary.HoleCards = []string{"back", "back"}
			for i, revealed := range info.RevealedCards {
				if revealed {
					summary.HoleCards[i] = info.HoleCards[i]
				}
			}
		}
		for _, w := range outcome.Winners {
			if w == opp {
				summary.Won = true
				break
			}
		}
		opponents = append(opponents, summary)
	}
	item := sessionlog.HandItem{
		Outcome: result, NetChange: net,
		Board: outcome.Board, BoardTwo: outcome.BoardTwo, HoleCards: holeCards, Opponents: opponents,
		CommitHash:     outcome.CommitHash,
		RootCommitHash: outcome.RootCommitHash,
		ServerSeed: func() string {
			for _, info := range outcome.PlayerHands {
				if !info.Revealed {
					return ""
				}
			}
			return outcome.ServerSeed
		}(),
	}
	// Without the seed, this per-position proof is the player's only way to
	// verify the deck from their history — see sessionlog.HandItem's fields.
	if proof, ok := outcome.FairnessProofs[id]; ok {
		item.RevealedCardSalts = make(map[string]sessionlog.RevealedSalt, len(proof.RevealedCardSalts))
		for index, reveal := range proof.RevealedCardSalts {
			item.RevealedCardSalts[strconv.Itoa(index)] = sessionlog.RevealedSalt{Card: reveal.Card, SaltHex: reveal.SaltHex}
		}
		item.UnrevealedCardHashes = make(map[string]string, len(proof.UnrevealedCardHashes))
		for index, hash := range proof.UnrevealedCardHashes {
			item.UnrevealedCardHashes[strconv.Itoa(index)] = hash
		}
	}
	return item
}

// autoRebuyRoomLookup and autoRebuyBuyinService narrow *roomstore.Store and
// *buyin.Service down to exactly what autoRebuySweep needs, so it's testable
// with plain fakes instead of the DynamoDB-backed integration harness
// buyin's own tests require.
type autoRebuyRoomLookup interface {
	Get(ctx context.Context, roomID string) (*roomstore.Room, error)
}

type autoRebuyBuyinService interface {
	SeatedSummary(ctx context.Context, roomID, playerID string) (buyin.SeatSummary, error)
	SandboxBalance(ctx context.Context, playerID string) (int64, error)
	BuyIn(ctx context.Context, roomID, playerID string, amount int64, midHand bool, idemKey string) error
}

// autoRebuySweep checks every one of a just-completed hand's participants
// and auto-rebuys anyone who busted (Stack==0) with auto-rebuy on and enough
// sandbox balance to cover their original buy-in. Sandbox rooms only — see
// docs/specs/2026-08-10-auto-buyin-design.md's Scope section for why
// real-money is excluded. Errors are logged and skipped per player, never
// retried: a skipped player just stays sitting out, same as insufficient
// balance.
func autoRebuySweep(ctx context.Context, buyinSvc autoRebuyBuyinService, rooms autoRebuyRoomLookup, tableID, handID string, outcome hand.HandOutcome) {
	room, err := rooms.Get(ctx, tableID)
	if err != nil {
		slog.Error("auto-rebuy: load room failed", "table", tableID, "err", err)
		return
	}
	if room == nil || room.CurrencyMode != roomstore.CurrencyModeSandbox {
		return
	}
	for _, playerID := range outcome.Participants {
		seat, err := buyinSvc.SeatedSummary(ctx, tableID, playerID)
		if err != nil {
			slog.Error("auto-rebuy: seat lookup failed", "table", tableID, "player", playerID, "err", err)
			continue
		}
		if !seat.Seated || seat.Stack != 0 || !seat.AutoRebuy || seat.BuyInAmount <= 0 {
			continue
		}
		balance, err := buyinSvc.SandboxBalance(ctx, playerID)
		if err != nil {
			slog.Error("auto-rebuy: balance check failed", "table", tableID, "player", playerID, "err", err)
			continue
		}
		if balance < seat.BuyInAmount {
			continue
		}
		// playerID is not repeated here — buyIn's key already prepends it
		// (roomID#playerID#buyin#nonce). Doing so anyway pushed the compound
		// idempotency key over ctech-wallet's MovementOpRequest.IdempotencyKey
		// max=128, so every sandbox debit came back 422 and no auto-rebuy ever
		// succeeded in production (root-caused 2026-08-11 from player.har +
		// prod logs, never caught by unit tests since those mock the wallet
		// client instead of enforcing its field-length validation).
		nonce := handID + "-auto"
		if err := buyinSvc.BuyIn(ctx, tableID, playerID, seat.BuyInAmount, false, nonce); err != nil {
			slog.Error("auto-rebuy: buy-in failed", "table", tableID, "player", playerID, "err", err)
		}
	}
}

// wireAutoRebuyHook installs the post-hand auto-rebuy sweep. Same
// construction-cycle reason as wirePlayerRemovedHook below: buyin.Service
// depends on *tablemanager.Manager, so this can only be wired after Fx
// builds both.
//
// autoRebuySweep is dispatched in a detached goroutine, never called inline:
// this callback fires synchronously from inside the table actor's own
// single-goroutine command loop (table/actor.go's notifyHandComplete, called
// from broadcastAll before the actor's Run loop reads its next command), and
// both SeatedSummary and BuyIn dispatch back into that same loop. Calling
// either synchronously here would deadlock the whole table.
func wireAutoRebuyHook(mgr *tablemanager.Manager, buyinSvc *buyin.Service, rooms *roomstore.Store) {
	mgr.SetOnAutoRebuySweep(func(tableID, handID string, outcome hand.HandOutcome) {
		go autoRebuySweep(context.Background(), buyinSvc, rooms, tableID, handID, outcome)
	})
}

// wirePlayerRemovedHook installs table.Actor's system-removal notification —
// AFK sweep / disconnect kick timeout only, see onPlayerRemoved's doc comment
// in actor.go. Split out of newTableManager (rather than set there like
// onHandComplete/onSeatsChanged) because buyin.Service itself depends on
// *tablemanager.Manager — wiring the hook here, after Fx has built both, is
// the only way to avoid a construction cycle. SetOnPlayerRemoved is safe to
// call after actors already exist (tablemanager.Manager checks the hook
// dynamically on every fire, not at actor-creation time).
func wirePlayerRemovedHook(mgr *tablemanager.Manager, buyinSvc *buyin.Service, reg ws.Registry, reactionOwnershipCache *reactionpurchase.OwnershipCache, reactionSvc *reactionpurchase.Service) {
	mgr.SetSystemSettlementIntent(buyinSvc.BuildSystemSettlementIntent)
	mgr.SetReactionOwnership(reactionOwnershipCache.IsOwned)
	mgr.SetReactionMarkUsed(reactionSvc.MarkUsed)
	mgr.SetOnPlayerRemoved(func(tableID, playerID, reason string, stack int64, holdID string) {
		ctx := context.Background()
		// Pushes an explicit "removed" frame straight to the removed player's
		// own connection (same per-player fan-out key the "state" broadcast
		// uses) — without it, a system-removed player's socket just stops
		// receiving state broadcasts (they're no longer in PlayersForActor)
		// with no signal telling the client why, so it sits on a stale table
		// instead of redirecting to the lobby.
		data, _ := goproto.Marshal(&pokerproto.ServerMessage{
			Type: "removed",
			Code: reason,
		})
		reg.Broadcast(ctx, tableID+"#"+playerID, data)
		if err := buyinSvc.SettleSystemRemoval(ctx, tableID, playerID, stack, holdID, reason); err != nil {
			slog.Error("buyin: settle system removal failed", "table", tableID, "player", playerID, "reason", reason, "err", err)
		}
	})
}

func roomBackedSeed(rooms *roomstore.Store) func(string) func() *hand.Table {
	return func(tableID string) func() *hand.Table {
		return func() *hand.Table {
			if rooms == nil {
				return hand.NewTable(nil, 10, 20)
			}
			room, err := rooms.Get(context.Background(), tableID)
			if err != nil || room == nil {
				return hand.NewTable(nil, 10, 20)
			}
			return table.SeedForRoom(room)
		}
	}
}

func registerRoutes(
	app *fiber.App,
	cfg *config.Config,
	db *dynamodb.Client,
	verifier *jwtverify.Verifier,
	manager *tablemanager.Manager,
	reg ws.Registry,
	cacheBackend cache.Backend,
	rooms *roomstore.Store,
	buyinSvc *buyin.Service,
	players *player.Service,
	leaderboardSvc *leaderboard.Service,
	dailyRewardSvc *dailyreward.Service,
	tableStore *tablestore.Store,
	sessionStore *sessionlog.Store,
	achievementStore *achievements.Store,
	playerNoteStore *playernotes.Store,
	handShareStore *handshare.Store,
	pokerStatsStore *pokerstats.Store,
	avatars *avatar.Service,
	sandboxPurchaseSvc *sandboxpurchase.Service,
	reactionPurchaseSvc *reactionpurchase.Service,
) {
	v1.Register(app, cfg, db, verifier, manager, reg, roomBackedSeed(rooms), cacheBackend, rooms, buyinSvc, players, leaderboardSvc, dailyRewardSvc, tableStore, sessionStore, achievementStore, playerNoteStore, handShareStore, pokerStatsStore, avatars, sandboxPurchaseSvc, reactionPurchaseSvc)
}

func startServer(lc fx.Lifecycle, app *fiber.App, cfg *config.Config, manager *tablemanager.Manager) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			addr := ":" + strconv.Itoa(cfg.Port)
			slog.Info("starting ctech-poker-api", "addr", addr, "env", cfg.Env)
			go func() {
				if err := app.Listen(addr); err != nil {
					slog.Error("server stopped", "err", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			slog.Info("shutting down ctech-poker-api, draining table manager leases")
			manager.DrainAndRelease(ctx)
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return app.ShutdownWithContext(stopCtx)
		},
	})
}
