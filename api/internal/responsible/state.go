// Package responsible owns the responsible-gambling rules of real-money play:
// consent to the gambling addendum, personal deposit limits, and self-exclusion.
// They lived in ctech-wallet until the wallet became a plain ledger; the domain
// rules in domain.go are ported unchanged (and so are their tests).
package responsible

import "time"

// CurrentGamblingAddendumVersion is the responsible-gambling addendum a player must
// have accepted to play for real money. Acceptance is a computed equality against
// this constant, never a stored boolean, so bumping it re-gates everyone at once.
// Carried over from ctech-wallet (domain/wallet/user.go) with the same value.
const CurrentGamblingAddendumVersion = "2.1"

const (
	exclusionRevokeFloor = 90 * 24 * time.Hour
	cooldownIncrease     = 7 * 24 * time.Hour
	cooldownMonthly      = 14 * 24 * time.Hour
)

// State is one player's whole responsible-gambling record. One DynamoDB item, so
// a consent, a limit change and the inflow counters never tear against each other;
// Version makes every write conditional.
type State struct {
	UserID                  string               `dynamodbav:"pk" json:"user_id"`
	SK                      string               `dynamodbav:"sk" json:"-"`
	Version                 int64                `dynamodbav:"version" json:"-"`
	GamblingAddendumVersion string               `dynamodbav:"gambling_addendum_version,omitempty" json:"gambling_addendum_version,omitempty"`
	GamblingAcceptedAt      string               `dynamodbav:"gambling_accepted_at,omitempty" json:"gambling_accepted_at,omitempty"`
	SelfExclusion           *SelfExclusion       `dynamodbav:"self_exclusion,omitempty" json:"self_exclusion,omitempty"`
	GameLimits              *GameLimits          `dynamodbav:"game_limits,omitempty" json:"game_limits,omitempty"`
	GameDepositCounters     *GameDepositCounters `dynamodbav:"game_deposit_counters,omitempty" json:"game_deposit_counters,omitempty"`
	// CarriedChecked is set once the player's CPF hash was looked up against the
	// exclusions carried over from erased accounts (see Service.Eligibility).
	CarriedChecked bool   `dynamodbav:"carried_checked,omitempty" json:"-"`
	ImportedAt     string `dynamodbav:"imported_at,omitempty" json:"-"`
	UpdatedAt      string `dynamodbav:"updated_at,omitempty" json:"-"`
}

// StateSK is the sort key of the state item (the table also holds audit and
// inflow-marker items under the same partition key).
const StateSK = "STATE"

// GamblingAccepted reports whether the player accepted the CURRENT addendum.
func (u *State) GamblingAccepted() bool {
	return u != nil && u.GamblingAddendumVersion == CurrentGamblingAddendumVersion
}

// CarriedExclusion is a self-exclusion kept past the erasure of the account that
// made it, keyed by ctech-account's CPF keyed hash, until ExpiresAt.
type CarriedExclusion struct {
	CPFHMAC       string        `json:"cpf_hmac"`
	SelfExclusion SelfExclusion `json:"exclusion"`
	ExpiresAt     int64         `json:"expires_at"` // unix seconds
}

// ErasureRetentionYears is how long an indefinite (or unparseable) exclusion is
// kept after the account is erased. Same value as ctech-wallet's rule.
const ErasureRetentionYears = 5

// CarriedSelfExclusion decides whether ex outlives the account and until when.
// A timed exclusion is kept until its end. An indefinite one, or one whose end
// cannot be parsed (fail closed, like SelfExcluded), is kept ErasureRetentionYears
// from now. An expired one carries nothing. Ported from ctech-wallet.
func CarriedSelfExclusion(ex *SelfExclusion, now time.Time) (SelfExclusion, time.Time, bool) {
	if ex == nil {
		return SelfExclusion{}, time.Time{}, false
	}
	if ex.Until == "" {
		return *ex, now.AddDate(ErasureRetentionYears, 0, 0), true
	}
	until, err := time.Parse(time.RFC3339, ex.Until)
	if err != nil {
		return *ex, now.AddDate(ErasureRetentionYears, 0, 0), true
	}
	if !now.Before(until) {
		return SelfExclusion{}, time.Time{}, false
	}
	return *ex, until, true
}
