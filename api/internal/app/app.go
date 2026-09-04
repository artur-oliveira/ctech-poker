// Package app wires the poker API using Fx dependency injection.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	fiberrecover "github.com/gofiber/fiber/v3/middleware/recover"
	"go.uber.org/fx"
	"gopkg.aoctech.app/api-commons/awsconfig"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/jwtverify"
	fiberobs "gopkg.aoctech.app/api-commons/observability/fiber"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/avatar"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/entitlement"
	"gopkg.aoctech.app/poker/api/internal/handhook"
	"gopkg.aoctech.app/poker/api/internal/handreveal"
	"gopkg.aoctech.app/poker/api/internal/handshare"
	"gopkg.aoctech.app/poker/api/internal/highlights"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/matchup"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/playernotes"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/presence"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/recentplayers"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/reports"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/social"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tableconn"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
	"gopkg.aoctech.app/poker/api/internal/tablestreak"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
	"gopkg.aoctech.app/poker/api/internal/wsdrain"

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
		newCosmeticsEntitlementStore,
		newCosmeticsPurchaseStore,
		newCosmeticsPurchaseService,
		newPlayerService,
		newPlayerNoteStore,
		newHandShareStore,
		newHandRevealStore,
		newHandRevealPaymentStore,
		newHandRevealService,
		newPokerStatsStore,
		newMatchupStore,
		newHighlightsStore,
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
		newSocialStore,
		newSocialEventStore,
		newSocialService,
		newPresenceStore,
		newPresenceService,
		newRecentPlayersStore,
		newRecentPlayersService,
		newReportStore,
		newReportService,
		walletclient.New,
		newBuyinService,
		newPendingStore,
		newEntitlementStore,
		newTableManager,
	),
	fx.Invoke(wirePlayerRemovedHook),
	fx.Invoke(wireAutoRebuyHook),
	fx.Invoke(wireCosmeticCurrentSelection),
	fx.Invoke(validateWalletScopes),
	fx.Invoke(registerRoutesWithSocialRuntime),
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
			return problem.InternalServer("an unexpected error occurred", c, err).Send(c)
		},
	})

	app.Use(fiberobs.RequestID())
	app.Use(fiberrecover.New(fiberrecover.Config{EnableStackTrace: true}))
	// AllowCredentials requires explicit origins. Development intentionally
	// leaves origins empty, which means wildcard/no credentials like Wallet.
	corsCfg := cors.Config{
		AllowMethods:  []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Authorization", "X-Request-ID", "Idempotency-Key"},
		ExposeHeaders: []string{"X-Request-ID"},
		MaxAge:        3600,
	}
	if len(cfg.CorsAllowedOrigins) > 0 {
		corsCfg.AllowOrigins = cfg.CorsAllowedOrigins
		corsCfg.AllowCredentials = true
	}
	app.Use(cors.New(corsCfg))
	app.Use(logger.New(logger.Config{
		Format: `{"time":"${time}","status":${status},"latency":"${latency}","method":"${method}",` +
			`"path":"${path}","request_id":"${respHeader:X-Request-Id}"}` + "\n",
		DisableColors: true,
		Skip: func(c fiber.Ctx) bool {
			return c.Path() == "/v1.0/health" || c.Path() == "/v1.0/health-check"
		},
	}))
	return app
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
	return jwtverify.NewVerifier(cfg.CtechJWKSURL, cfg.ServiceAudience, cfg.CtechIssuerURL, c)
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
func newPlayerService(store *player.Store, wallet *walletclient.Client, cosmeticsSvc *cosmeticpurchase.Service) *player.Service {
	return player.NewService(store).WithWallet(wallet).WithCosmetics(cosmeticsSvc)
}
func newPlayerNoteStore(db *dynamodb.Client, cfg *config.Config) *playernotes.Store {
	return playernotes.NewStore(db, cfg.Env)
}
func newHandShareStore(db *dynamodb.Client, cfg *config.Config) *handshare.Store {
	return handshare.NewStore(db, cfg.Env)
}
func newHandRevealStore(db *dynamodb.Client, cfg *config.Config) *handreveal.Store {
	return handreveal.NewStore(db, cfg.Env)
}
func newHandRevealPaymentStore(db *dynamodb.Client, cfg *config.Config) *handreveal.PaymentStore {
	return handreveal.NewPaymentStore(db, cfg.Env)
}
func newHandRevealService(wallet *walletclient.Client, payments *handreveal.PaymentStore) *handreveal.Service {
	return handreveal.NewService(wallet, payments)
}
func newPokerStatsStore(db *dynamodb.Client, cfg *config.Config) *pokerstats.Store {
	return pokerstats.NewStore(db, cfg.Env)
}
func newMatchupStore(db *dynamodb.Client, cfg *config.Config) *matchup.Store {
	return matchup.NewStore(db, cfg.Env)
}
func newHighlightsStore(db *dynamodb.Client, cfg *config.Config) *highlights.Store {
	return highlights.NewStore(db, cfg.Env)
}
func newAchievementStore(db *dynamodb.Client, cfg *config.Config) *achievements.Store {
	return achievements.NewStore(db, cfg.Env)
}
func newAchievementService(store *achievements.Store, cacheBackend cache.Backend) *achievements.Service {
	svc := achievements.NewService(store)
	// The "same pocket pair" streak remembers which pair a player last won
	// with. That memory has to be shared: any instance can serve the hand
	// that completes (see Service.cache).
	svc.SetCache(cacheBackend)
	return svc
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
func newCosmeticsEntitlementStore(db *dynamodb.Client, cfg *config.Config) *cosmeticpurchase.EntitlementStore {
	return cosmeticpurchase.NewEntitlementStore(db, cfg.Env)
}
func newCosmeticsPurchaseStore(db *dynamodb.Client, cfg *config.Config) *cosmeticpurchase.Store {
	return cosmeticpurchase.NewStore(db, cfg.Env)
}
func newCosmeticsPurchaseService(wallet *walletclient.Client, entitlements *cosmeticpurchase.EntitlementStore, store *cosmeticpurchase.Store) *cosmeticpurchase.Service {
	return cosmeticpurchase.NewService(wallet, entitlements, store)
}

// wireCosmeticCurrentSelection wires the "is this item currently applied"
// check Service.Refund needs into the cosmetics purchase service, after Fx
// has built both — cosmeticpurchase can't import player directly (player
// already depends on cosmeticpurchase for ownership checks), so the callback
// is injected the same way SetOwnershipInvalidator injects one.
func wireCosmeticCurrentSelection(cosmeticsSvc *cosmeticpurchase.Service, players *player.Service) {
	cosmeticsSvc.SetCurrentSelectionFunc(func(ctx context.Context, playerID string, kind cosmetics.Kind) (string, error) {
		profile, err := players.Get(ctx, playerID)
		if err != nil {
			return "", err
		}
		switch kind {
		case cosmetics.KindDeck:
			return profile.EffectiveDeckVariant(), nil
		case cosmetics.KindFelt:
			return profile.EffectiveTableTheme(), nil
		default:
			return "", nil
		}
	})
}
func newSessionStore(db *dynamodb.Client, cfg *config.Config) *sessionlog.Store {
	return sessionlog.NewStore(db, cfg.Env)
}
func newSocialStore(db *dynamodb.Client, cfg *config.Config) *social.Store {
	return social.NewStore(db, cfg.Env)
}
func newSocialEventStore(db *dynamodb.Client, cfg *config.Config) *social.DynamoEventStore {
	return social.NewEventStore(db, cfg.Env)
}
func newSocialService(store *social.Store, events *social.DynamoEventStore, rooms *roomstore.Store, sessions *sessionlog.Store, reg ws.Registry, cfg *config.Config) *social.Service {
	return social.NewService(store, cfg.SocialGraphEnabled).WithInbox(events).WithInvites(rooms, sessions).WithNotifier(newSocialNotifier(reg))
}

func newSocialNotifier(reg ws.Registry) social.NotifyFunc {
	return func(ctx context.Context, event social.Event, unread int) {
		messageType := "social_inbox_count"
		var eventPayload *pokerproto.SocialEvent
		if event.EventID != "" {
			messageType = "social_event"
			eventPayload = &pokerproto.SocialEvent{
				EventId: event.EventID, Type: string(event.Type), ActorId: event.ActorPlayerID,
				RoomId: event.RoomID, Status: string(event.Status), CreatedAt: event.CreatedAt, ExpiresAt: event.ExpiresAt,
			}
		}
		data, err := goproto.Marshal(&pokerproto.ServerMessage{Type: messageType, SocialEvent: eventPayload, UnreadCount: int32(unread)})
		if err == nil {
			reg.Broadcast(ctx, "user#"+event.RecipientPlayerID, data)
		}
	}
}
func newPresenceStore(cacheBackend cache.Backend, cfg *config.Config) presence.Store {
	if redis, ok := cacheBackend.(*cache.RedisBackend); ok {
		return presence.NewValkeyStore(redis.Client())
	}
	if cfg.Env != "dev" {
		panic("presence requires Valkey outside development")
	}
	return presence.NewMemoryStore()
}
func newPresenceService(store presence.Store, socialSvc *social.Service, sessions *sessionlog.Store, reg ws.Registry) *presence.Service {
	return presence.NewService(store, socialSvc, sessions, newPresenceNotifier(reg))
}
func newPresenceNotifier(reg ws.Registry) presence.NotifyFunc {
	return func(ctx context.Context, recipientID, playerID string, status presence.Status) {
		data, err := goproto.Marshal(&pokerproto.ServerMessage{Type: "social_presence_changed", SocialEvent: &pokerproto.SocialEvent{
			Type: "presence_changed", ActorId: playerID, Presence: &pokerproto.PlayerPresence{PlayerId: playerID, Status: string(status)},
		}})
		if err == nil {
			reg.Broadcast(ctx, "user#"+recipientID, data)
		}
	}
}
func newRecentPlayersStore(db *dynamodb.Client, cfg *config.Config) *recentplayers.DynamoStore {
	return recentplayers.NewStore(db, cfg.Env)
}
func newRecentPlayersService(store *recentplayers.DynamoStore, sessions *sessionlog.Store, socialSvc *social.Service) *recentplayers.Service {
	return recentplayers.NewService(store, sessions, socialSvc)
}
func newReportStore(db *dynamodb.Client, cfg *config.Config) *reports.DynamoStore {
	return reports.NewStore(db, cfg.Env)
}
func newReportService(store *reports.DynamoStore, tableStore *tablestore.Store, players *player.Service, cfg *config.Config) *reports.Service {
	return reports.NewService(store, tableStore, players)
}
func newPendingStore(db *dynamodb.Client, cfg *config.Config) *reconcile.PendingStore {
	return reconcile.NewPendingStore(db, cfg.Env)
}
func newEntitlementStore(db *dynamodb.Client, cfg *config.Config) *entitlement.Store {
	return entitlement.NewStore(db, cfg.Env)
}
func newBuyinService(cfg *config.Config, wallet *walletclient.Client, manager *tablemanager.Manager, rooms *roomstore.Store, players *player.Service, sessionStore *sessionlog.Store, pending *reconcile.PendingStore, entitlements *entitlement.Store, pokerStatsStore *pokerstats.Store, presenceSvc *presence.Service) *buyin.Service {
	if cfg.RealMoneyEnabled {
		return buyin.NewServiceWithGame(wallet, wallet, manager, rooms, wallet).WithPendingStore(pending).WithEntitlements(entitlements).WithSessionStore(sessionStore).WithPlayers(players).WithAvatarBaseURL(cfg.AvatarBaseURL).WithPokerStats(pokerStatsStore).WithPresence(presenceSvc)
	}
	return buyin.NewServiceWithPlayers(wallet, manager, rooms, players).WithPendingStore(pending).WithEntitlements(entitlements).WithSessionStore(sessionStore).WithAvatarBaseURL(cfg.AvatarBaseURL).WithPokerStats(pokerStatsStore).WithPresence(presenceSvc)
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

// dispatchGamificationPipeline detaches a completed hand's gamification
// bookkeeping (pipeline) onto its own goroutine so the table actor's own
// goroutine — which calls onHandComplete synchronously from
// table/actor.go's notifyHandComplete — never blocks on it (#61). A panic
// inside pipeline is recovered and logged rather than crashing the whole
// process, since this goroutine runs outside any request's recover
// middleware, the same reasoning as tablews.go's own ws handler recover.
func dispatchGamificationPipeline(tableID, handID string, pipeline func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("gamification: onHandComplete panic recovered", "table", tableID, "hand", handID, "panic", r)
			}
		}()
		pipeline()
	}()
}

func newTableManager(leases *tablelease.Service, store *tablestore.Store, cacheBackend cache.Backend, reg ws.Registry, achv *achievements.Service, leaderboardSvc *leaderboard.Service, rooms *roomstore.Store, sessionStore *sessionlog.Store, pokerStatsStore *pokerstats.Store, matchupStore *matchup.Store, highlightsStore *highlights.Store, recentSvc *recentplayers.Service, players *player.Service, cfg *config.Config, handRevealStore *handreveal.Store) *tablemanager.Manager {
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
	persistHandReveal := func(tableID, handID, mode string, outcome hand.HandOutcome) {
		if handRevealStore == nil || mode != roomstore.CurrencyModeSandbox {
			return
		}
		if !outcome.WonWithoutShowdown || len(outcome.Winners) != 1 {
			return
		}
		room, err := rooms.Get(context.Background(), tableID)
		if err != nil || room == nil {
			slog.Error("handreveal: load room for big blind failed", "table", tableID, "hand", handID, "err", err)
			return
		}
		winnerID := outcome.Winners[0]
		playerHands := make(map[string]handreveal.PlayerHandCode, len(outcome.PlayerHands))
		for id, info := range outcome.PlayerHands {
			playerHands[id] = handreveal.PlayerHandCode{Cards: info.HoleCards}
		}
		record := handreveal.HandRecord{
			HandID: handID, TableID: tableID, BigBlind: room.BigBlind,
			WinnerID: winnerID, WinnerShown: outcome.PlayerHands[winnerID].Revealed,
			PlayerHands: playerHands, EndedAt: time.Now().UnixMilli(),
		}
		if err := handRevealStore.Put(context.Background(), record); err != nil {
			slog.Error("handreveal: record hand failed", "table", tableID, "hand", handID, "err", err)
		}
	}
	// onHandComplete is invoked synchronously on the table actor's own
	// goroutine (table/actor.go's notifyHandComplete, called from
	// broadcastAll), but everything it does is gamification bookkeeping —
	// achievements, leaderboard, pokerstats, matchup, session/hand history,
	// highlights, recent players — none of which the actor's own state or
	// any client-visible broadcast depends on: broadcastAll already sent
	// every player their post-hand "state" snapshot before it ever reaches
	// this hook, and notifyHandComplete's handhook SET NX claim (fleet-wide
	// once-per-hand dedup, internal/handhook) has already been taken
	// synchronously before this closure runs. So the whole body is safe to
	// detach into its own goroutine, exactly like autoRebuySweep
	// (app.wireAutoRebuyHook) — at real-table scale this was hundreds of
	// sequential DynamoDB round trips blocking the actor and every other
	// player's next action for multiple seconds (#61). A panic here must
	// not take the process down with it, same reasoning as the ws
	// handler's own recover in tablews.go.
	//
	// The handhook claim bounds which instance may run this closure at all,
	// but it fails OPEN on its own Valkey error (see internal/handhook), so
	// a Valkey blip can still let two instances both reach here for the same
	// hand. That is only harmless for the writers below that are already
	// idempotent per (table_id, hand_id) on their own (pokerstats, matchup,
	// and the plain-overwrite session/hand-history, highlights, recent
	// players writes) — the achievements/leaderboard counters are bare ADDs
	// with no guard of their own, so they are additionally gated behind
	// achv.ClaimHandCounters below (issue #66).
	onHandComplete := func(tableID, handID string, outcome hand.HandOutcome, names map[string]string) {
		dispatchGamificationPipeline(tableID, handID, func() {
			ctx := context.Background()
			mode, err := tableCurrencyMode(ctx, rooms, tableID)
			if err != nil {
				slog.Error("gamification: load room mode failed", "table", tableID, "err", err)
				return
			}
			// Player-visible reads (the last-winners strip's hand history and
			// the "maior pote de hoje" highlight) are written first: the client
			// invalidates both the instant it sees the `complete` snapshot, and
			// everything below (achievements, leaderboard, pokerstats, matchup)
			// is tens-to-hundreds of sequential DynamoDB round trips that would
			// otherwise make the client's refetch race ahead of these writes
			// and show the finished hand a whole hand late. All three are plain
			// idempotent overwrites and depend only on the outcome, not on any
			// metrics computed below.
			persistHandHistory(tableID, handID, mode, outcome, names)
			persistHandReveal(tableID, handID, mode, outcome)
			if err := highlightsStore.RecordHand(ctx, tableID, handID, outcome, names); err != nil {
				slog.Error("highlights: record hand failed", "table", tableID, "hand", handID, "err", err)
			}
			var metrics []pokerstats.HandMetric
			actions, metricsErr := store.LoadActionsSince(ctx, tableID, handID, 0)
			if metricsErr != nil {
				slog.Error("pokerstats: load hand actions failed", "table", tableID, "hand", handID, "err", metricsErr)
			} else {
				metrics = pokerstats.Analyze(outcome.Participants, actions)
			}
			// peeked scans the whole hand's action log (unlike pokerstats.Analyze,
			// which intentionally stops at the flop) since a player can peek at
			// their own cards at any street, not just preflop.
			peeked := make(map[string]bool)
			// Time bank is charged per action, so one hand can carry several
			// charges for the same player.
			timeBankMs := make(map[string]int64)
			for _, entry := range actions {
				if entry.Action == "peek_cards" {
					peeked[entry.PlayerID] = true
				}
				timeBankMs[entry.PlayerID] += entry.TimeBankMs
			}
			achievementMetrics := make([]achievements.HandMetric, len(metrics))
			for i, metric := range metrics {
				achievementMetrics[i] = achievements.HandMetric{
					PlayerID:   metric.PlayerID,
					VPIP:       metric.VPIP,
					ThreeBet:   metric.ThreeBet,
					Peeked:     peeked[metric.PlayerID],
					TimeBankMs: timeBankMs[metric.PlayerID],
				}
			}
			// claimHandCounters is the correctness backstop for the two
			// non-idempotent ADD-based writers below (achievements'
			// counters, leaderboard's hands_played/hands_won/achievement
			// points): internal/handhook's own claim already dedupes which
			// instance may reach this pipeline at all, but it fails OPEN on
			// a Valkey error, so a Valkey blip during hand completion can
			// let two instances both pass it and both reach here — and
			// unlike matchup/pokerstats (which guard their own
			// TransactWrite) neither of these had a per-hand guard of their
			// own before issue #66. Skip both entirely when the claim is
			// lost or errors — an error here fails CLOSED (never runs the
			// increments) rather than open, because this guard IS the
			// source of truth for "already counted", not an optional
			// accelerator. matchup/pokerstats/sessionlog/highlights/recent
			// players are unaffected: they stay outside this guard exactly
			// as before, since each is already idempotent on its own.
			claimed, err := achv.ClaimHandCounters(ctx, tableID, handID)
			if err != nil {
				slog.Error("gamification: hand counter guard failed, skipping achievement/leaderboard increments", "table", tableID, "hand", handID, "err", err)
			} else if !claimed {
				slog.Info("gamification: hand counters already claimed for this hand, skipping duplicate achievement/leaderboard increments", "table", tableID, "hand", handID)
			} else {
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
			}
			if metricsErr == nil {
				if err := pokerStatsStore.RecordHand(ctx, mode, tableID, handID, metrics); err != nil {
					slog.Error("pokerstats: record hand failed", "table", tableID, "hand", handID, "err", err)
				}
			}
			if err := matchupStore.RecordHand(ctx, mode, tableID, handID, outcome); err != nil {
				slog.Error("matchup: record hand failed", "table", tableID, "hand", handID, "err", err)
			}
			if recentSvc != nil {
				if err := recentSvc.RecordHand(ctx, tableID, handID, outcome.Participants, time.Now()); err != nil {
					slog.Error("recent players: record hand failed", "table", tableID, "hand", handID, "err", err)
				}
			}
		})
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
	// The streak badge is shared state, not per-process state: several
	// instances can run an actor for one table and each used to publish its
	// own tally (see internal/tablestreak).
	mgr.SetStreakStore(tablestreak.NewService(cacheBackend))
	// Post-hand hooks credit non-idempotent counters, and any instance that
	// merely broadcasts a table already sitting on Complete used to fire them
	// again (see internal/handhook). The claim needs SET NX, which
	// cache.Backend cannot express, so it takes the raw client — same reason
	// newPresenceStore does.
	if redis, ok := cacheBackend.(*cache.RedisBackend); ok {
		mgr.SetHandHookClaimer(handhook.NewService(redis.Client()))
	}
	// The seat's connection dot has to see sockets terminating on other
	// instances too (see internal/tableconn).
	mgr.SetConnStore(tableconn.NewService(cacheBackend))
	mgr.SetOnTableStreak(func(tableID, handID string, outcome hand.HandOutcome) map[string]int {
		mode, err := tableCurrencyMode(context.Background(), rooms, tableID)
		if err != nil {
			slog.Error("achievements: load room mode for table streak failed", "table", tableID, "hand", handID, "err", err)
			return nil
		}
		streaks, err := achv.RecordTableStreak(context.Background(), tableID, mode, outcome)
		if err != nil {
			slog.Error("achievements: record table streak failed", "table", tableID, "hand", handID, "err", err)
			return nil
		}
		return streaks
	})
	mgr.SetOnHandUpdated(func(tableID, handID string, outcome hand.HandOutcome, names map[string]string) {
		mode, err := tableCurrencyMode(context.Background(), rooms, tableID)
		if err != nil {
			slog.Error("sessionlog: load room mode for hand update failed", "table", tableID, "hand", handID, "err", err)
			return
		}
		persistHandHistory(tableID, handID, mode, outcome, names)
		persistHandReveal(tableID, handID, mode, outcome)
	})
	mgr.SetOnSeatsChanged(func(tableID string, seatsTaken int) {
		if err := rooms.SetSeatsTaken(context.Background(), tableID, seatsTaken); err != nil {
			slog.Error("roomstore: seats taken write-through failed", "table", tableID, "err", err)
		}
		data, err := goproto.Marshal(&pokerproto.ServerMessage{
			Type:       "room_updated",
			RoomId:     tableID,
			SeatsTaken: int32(seatsTaken),
		})
		if err != nil {
			slog.Error("room update event serialization failed", "table", tableID, "err", err)
		} else {
			reg.Broadcast(context.Background(), "lobby", data)
		}
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
		SmallBlind: outcome.SmallBlind, BigBlind: outcome.BigBlind,
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
	mgr.SetReactionMarkUsed(reactionSvc.BuildMarkUsedIntent)
	reactionSvc.SetOwnershipInvalidator(reactionOwnershipCache.Invalidate)
	mgr.SetOnPlayerRemoved(func(tableID, playerID, reason, settlementNonce string, stack int64, holdID string) {
		ctx := context.Background()
		// Pushes an explicit "removed" frame straight to the removed player's
		// own connection (same per-player fan-out key the "state" broadcast
		// uses) — without it, a system-removed player's socket just stops
		// receiving state broadcasts (they're no longer in PlayersForActor)
		// with no signal telling the client why, so it sits on a stale table
		// instead of redirecting to the lobby.
		data, err := goproto.Marshal(&pokerproto.ServerMessage{
			Type:   "removed",
			Code:   reason,
			Amount: stack,
		})
		if err != nil {
			slog.Error("player removed event serialization failed", "table", tableID, "player", playerID, "err", err)
		} else {
			reg.Broadcast(ctx, tableID+"#"+playerID, data)
		}
		if err := buyinSvc.SettleSystemRemoval(ctx, tableID, playerID, stack, holdID, reason, settlementNonce); err != nil {
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

func registerRoutesWithSocialRuntime(
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
	handRevealStore *handreveal.Store,
	handRevealSvc *handreveal.Service,
	pokerStatsStore *pokerstats.Store,
	matchupStore *matchup.Store,
	highlightsStore *highlights.Store,
	avatars *avatar.Service,
	sandboxPurchaseSvc *sandboxpurchase.Service,
	reactionPurchaseSvc *reactionpurchase.Service,
	cosmeticPurchaseSvc *cosmeticpurchase.Service,
	socialSvc *social.Service,
	presenceSvc *presence.Service,
	recentSvc *recentplayers.Service,
	reportSvc *reports.Service,
) {
	v1.Register(app, cfg, db, verifier, manager, reg, roomBackedSeed(rooms), cacheBackend, rooms, buyinSvc, players, leaderboardSvc, dailyRewardSvc, tableStore, sessionStore, achievementStore, playerNoteStore, handShareStore, handRevealStore, handRevealSvc, pokerStatsStore, matchupStore, highlightsStore, avatars, sandboxPurchaseSvc, reactionPurchaseSvc, cosmeticPurchaseSvc, socialSvc, presenceSvc, recentSvc, reportSvc)
}

// registerRoutes retains the narrow construction seam used by older unit
// tests; production invokes registerRoutesWithSocialRuntime above.
func registerRoutes(
	app *fiber.App, cfg *config.Config, db *dynamodb.Client, verifier *jwtverify.Verifier,
	manager *tablemanager.Manager, reg ws.Registry, cacheBackend cache.Backend,
	rooms *roomstore.Store, buyinSvc *buyin.Service, players *player.Service,
	leaderboardSvc *leaderboard.Service, dailyRewardSvc *dailyreward.Service,
	tableStore *tablestore.Store, sessionStore *sessionlog.Store, achievementStore *achievements.Store,
	playerNoteStore *playernotes.Store, handShareStore *handshare.Store, pokerStatsStore *pokerstats.Store,
	highlightsStore *highlights.Store,
	avatars *avatar.Service, sandboxPurchaseSvc *sandboxpurchase.Service,
	reactionPurchaseSvc *reactionpurchase.Service, cosmeticPurchaseSvc *cosmeticpurchase.Service, socialSvc *social.Service,
) {
	v1.Register(app, cfg, db, verifier, manager, reg, roomBackedSeed(rooms), cacheBackend, rooms, buyinSvc, players, leaderboardSvc, dailyRewardSvc, tableStore, sessionStore, achievementStore, playerNoteStore, handShareStore, nil, nil, pokerStatsStore, nil, highlightsStore, avatars, sandboxPurchaseSvc, reactionPurchaseSvc, cosmeticPurchaseSvc, socialSvc, nil, nil, nil)
}

// wsDrainGrace is how long OnStop waits after sending close frames so
// clients can process them and start reconnecting. Kept well under the 5s
// ShutdownWithContext budget it precedes.
const wsDrainGrace = 1500 * time.Millisecond

// validateWalletScopes fails startup loudly when real-money mode is enabled
// but ctech-account has not granted poker's M2M client the two scopes
// DebitReal/IsGamblingActivated depend on (internal:wallet:debit-real,
// internal:wallet:game-status — see walletclient.ValidateRequiredScopes and
// api/CLAUDE.md's "Still blocking" note: granting them is a config change in
// ctech-account, not something this repo can fix on its own). Registered
// before startServer so a broken grant never reaches "listening" — the
// process exits with a clear cause instead of the first real player's entry
// fee or gambling-activation check failing silently in production.
func validateWalletScopes(lc fx.Lifecycle, cfg *config.Config, wallet *walletclient.Client) {
	if !cfg.RealMoneyEnabled {
		return
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := wallet.ValidateRequiredScopes(ctx); err != nil {
				return fmt.Errorf("real-money mode (REAL_MONEY_ENABLED=true) is enabled but startup scope check failed: %w", err)
			}
			slog.Info("ctech-account wallet scopes verified for real-money mode")
			return nil
		},
	})
}

// spotTerminationMetadataURL is the EC2 instance metadata (IMDS) path that
// reports an impending spot reclamation
// (https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-instance-termination-notices.html):
// a 200 response means this instance has ~2 minutes left; 404 means no
// notice yet. A var, not a const, so tests can point it at an httptest
// server. Deliberately IMDSv1 (plain GET, no token dance) — this only ever
// reads a single unauthenticated, non-sensitive metadata path.
var spotTerminationMetadataURL = "http://169.254.169.254/latest/meta-data/spot/instance-action"

// spotPollInterval matches the "every few seconds" the issue asks for —
// frequent enough that the ~2 minute spot notice window comfortably covers
// several polls, without hammering IMDS. A var (not const) so tests can
// speed it up instead of waiting out a real 5s ticker.
var spotPollInterval = 5 * time.Second

const (
	// spotPollTimeout bounds a single IMDS request. On any host that isn't
	// EC2 (local/dev, CI) 169.254.169.254 is unroutable and fails fast well
	// under this; it exists to bound the rare hung-connection case.
	spotPollTimeout = 2 * time.Second
	// spotDrainTimeout bounds the proactive drain triggered by a termination
	// notice. Generous relative to the ~2 minute spot warning, but bounded so
	// a stuck actor can't wedge the poller forever.
	spotDrainTimeout = 30 * time.Second
)

func startServer(lc fx.Lifecycle, app *fiber.App, cfg *config.Config, manager *tablemanager.Manager) {
	pollCtx, cancelPoll := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			addr := ":" + strconv.Itoa(cfg.Port)
			slog.Info("starting ctech-poker-api", "addr", addr, "env", cfg.Env)
			go func() {
				if err := app.Listen(addr); err != nil {
					slog.Error("server stopped", "err", err)
				}
			}()
			go monitorMemoryPressure(pollCtx)
			// Reduce reliance on the ASG lifecycle hook + drain Lambda alone
			// (#33): during the 2026-09-01 spot rebalance storm it fired for
			// only 3 of at least 4-5 real terminations, stranding leases the
			// commit-guard backstops then had to paper over (api/CLAUDE.md,
			// docs/specs/2026-09-01-duplicate-seat-commit-guard.md). This
			// instance also polls its own spot termination notice directly
			// and drains proactively the moment one appears, instead of
			// waiting solely on the external hook to complete before
			// termination. Only meaningful on real EC2 instances (prod);
			// skip it elsewhere so dev/test never spend a background
			// goroutine polling a link-local address that doesn't exist.
			if cfg.Env == "prod" {
				go pollSpotTermination(pollCtx, manager, &http.Client{Timeout: spotPollTimeout})
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			cancelPoll()
			// Hand every live socket a 1001 "going away" before anything else:
			// ShutdownWithContext below force-closes them once its window
			// elapses, and a client that learns about it only from a dead read
			// starts reconnecting far later than one that got a close frame.
			// See docs/specs/2026-08-24-graceful-ws-shutdown-on-deploy.md.
			if n := wsdrain.CloseAll(ctx, wsDrainGrace); n > 0 {
				slog.Info("sent websocket going-away frames", "conns", n)
			}
			slog.Info("shutting down ctech-poker-api, draining table manager leases")
			// Idempotent (#33): a no-op here if pollSpotTermination already
			// drained this instance proactively.
			manager.DrainAndRelease(ctx)
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return app.ShutdownWithContext(stopCtx)
		},
	})
}

// pollSpotTermination polls this instance's own EC2 metadata for a spot
// termination notice and, the moment one appears, proactively runs the same
// idempotent manager.DrainAndRelease the OnStop SIGTERM handler above runs —
// rather than waiting solely on the ASG lifecycle hook's drain Lambda to
// reach this instance before AWS reclaims it (#33). Exits after the first
// detected notice (nothing left to poll for once this instance is
// draining); ctx cancellation (OnStop) also stops it early on a normal
// deploy, where no notice ever appears.
func pollSpotTermination(ctx context.Context, manager *tablemanager.Manager, client *http.Client) {
	ticker := time.NewTicker(spotPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			noticed, err := spotTerminationNoticed(ctx, client)
			if err != nil {
				// Not on EC2, or IMDS transiently unreachable — try again
				// next tick rather than logging noise every 5s.
				continue
			}
			if !noticed {
				continue
			}
			slog.Warn("spot termination notice detected via instance metadata, draining proactively ahead of the lifecycle hook")
			drainCtx, cancel := context.WithTimeout(context.Background(), spotDrainTimeout)
			manager.DrainAndRelease(drainCtx)
			cancel()
			return
		}
	}
}

// spotTerminationNoticed reports whether IMDS currently has a spot
// instance-action published for this instance. A non-nil error means the
// check was inconclusive (not on EC2, network error, unexpected status) —
// callers must treat that as "no notice yet", not as a notice.
func spotTerminationNoticed(ctx context.Context, client *http.Client) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, spotPollTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, spotTerminationMetadataURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected instance metadata status %d", resp.StatusCode)
	}
}
