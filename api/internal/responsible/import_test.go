package responsible

import (
	"context"
	"testing"
	"time"
)

type fakeExporter struct{ pages []*ExportPage }

func (f *fakeExporter) ResponsibleExport(_ context.Context, cursor string) (*ExportPage, error) {
	i := 0
	if cursor != "" {
		for n := range f.pages {
			if f.pages[n].NextCursor == cursor {
				i = n + 1
			}
		}
	}
	return f.pages[i], nil
}

func TestImportCopiesEverythingVerbatimAndIsHarmlessTwice(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	pending := &PendingLimits{Daily: 900, Weekly: 5000, Monthly: 10000, AppliesAt: "2026-10-20T10:00:00Z"}
	src := &fakeExporter{pages: []*ExportPage{
		{NextCursor: "p2", Users: []ExportedUser{{
			UserID: "excluded", GamblingAddendumVersion: "2.1", GamblingActivatedAt: "2026-09-01T10:00:00Z",
			SelfExclusion: &SelfExclusion{Period: "90d", RequestedAt: "2026-10-01T10:00:00Z", Until: "2026-12-30T10:00:00Z"},
		}}},
		{Users: []ExportedUser{
			{UserID: "pending", GamblingAddendumVersion: "2.1", GamblingActivatedAt: "2026-09-02T10:00:00Z",
				GameLimits:          &GameLimits{Daily: 500, Weekly: 5000, Monthly: 10000, Pending: pending},
				GameDepositCounters: &GameDepositCounters{DayKey: "2026-10-15", DaySum: 300}},
			{UserID: "never-consented"},
		}, CarriedExclusions: []CarriedExclusion{{CPFHMAC: "h1", SelfExclusion: SelfExclusion{Period: "indefinite"}, ExpiresAt: now.AddDate(5, 0, 0).Unix()}}},
	}}
	store := NewMemoryStore()

	counts, err := Import(ctx, src, store, false, now)
	if err != nil || counts.Users != 3 || counts.Imported != 3 || counts.Carried != 1 {
		t.Fatalf("counts=%+v err=%v", counts, err)
	}
	ex, _ := store.Get(ctx, "excluded")
	if ex.SelfExclusion.Until != "2026-12-30T10:00:00Z" || ex.GamblingAddendumVersion != "2.1" || ex.GamblingAcceptedAt != "2026-09-01T10:00:00Z" {
		t.Fatalf("exclusion or consent changed: %+v", ex)
	}
	pd, _ := store.Get(ctx, "pending")
	if pd.GameLimits.Pending == nil || *pd.GameLimits.Pending != *pending || pd.GameDepositCounters.DaySum != 300 {
		t.Fatalf("pending increase or counters changed: %+v", pd)
	}
	nc, _ := store.Get(ctx, "never-consented")
	if nc.GamblingAccepted() {
		t.Fatal("a player who never consented must stay unconsented")
	}
	if got, _ := store.GetCarried(ctx, "h1", now); got == nil {
		t.Fatal("the carried exclusion was dropped by the move")
	}

	// Meanwhile the player changed something in Poker: a second run must not undo it.
	svc := NewService(store).WithClock(func() time.Time { return now })
	if _, err := svc.SetLimits(ctx, "pending", 100, 5000, 10000); err != nil {
		t.Fatal(err)
	}
	again, err := Import(ctx, src, store, false, now)
	if err != nil || again.Imported != 0 || again.AlreadyThere != 3 {
		t.Fatalf("second run=%+v err=%v", again, err)
	}
	if after, _ := store.Get(ctx, "pending"); after.GameLimits.Daily != 100 {
		t.Fatalf("the second run overwrote a newer change: %+v", after.GameLimits)
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	src := &fakeExporter{pages: []*ExportPage{{Users: []ExportedUser{{UserID: "u", GamblingAddendumVersion: "2.1"}}}}}
	store := NewMemoryStore()
	counts, err := Import(ctx, src, store, true, time.Now())
	if err != nil || counts.Users != 1 || counts.Imported != 0 {
		t.Fatalf("counts=%+v err=%v", counts, err)
	}
	if st, _ := store.Get(ctx, "u"); st != nil {
		t.Fatal("a dry run wrote a player")
	}
}
