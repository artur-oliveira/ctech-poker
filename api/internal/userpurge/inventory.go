package userpurge

// Unprefixed table names. Each owning package keeps its own unexported
// constant; this file mirrors them so the purge can address every table, and
// TestInventoryClassifiesEveryTable fails when cdk/lib declares a poker_*
// table that inventory does not classify.
const (
	TableTableState           = "poker_table_state"
	TableTableStateHistory    = "poker_table_state_history"
	TableActionLog            = "poker_action_log"
	TableActionGuards         = "poker_action_guards"
	TableRooms                = "poker_rooms"
	TablePlayerProfiles       = "poker_player_profiles"
	TableAchievementProgress  = "poker_achievement_progress"
	TableLeaderboardStats     = "poker_leaderboard_stats"
	TableDailyReward          = "poker_daily_reward"
	TablePendingCashouts      = "poker_pending_cashouts"
	TablePlayerSessions       = "poker_player_sessions"
	TablePlayerHands          = "poker_player_hands"
	TablePlayerNotes          = "poker_player_notes"
	TableHandMeta             = "poker_hand_meta"
	TableHandShares           = "poker_hand_shares"
	TablePokerStats           = "poker_player_poker_stats"
	TableMatchups             = "poker_player_matchups"
	TableSandboxPurchases     = "poker_sandbox_purchases"
	TableReactionEntitlements = "poker_reaction_entitlements"
	TableReactionPurchases    = "poker_reaction_purchases"
	TableCosmeticEntitlements = "poker_cosmetic_entitlements"
	TableCosmeticPurchases    = "poker_cosmetic_purchases"
	TableCosmeticLoadouts     = "poker_cosmetic_loadouts"
	TableChatPrefs            = "poker_chat_prefs"
	TableBotCheckContests     = "poker_botcheck_contests"
	TableWalletAlertPrefs     = "poker_wallet_alert_prefs"
	TablePromoCodes           = "poker_promo_codes"
	TablePromoRedemptions     = "poker_promo_redemptions"
	TableTableEntitlements    = "poker_table_entitlements"
	TableTableHighlights      = "poker_table_highlights"
	TableHandReveals          = "poker_hand_reveals"
	TableHandRevealPayments   = "poker_hand_reveal_payments"
	TableSocialEdges          = "poker_social_edges"
	TableRecentPlayers        = "poker_recent_players"
	TableSocialEvents         = "poker_social_events"
	TablePlayerReports        = "poker_player_reports"
	TableErasureState         = "poker_erasure_state"
	TableResponsible          = "poker_responsible"

	// CountSelfExclusionCarried is the ack count for an exclusion kept past the account.
	CountSelfExclusionCarried = "self_exclusion_carried"
)

// Index names the purge queries (cdk/lib/dynamodb-stack.ts).
const (
	gsiReportReporter    = "gsi_reporter"           // reports/store.go
	gsiHandShareOwner    = "gsi_owner"              // handshare/store.go
	gsiPlayerSettlements = "gsi_player_settlements" // reconcile/pending.go
)

// inventory is the data inventory §7 as built: the treatment of every table
// (plan rulings R7, R9, R11).
var inventory = map[string]string{
	TableTableState:           "untouched: live actor-owned item; the user cannot be seated (blocker), the chat ring rolls over, 7-day TTL",
	TableTableStateHistory:    "anonymize: every snapshot of every table the user played",
	TableActionLog:            "anonymize: every action of every hand the user played, plus the S3 archive",
	TableActionGuards:         "untouched: no player data",
	TableRooms:                "untouched: created_by / invite grants hold the opaque sub only, reaped by TTL",
	TablePlayerProfiles:       "erase: own row, avatar-report guard rows by/about the user, the user in avatar_reporters sets",
	TableAchievementProgress:  "erase",
	TableLeaderboardStats:     "anonymize: re-keyed to the pseudonym, player_name dropped",
	TableDailyReward:          "erase",
	TablePendingCashouts:      "erase: resolved rows only; an unresolved row is a blocker",
	TablePlayerSessions:       "erase: sessions and buyinguard# rows",
	TablePlayerHands:          "anonymize other players' rows of shared hands; erase the user's own rows",
	TablePlayerNotes:          "erase: notes written by and about the user",
	TableHandMeta:             "erase",
	TableHandShares:           "erase",
	TablePokerStats:           "erase",
	TableMatchups:             "erase: a pair aggregate is only readable with both real ids",
	TableSandboxPurchases:     "erase (the payment is retained in the wallet ledger)",
	TableReactionEntitlements: "erase",
	TableReactionPurchases:    "erase (the payment is retained in the wallet ledger)",
	TableCosmeticEntitlements: "erase",
	TableCosmeticPurchases:    "erase (the payment is retained in the wallet ledger)",
	TableCosmeticLoadouts:     "erase",
	TableChatPrefs:            "erase",
	TableBotCheckContests:     "erase",
	TableWalletAlertPrefs:     "erase",
	TablePromoCodes:           "untouched: counters only",
	TablePromoRedemptions:     "erase",
	TableTableEntitlements:    "erase",
	TableTableHighlights:      "anonymize: every day of every table the user played",
	TableHandReveals:          "anonymize: every hand the user played",
	TableHandRevealPayments:   "erase (the payment is retained in the wallet ledger)",
	TableSocialEdges:          "erase: both directions",
	TableRecentPlayers:        "erase the user's rows; anonymize opponents' rows of shared hands",
	TableSocialEvents:         "erase: own inbox, and events/invite guards the user caused in friends' inboxes",
	TablePlayerReports:        "retain anonymized 1 year: re-keyed to the pseudonym, the user's own text dropped",
	TableErasureState:         "untouched: the saga's own lock/tombstone (api-commons erasure.Store)",
	TableResponsible:          "erase: consent, limits, counters, audit and inflow markers; an active self-exclusion is carried first, keyed by the CPF keyed hash (carried rows are keyed SELFEXCL#<hash>, not by the user)",
}
