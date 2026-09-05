// Package reports defines player-safety reports and their moderation lifecycle.
package reports

type Status string
type Category string
type Surface string
type Resolution string

const (
	StatusOpen      Status = "open"
	StatusReviewing Status = "reviewing"
	StatusResolved  Status = "resolved"

	CategoryHarassment           Category = "harassment"
	CategoryHate                 Category = "hate"
	CategorySpam                 Category = "spam"
	CategoryCheating             Category = "cheating"
	CategoryInappropriateProfile Category = "inappropriate_profile"
	CategoryOther                Category = "other"

	SurfaceTableChat     Surface = "table_chat"
	SurfaceTableReaction Surface = "table_reaction"
	SurfaceTableBehavior Surface = "table_behavior"
	SurfaceProfile       Surface = "profile"
	SurfaceRecentPlayer  Surface = "recent_player"

	ResolutionNoAction            Resolution = "no_action"
	ResolutionContentRemoved      Resolution = "content_removed"
	ResolutionWarningRequested    Resolution = "warning_requested"
	ResolutionSuspensionRequested Resolution = "suspension_requested"
)

type Report struct {
	TargetPlayerID    string     `dynamodbav:"pk" json:"target_player_id"`
	StorageKey        string     `dynamodbav:"sk" json:"storage_key"`
	ReportID          string     `dynamodbav:"report_id" json:"report_id"`
	ReporterPlayerID  string     `dynamodbav:"reporter_id" json:"reporter_player_id"`
	Category          Category   `dynamodbav:"category" json:"category"`
	Surface           Surface    `dynamodbav:"surface" json:"surface"`
	TableID           string     `dynamodbav:"table_id,omitempty" json:"table_id,omitempty"`
	HandID            string     `dynamodbav:"hand_id,omitempty" json:"hand_id,omitempty"`
	ActionID          string     `dynamodbav:"action_id,omitempty" json:"action_id,omitempty"`
	ReactionID        string     `dynamodbav:"reaction_id,omitempty" json:"reaction_id,omitempty"`
	Details           string     `dynamodbav:"details,omitempty" json:"details,omitempty"`
	EvidenceMessage   string     `dynamodbav:"evidence_message,omitempty" json:"evidence_message,omitempty"`
	Status            Status     `dynamodbav:"status" json:"status"`
	CreatedAt         int64      `dynamodbav:"created_at" json:"created_at"`
	ReviewedAt        int64      `dynamodbav:"reviewed_at,omitempty" json:"reviewed_at,omitempty"`
	ReviewedBy        string     `dynamodbav:"reviewed_by,omitempty" json:"reviewed_by,omitempty"`
	Resolution        Resolution `dynamodbav:"resolution,omitempty" json:"resolution,omitempty"`
	ResolvedAt        int64      `dynamodbav:"resolved_at,omitempty" json:"resolved_at,omitempty"`
	ResolvedBy        string     `dynamodbav:"resolved_by,omitempty" json:"resolved_by,omitempty"`
	StatusPartition   string     `dynamodbav:"gsi_status_pk" json:"-"`
	StatusSort        string     `dynamodbav:"gsi_status_sk" json:"-"`
	ReporterPartition string     `dynamodbav:"gsi_reporter_pk" json:"-"`
	ReporterSort      string     `dynamodbav:"gsi_reporter_sk" json:"-"`
	TTL               int64      `dynamodbav:"ttl,omitempty" json:"-"`
}

// Summary deliberately excludes reporter-authored and copied evidence text.
// It is the only shape used by list and public HTTP responses.
type Summary struct {
	TargetPlayerID   string     `json:"target_player_id"`
	StorageKey       string     `json:"storage_key,omitempty"`
	ReportID         string     `json:"report_id"`
	ReporterPlayerID string     `json:"reporter_player_id,omitempty"`
	Category         Category   `json:"category"`
	Surface          Surface    `json:"surface"`
	TableID          string     `json:"table_id,omitempty"`
	HandID           string     `json:"hand_id,omitempty"`
	ActionID         string     `json:"action_id,omitempty"`
	ReactionID       string     `json:"reaction_id,omitempty"`
	Status           Status     `json:"status"`
	CreatedAt        int64      `json:"created_at"`
	Resolution       Resolution `json:"resolution,omitempty"`
}

// PlayerReportView is the only shape ever returned to the reporter over
// HTTP: no reviewer, no evidence, no reporter/target ids (redundant — the
// caller already knows they filed it), and Resolution translated to a
// generic status message rather than the raw internal enum.
type PlayerReportView struct {
	ReportID          string   `json:"report_id"`
	Category          Category `json:"category"`
	Surface           Surface  `json:"surface"`
	TableID           string   `json:"table_id,omitempty"`
	HandID            string   `json:"hand_id,omitempty"`
	Status            Status   `json:"status"`
	CreatedAt         int64    `json:"created_at"`
	ResolutionMessage string   `json:"resolution_message,omitempty"`
}

func (s Summary) ForReporter() PlayerReportView {
	return PlayerReportView{ReportID: s.ReportID, Category: s.Category, Surface: s.Surface,
		TableID: s.TableID, HandID: s.HandID, Status: s.Status, CreatedAt: s.CreatedAt,
		ResolutionMessage: ResolutionMessage(s.Status, s.Resolution)}
}

func (r Report) Summary() Summary {
	return Summary{TargetPlayerID: r.TargetPlayerID, StorageKey: r.StorageKey, ReportID: r.ReportID,
		ReporterPlayerID: r.ReporterPlayerID, Category: r.Category, Surface: r.Surface,
		TableID: r.TableID, HandID: r.HandID, ActionID: r.ActionID, ReactionID: r.ReactionID,
		Status: r.Status, CreatedAt: r.CreatedAt, Resolution: r.Resolution}
}

// ResolutionMessage translates a moderation resolution into a short, generic
// message safe to show the reporter — never the raw internal enum, and never
// anything that implies what moderation action was actually taken.
func ResolutionMessage(status Status, resolution Resolution) string {
	if status != StatusResolved {
		return ""
	}
	switch resolution {
	case ResolutionNoAction:
		return "Denúncia analisada, sem ação necessária"
	case ResolutionContentRemoved, ResolutionWarningRequested, ResolutionSuspensionRequested:
		return "Ação tomada"
	default:
		return "Denúncia analisada"
	}
}

type Page struct {
	Reports    []Report `json:"reports"`
	NextCursor string   `json:"next_cursor,omitempty"`
}
