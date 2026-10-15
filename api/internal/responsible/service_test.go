package responsible

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var ctx = context.Background()

func newSvc(t *testing.T, now time.Time) (*Service, *MemoryStore) {
	t.Helper()
	store := NewMemoryStore()
	return NewService(store).WithClock(func() time.Time { return now }), store
}

func ready(t *testing.T, s *Service, user string) {
	t.Helper()
	if err := s.AcceptGambling(ctx, user); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLimits(ctx, user, 1000, 5000, 10000); err != nil {
		t.Fatal(err)
	}
}

func TestEligibilityRefusals(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	s, _ := newSvc(t, now)

	if err := s.Eligibility(ctx, "u", 100); !errors.Is(err, ErrConsentRequired) {
		t.Fatalf("no consent: %v", err)
	}
	if err := s.AcceptGambling(ctx, "u"); err != nil {
		t.Fatal(err)
	}
	if err := s.Eligibility(ctx, "u", 100); !errors.Is(err, ErrLimitsNotConfigured) {
		t.Fatalf("no limits: %v", err)
	}
	if _, err := s.SetLimits(ctx, "u", 1000, 5000, 10000); err != nil {
		t.Fatal(err)
	}
	if err := s.Eligibility(ctx, "u", 1000); err != nil {
		t.Fatalf("exactly at the limit must pass: %v", err)
	}
	var breach *BreachError
	if err := s.Eligibility(ctx, "u", 1001); !errors.As(err, &breach) || breach.Breach.Window != "daily" {
		t.Fatalf("over the daily limit: %v", err)
	}
	if _, err := s.SelfExclude(ctx, "u", "30d"); err != nil {
		t.Fatal(err)
	}
	var excluded *ExcludedError
	if err := s.Eligibility(ctx, "u", 100); !errors.As(err, &excluded) || excluded.Until == "" {
		t.Fatalf("self-excluded: %v", err)
	}
}

func TestAnExpiredExclusionNoLongerBlocks(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	s, _ := newSvc(t, now)
	ready(t, s, "u")
	if _, err := s.SelfExclude(ctx, "u", "30d"); err != nil {
		t.Fatal(err)
	}
	later := s.WithClock(func() time.Time { return now.Add(31 * 24 * time.Hour) })
	if err := later.Eligibility(ctx, "u", 100); err != nil {
		t.Fatalf("after the end of the exclusion: %v", err)
	}
}

func TestInflowIsGrossAndIdempotentPerKey(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	s, _ := newSvc(t, now)
	ready(t, s, "u")
	for range 3 { // a retry of the same buy-in reproduces the key
		if b, err := s.RecordInflow(ctx, "u", 600, "buyin-1"); err != nil || b != nil {
			t.Fatalf("breach=%v err=%v", b, err)
		}
	}
	st, _ := s.Status(ctx, "u")
	if st.Usage.Daily != 600 {
		t.Fatalf("one key counted %d times: daily=%d", st.Usage.Daily, st.Usage.Daily)
	}
	// A second, different buy-in that no longer fits is refused, counters unchanged.
	b, err := s.RecordInflow(ctx, "u", 600, "buyin-2")
	if err != nil || b == nil || b.Window != "daily" {
		t.Fatalf("breach=%v err=%v", b, err)
	}
	if st, _ = s.Status(ctx, "u"); st.Usage.Daily != 600 {
		t.Fatalf("a refused inflow was counted: %d", st.Usage.Daily)
	}
}

// Cashing out never frees headroom (old invariant 8): nothing in the package can
// lower a counter, so a cap cannot be churned around by cashing out and buying in.
func TestNoPathLowersAnInflowCounter(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(&Service{}), reflect.TypeOf((*Store)(nil)).Elem()} {
		for i := 0; i < typ.NumMethod(); i++ {
			name := strings.ToLower(typ.Method(i).Name)
			for _, bad := range []string{"release", "decrement", "refund", "cashout", "credit", "reset"} {
				if strings.Contains(name, bad) {
					t.Errorf("%s.%s could lower an inflow counter", typ, typ.Method(i).Name)
				}
			}
		}
	}
}

func TestLimitDecreaseAppliesAtOnceIncreaseWaits(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	s, _ := newSvc(t, now)
	ready(t, s, "u")

	lim, err := s.SetLimits(ctx, "u", 500, 5000, 10000) // daily down
	if err != nil || lim.Daily != 500 || lim.Pending != nil {
		t.Fatalf("decrease: %+v err=%v", lim, err)
	}
	lim, err = s.SetLimits(ctx, "u", 800, 5000, 10000) // daily up
	if err != nil || lim.Daily != 500 || lim.Pending == nil || lim.Pending.Daily != 800 {
		t.Fatalf("increase must wait: %+v err=%v", lim, err)
	}
	if got := lim.Pending.AppliesAt; got != now.Add(7*24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("daily/weekly increase cooldown = %s", got)
	}
	lim, _ = s.SetLimits(ctx, "u", 500, 5000, 20000) // monthly up
	if got := lim.Pending.AppliesAt; got != now.Add(14*24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("monthly increase cooldown = %s", got)
	}
	// The pending set takes effect once its time has come, without anyone touching it.
	later := s.WithClock(func() time.Time { return now.Add(15 * 24 * time.Hour) })
	st, _ := later.Status(ctx, "u")
	if st.Limits.Monthly != 20000 || st.Limits.Pending != nil {
		t.Fatalf("matured pending not applied: %+v", st.Limits)
	}
	cleared, err := s.CancelPendingLimits(ctx, "u")
	if err != nil || cleared.Pending != nil || cleared.Monthly != 10000 {
		t.Fatalf("cancel: %+v err=%v", cleared, err)
	}
}

func TestExclusionCanOnlyBeExtendedAndRevokedLate(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	s, _ := newSvc(t, now)
	ready(t, s, "u")
	if _, err := s.SelfExclude(ctx, "u", "90d"); err != nil {
		t.Fatal(err)
	}
	var rule *RuleError
	if _, err := s.SelfExclude(ctx, "u", "30d"); !errors.As(err, &rule) {
		t.Fatalf("a shorter exclusion must be refused: %v", err)
	}
	if err := s.RevokeSelfExclusion(ctx, "u"); !errors.As(err, &rule) {
		t.Fatalf("a fixed period is not revocable: %v", err)
	}
	if _, err := s.SelfExclude(ctx, "u", "indefinite"); err != nil {
		t.Fatalf("extending to indefinite: %v", err)
	}
	if err := s.RevokeSelfExclusion(ctx, "u"); !errors.As(err, &rule) {
		t.Fatalf("revoking before 90 days: %v", err)
	}
	later := s.WithClock(func() time.Time { return now.Add(91 * 24 * time.Hour) })
	if err := later.RevokeSelfExclusion(ctx, "u"); err != nil {
		t.Fatalf("revoking after 90 days: %v", err)
	}
}

func TestACarriedExclusionBindsANewAccountOfTheSamePerson(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	s := NewService(store).WithClock(func() time.Time { return now }).
		WithCPFHasher(func(_ context.Context, user string) (string, error) { return "hash-of-" + user[:1], nil })
	until := now.Add(40 * 24 * time.Hour).Format(time.RFC3339)
	if err := store.PutCarried(ctx, "hash-of-a", SelfExclusion{Period: "90d", Until: until}, now.Add(40*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	ready(t, s, "a-new-account")
	var excluded *ExcludedError
	if err := s.Eligibility(ctx, "a-new-account", 100); !errors.As(err, &excluded) || excluded.Until != until {
		t.Fatalf("carried exclusion not applied: %v", err)
	}
	// Another person is unaffected.
	ready(t, s, "b-someone-else")
	if err := s.Eligibility(ctx, "b-someone-else", 100); err != nil {
		t.Fatalf("unrelated player blocked: %v", err)
	}
	// An expired carried record is ignored.
	if err := store.PutCarried(ctx, "hash-of-c", SelfExclusion{Until: now.Add(-time.Hour).Format(time.RFC3339)}, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	ready(t, s, "c-expired")
	if err := s.Eligibility(ctx, "c-expired", 100); err != nil {
		t.Fatalf("expired carried record blocked: %v", err)
	}
}

func TestACPFLookupFailureFailsClosed(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	s := NewService(NewMemoryStore()).WithClock(func() time.Time { return now }).
		WithCPFHasher(func(context.Context, string) (string, error) { return "", errors.New("account down") })
	ready(t, s, "u")
	if err := s.Eligibility(ctx, "u", 100); err == nil {
		t.Fatal("an unverifiable carried exclusion must not let a buy-in through")
	}
}

func TestCarriedSelfExclusionRule(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	if _, _, ok := CarriedSelfExclusion(nil, now); ok {
		t.Fatal("nothing to carry")
	}
	if _, _, ok := CarriedSelfExclusion(&SelfExclusion{Until: now.Add(-time.Hour).Format(time.RFC3339)}, now); ok {
		t.Fatal("an ended exclusion carries nothing")
	}
	_, until, ok := CarriedSelfExclusion(&SelfExclusion{Until: now.Add(time.Hour).Format(time.RFC3339)}, now)
	if !ok || !until.Equal(now.Add(time.Hour).Truncate(time.Second)) {
		t.Fatalf("a timed exclusion is kept to its end: %v %v", until, ok)
	}
	_, until, ok = CarriedSelfExclusion(&SelfExclusion{Period: "indefinite"}, now)
	if !ok || !until.Equal(now.AddDate(ErasureRetentionYears, 0, 0)) {
		t.Fatalf("an indefinite one is kept the retention period: %v", until)
	}
	if _, _, ok := CarriedSelfExclusion(&SelfExclusion{Until: "garbage"}, now); !ok {
		t.Fatal("an unparseable end fails closed")
	}
}

func TestPurgeCarriesAnActiveExclusionKeyedByTheCPFHashAndOnlyThen(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	s := NewService(store).WithClock(func() time.Time { return now }).
		WithCPFHasher(func(_ context.Context, user string) (string, error) { return "hash-" + user, nil })

	ready(t, s, "excluded")
	if _, err := s.SelfExclude(ctx, "excluded", "90d"); err != nil {
		t.Fatal(err)
	}
	carried, err := s.CarryOnErase(ctx, "excluded")
	if err != nil || !carried {
		t.Fatalf("carried=%v err=%v", carried, err)
	}
	got, _ := store.GetCarried(ctx, "hash-excluded", now)
	if got == nil || got.Until != now.Add(90*24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("carried record = %+v", got)
	}

	ready(t, s, "free") // never excluded: nothing to carry
	if carried, err := s.CarryOnErase(ctx, "free"); err != nil || carried {
		t.Fatalf("free: carried=%v err=%v", carried, err)
	}
	ready(t, s, "ended")
	if _, err := s.SelfExclude(ctx, "ended", "30d"); err != nil {
		t.Fatal(err)
	}
	later := s.WithClock(func() time.Time { return now.Add(31 * 24 * time.Hour) })
	if carried, err := later.CarryOnErase(ctx, "ended"); err != nil || carried {
		t.Fatalf("an ended exclusion carries nothing: carried=%v err=%v", carried, err)
	}
}

func TestPurgeStopsWhenAnActiveExclusionCannotBeKeyed(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	s := NewService(store).WithClock(func() time.Time { return now })
	ready(t, s, "u")
	if _, err := s.SelfExclude(ctx, "u", "indefinite"); err != nil {
		t.Fatal(err)
	}
	// No CPF lookup at all: erasing would drop the exclusion silently.
	if _, err := s.CarryOnErase(ctx, "u"); err == nil {
		t.Fatal("must fail so the purge retries instead of erasing the exclusion")
	}
	failing := s.WithCPFHasher(func(context.Context, string) (string, error) { return "", errors.New("account down") })
	if _, err := failing.CarryOnErase(ctx, "u"); err == nil {
		t.Fatal("a failed lookup must fail the purge step")
	}
	// An account with no CPF on record has nothing to key by: nothing is carried, no error.
	noCPF := s.WithCPFHasher(func(context.Context, string) (string, error) { return "", nil })
	if carried, err := noCPF.CarryOnErase(ctx, "u"); err != nil || carried {
		t.Fatalf("carried=%v err=%v", carried, err)
	}
}
