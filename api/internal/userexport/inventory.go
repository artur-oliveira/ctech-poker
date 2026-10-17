package userexport

// inventory classifies every poker_* table for the personal-data export (LGPD
// art. 18, V; ctech-account docs/specs/2026-10-17-data-export.md § 5). It
// mirrors userpurge's inventory: a table is either exported (the prefix
// "exported:") or left out for a stated reason ("excluded:"), and
// TestInventoryClassifiesEveryTable fails when cdk/lib declares a poker_* table
// that is in neither, so a new table cannot be added without choosing.
var inventory = map[string]string{
	"poker_table_state":           "excluded: live actor-owned table state, not the person's own record",
	"poker_table_state_history":   "excluded: snapshots of shared tables; the person's own hands are in poker_player_hands",
	"poker_action_log":            "excluded: every player's actions of shared hands; the person's own view of each hand is in poker_player_hands",
	"poker_action_guards":         "excluded: idempotency guards, no player data",
	"poker_rooms":                 "excluded: table configuration; the sub appears only as creator or invitee",
	"poker_player_profiles":       "exported: own profile row",
	"poker_achievement_progress":  "exported",
	"poker_leaderboard_stats":     "exported",
	"poker_daily_reward":          "exported",
	"poker_pending_cashouts":      "exported: own settlements (by player index)",
	"poker_player_sessions":       "exported: own sessions",
	"poker_player_hands":          "exported: own hand history; other players' names, avatars and chat are removed",
	"poker_player_notes":          "exported: notes the person wrote (notes others wrote about them are not exported: they are the authors' data)",
	"poker_hand_meta":             "exported",
	"poker_hand_shares":           "exported: own shares (by owner index)",
	"poker_player_poker_stats":    "exported: sandbox and real stats",
	"poker_player_matchups":       "excluded: pair aggregates are only readable with both real ids, so they describe the opponent as much as the person",
	"poker_sandbox_purchases":     "exported",
	"poker_reaction_entitlements": "exported",
	"poker_reaction_purchases":    "exported",
	"poker_cosmetic_entitlements": "exported",
	"poker_cosmetic_purchases":    "exported",
	"poker_cosmetic_loadouts":     "exported",
	"poker_chat_prefs":            "exported",
	"poker_botcheck_contests":     "exported",
	"poker_wallet_alert_prefs":    "exported",
	"poker_promo_codes":           "excluded: promo code counters, no player data",
	"poker_promo_redemptions":     "exported",
	"poker_table_entitlements":    "exported",
	"poker_table_highlights":      "excluded: daily highlights of shared tables",
	"poker_hand_reveals":          "excluded: reveals of shared hands, reachable only through the hand rows",
	"poker_hand_reveal_payments":  "excluded: the payment itself is in the wallet ledger (wallet's part of the export)",
	"poker_social_edges":          "exported: friends and blocks, as opaque ids",
	"poker_recent_players":        "exported: recently played opponents, as opaque ids",
	"poker_social_events":         "exported: own inbox",
	"poker_player_reports":        "excluded: anti-abuse records that hold the reporters' and reported players' data; to be confirmed with the DPO",
	"poker_erasure_state":         "excluded: the deletion saga's own lock/tombstone",
	"poker_responsible":           "exported: consent, limits, counters, audit and inflow markers",
}
