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

type PlayerProfile struct {
	UserID               string   `dynamodbav:"pk" json:"user_id"`
	Name                 string   `dynamodbav:"name,omitempty" json:"name,omitempty"`
	FriendCode           string   `dynamodbav:"friend_code,omitempty" json:"friend_code,omitempty"`
	WalletMode           string   `dynamodbav:"wallet_mode,omitempty" json:"wallet_mode,omitempty"`
	DeckVariant          string   `dynamodbav:"deck_variant,omitempty" json:"deck_variant,omitempty"`
	ShowcasePublic       bool     `dynamodbav:"showcase_public,omitempty" json:"showcase_public"`
	PlaystylePublic      bool     `dynamodbav:"playstyle_public,omitempty" json:"playstyle_public"`
	FeaturedAchievements []string `dynamodbav:"featured_achievements,omitempty" json:"featured_achievements,omitempty"`
	FavoriteReactions    []string `dynamodbav:"favorite_reactions,omitempty" json:"favorite_reactions,omitempty"`
	PokerTermsVersion    string   `dynamodbav:"poker_terms_version,omitempty" json:"-"`
	TermsAcceptedAt      string   `dynamodbav:"poker_terms_accepted_at,omitempty" json:"poker_terms_accepted_at,omitempty"`
	AvatarKey            string   `dynamodbav:"avatar_key,omitempty" json:"-"`
	AvatarVersion        int      `dynamodbav:"avatar_version,omitempty" json:"-"`
	AvatarBlocked        bool     `dynamodbav:"avatar_blocked,omitempty" json:"-"`
	CreatedAt            string   `dynamodbav:"created_at" json:"-"`
	UpdatedAt            string   `dynamodbav:"updated_at" json:"-"`
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
