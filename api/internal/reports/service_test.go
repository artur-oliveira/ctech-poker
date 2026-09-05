package reports

import (
	"context"
	"strings"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type memoryStore struct {
	report Report
	calls  int
}

func (s *memoryStore) Create(_ context.Context, report Report, _ string) (*Report, bool, error) {
	s.calls++
	report.ReportID, report.StorageKey = "report-id", "reporter#report-id"
	s.report = report
	return &report, s.calls == 1, nil
}
func (s *memoryStore) Get(context.Context, string, string) (*Report, error) { return &s.report, nil }
func (s *memoryStore) ListByStatus(context.Context, Status, string, int) (Page, error) {
	return Page{}, nil
}
func (s *memoryStore) ListByReporter(context.Context, string, string, int) (Page, error) {
	return Page{}, nil
}
func (s *memoryStore) SetStatus(context.Context, string, string, Status, Resolution, string) error {
	return nil
}

type fakeActions struct{ entry *tablestore.ActionLogEntry }

func (f fakeActions) FindActionByID(context.Context, string, string, string) (*tablestore.ActionLogEntry, error) {
	return f.entry, nil
}

type fakePlayers struct{ exists bool }

func (f fakePlayers) Get(_ context.Context, id string) (*player.PlayerProfile, error) {
	if !f.exists {
		return nil, nil
	}
	return &player.PlayerProfile{UserID: id}, nil
}

func TestCreateCopiesAuthoritativeChatEvidenceAndEmitsMetricOnce(t *testing.T) {
	store := &memoryStore{}
	metrics := 0
	svc := NewService(store, fakeActions{entry: &tablestore.ActionLogEntry{
		TableID: "table", HandID: "hand", ActionID: "action", PlayerID: "target", Action: "chat", Message: "sanitized message",
	}}, fakePlayers{exists: true}).WithMetric(func(Category, Surface) { metrics++ })
	input := CreateInput{TargetPlayerID: "target", Category: CategoryHarassment, Surface: SurfaceTableChat, TableID: "table", HandID: "hand", ActionID: "action", Details: " context "}

	report, err := svc.Create(context.Background(), "reporter", "idem", input)
	if err != nil {
		t.Fatal(err)
	}
	if report.EvidenceMessage != "sanitized message" || report.Details != "context" {
		t.Fatalf("report=%+v", report)
	}
	if _, err := svc.Create(context.Background(), "reporter", "idem", input); err != nil {
		t.Fatal(err)
	}
	if metrics != 1 {
		t.Fatalf("metrics=%d want=1", metrics)
	}
}

func TestCreateRejectsClientEvidenceMismatchAndOversizedDetails(t *testing.T) {
	actions := fakeActions{entry: &tablestore.ActionLogEntry{TableID: "table", HandID: "hand", ActionID: "action", PlayerID: "someone-else", Action: "chat", Message: "message"}}
	svc := NewService(&memoryStore{}, actions, fakePlayers{exists: true})
	_, err := svc.Create(context.Background(), "reporter", "idem", CreateInput{TargetPlayerID: "target", Category: CategoryHate, Surface: SurfaceTableChat, TableID: "table", HandID: "hand", ActionID: "action"})
	if err != ErrEvidenceMissing {
		t.Fatalf("got %v", err)
	}
	_, err = svc.Create(context.Background(), "reporter", "idem", CreateInput{TargetPlayerID: "target", Category: CategoryOther, Surface: SurfaceProfile, Details: strings.Repeat("á", MaxDetailsRunes+1)})
	if err != ErrInvalidReport {
		t.Fatalf("got %v", err)
	}
}

func TestCreateReactionCopiesCatalogIDFromAction(t *testing.T) {
	store := &memoryStore{}
	svc := NewService(store, fakeActions{entry: &tablestore.ActionLogEntry{TableID: "t", HandID: "h", ActionID: "a", PlayerID: "target", Action: "reaction", ReactionID: "cold"}}, fakePlayers{exists: true})
	report, err := svc.Create(context.Background(), "reporter", "idem", CreateInput{TargetPlayerID: "target", Category: CategorySpam, Surface: SurfaceTableReaction, TableID: "t", HandID: "h", ActionID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReactionID != "cold" || report.EvidenceMessage != "" {
		t.Fatalf("report=%+v", report)
	}
}
