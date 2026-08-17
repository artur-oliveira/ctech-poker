// Package reports defines player-safety reports and their moderation lifecycle.
package reports

type Status string

const (
	StatusOpen      Status = "open"
	StatusReviewing Status = "reviewing"
	StatusResolved  Status = "resolved"
)

type Report struct {
	TargetPlayerID   string `dynamodbav:"pk"`
	StorageKey       string `dynamodbav:"sk"`
	ReportID         string `dynamodbav:"report_id"`
	ReporterPlayerID string `dynamodbav:"reporter_id"`
	Category         string `dynamodbav:"category"`
	Surface          string `dynamodbav:"surface"`
	TableID          string `dynamodbav:"table_id,omitempty"`
	HandID           string `dynamodbav:"hand_id,omitempty"`
	ActionID         string `dynamodbav:"action_id,omitempty"`
	ReactionID       string `dynamodbav:"reaction_id,omitempty"`
	Details          string `dynamodbav:"details,omitempty"`
	EvidenceMessage  string `dynamodbav:"evidence_message,omitempty"`
	Status           Status `dynamodbav:"status"`
	CreatedAt        int64  `dynamodbav:"created_at"`
	Resolution       string `dynamodbav:"resolution,omitempty"`
	ResolvedAt       int64  `dynamodbav:"resolved_at,omitempty"`
	ResolvedBy       string `dynamodbav:"resolved_by,omitempty"`
	StatusPartition  string `dynamodbav:"gsi_status_pk"`
	StatusSort       string `dynamodbav:"gsi_status_sk"`
	TTL              int64  `dynamodbav:"ttl,omitempty"`
}

type Page struct {
	Reports    []Report
	NextCursor string
}
