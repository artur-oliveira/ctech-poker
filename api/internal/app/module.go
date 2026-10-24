package app

import (
	"go.uber.org/fx"

	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// infraModule: process-level plumbing: config, HTTP server, Valkey, DynamoDB, auth verifier, websocket registry, erasure state.
var infraModule = fx.Module("infra",
	fx.Provide(
		config.Load,
		newFiberApp,
		newCacheBackend,
		newRealtimeValkeyClient,
		newVerifier,
		newSupervisor,
		newWsRegistry,
		newDynamoClient,
		newErasureStore,
		newEligibility,
	),
)

// tablesModule: the table/room runtime: leases, stores, the table manager and the buy-in service.
var tablesModule = fx.Module("tables",
	fx.Provide(
		newTableLeaseService,
		newTableStore,
		newRoomStore,
		newSessionStore,
		newPendingStore,
		newEntitlementStore,
		newBuyinService,
		newTableManager,
	),
)

// playersModule: player-facing domains: profiles, hands, stats, leaderboard, achievements, chat, responsible gambling.
var playersModule = fx.Module("players",
	fx.Provide(
		newAvatarService,
		newPlayerStore,
		newPlayerService,
		newPlayerNoteStore,
		newHandMetaStore,
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
		newChatPrefsStore,
		newChatPrefsCache,
		newBotCheckContestStore,
		newResponsibleStore,
		newResponsibleService,
	),
)

// purchasesModule: wallet-backed purchases: cosmetics, reactions, sandbox credits, promo codes, wallet alerts and the wallet/billing clients.
var purchasesModule = fx.Module("purchases",
	fx.Provide(
		newCosmeticsEntitlementStore,
		newCosmeticsPurchaseStore,
		newCosmeticsPurchaseService,
		newCosmeticLoadoutStore,
		newCosmeticLoadoutService,
		newSandboxPurchaseStore,
		newPromoCodeStore,
		newSandboxPurchaseService,
		newWalletAlertStore,
		newWalletAlertService,
		newReactionEntitlementStore,
		newReactionPurchaseStore,
		newReactionPurchaseService,
		newReactionOwnershipCache,
		newBillingClient,
		newPurchaseWallet,
		walletclient.New,
	),
)

// socialModule: friends, presence, recent players, reports.
var socialModule = fx.Module("social",
	fx.Provide(
		newSocialStore,
		newSocialEventStore,
		newSocialService,
		newPresenceStore,
		newPresenceService,
		newRecentPlayersStore,
		newRecentPlayersService,
		newReportStore,
		newReportService,
	),
)

// Module is the root Fx module for the poker API. Invoke order is hook
// registration order, so OnStop runs in the reverse of it: keep startServer last.
var Module = fx.Options(
	infraModule, tablesModule, playersModule, purchasesModule, socialModule,
	fx.Invoke(wirePlayerRemovedHook),
	fx.Invoke(wireAutoRebuyHook),
	fx.Invoke(wireCosmeticCurrentSelection),
	fx.Invoke(wireCosmeticLoadoutApply),
	fx.Invoke(validateWalletScopes),
	fx.Invoke(registerRoutesWithSocialRuntime),
	fx.Invoke(registerErasureRoutes),
	fx.Invoke(startErasureConsumer),
	fx.Invoke(startServer),
)
