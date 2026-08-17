package reports

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

const MaxDetailsRunes = 500

var (
	ErrInvalidReport   = errors.New("reports: invalid report")
	ErrTargetNotFound  = errors.New("reports: target player not found")
	ErrEvidenceMissing = errors.New("reports: authoritative evidence not found")
)

type ActionSource interface {
	FindActionByID(context.Context, string, string, string) (*tablestore.ActionLogEntry, error)
}
type PlayerSource interface {
	Get(context.Context, string) (*player.PlayerProfile, error)
}
type MetricFunc func(category Category, surface Surface)

type CreateInput struct {
	TargetPlayerID string
	Category       Category
	Surface        Surface
	TableID        string
	HandID         string
	ActionID       string
	Details        string
}

type Service struct {
	store   Store
	actions ActionSource
	players PlayerSource
	metric  MetricFunc
	now     func() time.Time
}

func NewService(store Store, actions ActionSource, players PlayerSource) *Service {
	return &Service{store: store, actions: actions, players: players, now: time.Now}
}
func (s *Service) WithMetric(metric MetricFunc) *Service { s.metric = metric; return s }

func (s *Service) Create(ctx context.Context, reporterID, idempotencyKey string, input CreateInput) (*Report, error) {
	input.TargetPlayerID, input.TableID, input.HandID, input.ActionID = strings.TrimSpace(input.TargetPlayerID), strings.TrimSpace(input.TableID), strings.TrimSpace(input.HandID), strings.TrimSpace(input.ActionID)
	input.Details = strings.TrimSpace(input.Details)
	if reporterID == "" || idempotencyKey == "" || input.TargetPlayerID == "" || input.TargetPlayerID == reporterID ||
		!ValidCategory(input.Category) || !ValidSurface(input.Surface) || utf8.RuneCountInString(input.Details) > MaxDetailsRunes {
		return nil, ErrInvalidReport
	}
	profile, err := s.players.Get(ctx, input.TargetPlayerID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrTargetNotFound
	}
	report := Report{TargetPlayerID: input.TargetPlayerID, ReporterPlayerID: reporterID, Category: input.Category,
		Surface: input.Surface, TableID: input.TableID, HandID: input.HandID, ActionID: input.ActionID,
		Details: input.Details, Status: StatusOpen, CreatedAt: s.now().UTC().UnixMilli()}
	if err := s.attachEvidence(ctx, &report); err != nil {
		return nil, err
	}
	created, inserted, err := s.store.Create(ctx, report, idempotencyKey)
	if err == nil && inserted && s.metric != nil {
		s.metric(created.Category, created.Surface)
	}
	return created, err
}

func (s *Service) attachEvidence(ctx context.Context, report *Report) error {
	tableSurface := report.Surface == SurfaceTableChat || report.Surface == SurfaceTableReaction || report.Surface == SurfaceTableBehavior
	if !tableSurface {
		if report.TableID != "" || report.HandID != "" || report.ActionID != "" {
			return ErrInvalidReport
		}
		return nil
	}
	if report.TableID == "" || report.HandID == "" {
		return ErrInvalidReport
	}
	if report.ActionID == "" {
		if report.Surface == SurfaceTableBehavior {
			return nil
		}
		return ErrInvalidReport
	}
	if s.actions == nil {
		return ErrEvidenceMissing
	}
	action, err := s.actions.FindActionByID(ctx, report.TableID, report.HandID, report.ActionID)
	if err != nil {
		return err
	}
	if action == nil || action.PlayerID != report.TargetPlayerID {
		return ErrEvidenceMissing
	}
	switch report.Surface {
	case SurfaceTableChat:
		if action.Action != "chat" || action.Message == "" {
			return ErrEvidenceMissing
		}
		report.EvidenceMessage = action.Message
	case SurfaceTableReaction:
		if action.Action != "reaction" || action.ReactionID == "" {
			return ErrEvidenceMissing
		}
		report.ReactionID = action.ReactionID
	}
	return nil
}

func ValidCategory(value Category) bool {
	return value == CategoryHarassment || value == CategoryHate || value == CategorySpam || value == CategoryCheating || value == CategoryInappropriateProfile || value == CategoryOther
}
func ValidSurface(value Surface) bool {
	return value == SurfaceTableChat || value == SurfaceTableReaction || value == SurfaceTableBehavior || value == SurfaceProfile || value == SurfaceRecentPlayer
}
