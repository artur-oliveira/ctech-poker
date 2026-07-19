package player

const CurrentPokerTermsVersion = "1.0"

type PlayerProfile struct {
	UserID            string `dynamodbav:"pk" json:"user_id"`
	PokerTermsVersion string `dynamodbav:"poker_terms_version,omitempty" json:"-"`
	TermsAcceptedAt   string `dynamodbav:"poker_terms_accepted_at,omitempty" json:"poker_terms_accepted_at,omitempty"`
	CreatedAt         string `dynamodbav:"created_at" json:"-"`
	UpdatedAt         string `dynamodbav:"updated_at" json:"-"`
}

func (p *PlayerProfile) TermsAccepted() bool {
	return p != nil && p.PokerTermsVersion == CurrentPokerTermsVersion
}
