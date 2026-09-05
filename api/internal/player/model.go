package player

const CurrentPokerTermsVersion = "2.1"

// Wallet modes a player can pick in their profile — which balance the lobby
// should show/filter by. Enforced at the buy-in boundary already lives on
// roomstore.Room.CurrencyMode; this is only the player's own display/filter
// preference.
const (
	WalletModeSandbox = "sandbox"
	WalletModeReal    = "real"
)

// DefaultDeckVariant matches DEFAULT_DECK_VARIANT in the UI's
// src/lib/cardVariants.ts — kept as a plain string here since the full
// variant catalog (cosmetic-only) lives and grows on the frontend.
const DefaultDeckVariant = "four-color"

// DefaultTableTheme matches DEFAULT_TABLE_THEME (the "classic" entry) in the
// UI's src/lib/tablePreferences.ts.
const DefaultTableTheme = "classic"

const (
	ShowcaseSectionAchievements = "achievements"
	ShowcaseSectionBestHand     = "best_hand"
	ShowcaseSectionMatchup      = "matchup"
)

// ShowcaseLayout controls the order and optional visibility of sections in a
// public profile. Achievements is always present so a valid layout cannot
// produce a silent empty showcase.
type ShowcaseLayout struct {
	Order  []string `dynamodbav:"order,omitempty" json:"order"`
	Hidden []string `dynamodbav:"hidden,omitempty" json:"hidden"`
}

func DefaultShowcaseLayout() ShowcaseLayout {
	return ShowcaseLayout{Order: []string{ShowcaseSectionAchievements, ShowcaseSectionBestHand, ShowcaseSectionMatchup}, Hidden: []string{}}
}

type PlayerProfile struct {
	UserID          string `dynamodbav:"pk" json:"user_id"`
	Name            string `dynamodbav:"name,omitempty" json:"name,omitempty"`
	FriendCode      string `dynamodbav:"friend_code,omitempty" json:"friend_code,omitempty"`
	WalletMode      string `dynamodbav:"wallet_mode,omitempty" json:"wallet_mode,omitempty"`
	DeckVariant     string `dynamodbav:"deck_variant,omitempty" json:"deck_variant,omitempty"`
	TableTheme      string `dynamodbav:"table_theme,omitempty" json:"table_theme,omitempty"`
	ShowcasePublic  bool   `dynamodbav:"showcase_public,omitempty" json:"showcase_public"`
	PlaystylePublic bool   `dynamodbav:"playstyle_public,omitempty" json:"playstyle_public"`
	// TablePublic lets friends see which PUBLIC room this player is sitting
	// in, so they can join it. Off by default: presence is otherwise
	// room-blind by design (see internal/presence), and a private room is
	// never exposed even with this on.
	TablePublic          bool           `dynamodbav:"table_public,omitempty" json:"table_public"`
	FeaturedAchievements []string       `dynamodbav:"featured_achievements,omitempty" json:"featured_achievements,omitempty"`
	ShowcaseLayout       ShowcaseLayout `dynamodbav:"showcase_layout,omitempty" json:"showcase_layout,omitempty"`
	FavoriteReactions    []string       `dynamodbav:"favorite_reactions,omitempty" json:"favorite_reactions,omitempty"`
	// ReactionWheel is the player's own ordered subset of owned reactions for
	// the quick-react wheel (#338) — separate from FavoriteReactions, which is
	// an unordered showcase pick, not a UI ordering.
	ReactionWheel []string `dynamodbav:"reaction_wheel,omitempty" json:"reaction_wheel,omitempty"`
	// StatsGoals holds the player's personal targets for pokerstats metrics
	// (#331), keyed by metric ("vpip_rate", "pfr_rate", "three_bet_rate").
	StatsGoals        map[string]float64 `dynamodbav:"stats_goals,omitempty" json:"stats_goals,omitempty"`
	PokerTermsVersion string             `dynamodbav:"poker_terms_version,omitempty" json:"-"`
	TermsAcceptedAt   string             `dynamodbav:"poker_terms_accepted_at,omitempty" json:"poker_terms_accepted_at,omitempty"`
	AvatarKey         string             `dynamodbav:"avatar_key,omitempty" json:"-"`
	AvatarVersion     int                `dynamodbav:"avatar_version,omitempty" json:"-"`
	AvatarBlocked     bool               `dynamodbav:"avatar_blocked,omitempty" json:"-"`
	CreatedAt         string             `dynamodbav:"created_at" json:"-"`
	UpdatedAt         string             `dynamodbav:"updated_at" json:"-"`
}

func (p *PlayerProfile) TermsAccepted() bool {
	return p != nil && p.PokerTermsVersion == CurrentPokerTermsVersion
}

// EffectiveWalletMode defaults an unset preference to sandbox — a brand new
// profile has never chosen a mode, and sandbox is the safe default.
func (p *PlayerProfile) EffectiveWalletMode() string {
	if p == nil || p.WalletMode == "" {
		return WalletModeSandbox
	}
	return p.WalletMode
}

// EffectiveDeckVariant defaults an unset preference to four-color, same
// rationale as EffectiveWalletMode.
func (p *PlayerProfile) EffectiveDeckVariant() string {
	if p == nil || p.DeckVariant == "" {
		return DefaultDeckVariant
	}
	return p.DeckVariant
}

// EffectiveTableTheme defaults an unset preference to classic, same rationale
// as EffectiveDeckVariant.
func (p *PlayerProfile) EffectiveTableTheme() string {
	if p == nil || p.TableTheme == "" {
		return DefaultTableTheme
	}
	return p.TableTheme
}
