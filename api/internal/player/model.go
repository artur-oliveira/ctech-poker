package player

const CurrentPokerTermsVersion = "1.0"

// Wallet modes a player can pick in their profile — which balance the lobby
// should show/filter by. Enforced at the buy-in boundary already lives on
// roomstore.Room.CurrencyMode; this is only the player's own display/filter
// preference.
const (
	WalletModeSandbox = "sandbox"
	WalletModeReal    = "real"
)

type PlayerProfile struct {
	UserID            string `dynamodbav:"pk" json:"user_id"`
	Name              string `dynamodbav:"name,omitempty" json:"name,omitempty"`
	WalletMode        string `dynamodbav:"wallet_mode,omitempty" json:"wallet_mode,omitempty"`
	PokerTermsVersion string `dynamodbav:"poker_terms_version,omitempty" json:"-"`
	TermsAcceptedAt   string `dynamodbav:"poker_terms_accepted_at,omitempty" json:"poker_terms_accepted_at,omitempty"`
	CreatedAt         string `dynamodbav:"created_at" json:"-"`
	UpdatedAt         string `dynamodbav:"updated_at" json:"-"`
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
