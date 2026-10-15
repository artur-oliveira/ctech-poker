package responsible

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Refusals a real-money buy-in (or a settings change) can meet. Each is a
// decision about the player, not a failure of the store.
var (
	// ErrConsentRequired: the player has not accepted the current gambling addendum.
	ErrConsentRequired = errors.New("responsible: gambling addendum not accepted")
	// ErrLimitsNotConfigured: a player must set all three limits before real-money play.
	ErrLimitsNotConfigured = errors.New("responsible: personal limits not configured")
)

// ExcludedError: the player is self-excluded. Until is empty for an indefinite exclusion.
type ExcludedError struct{ Until string }

func (e *ExcludedError) Error() string { return "responsible: self-excluded" }

// BreachError: the amount would overflow one of the personal limits.
type BreachError struct{ Breach LimitBreach }

func (e *BreachError) Error() string {
	return fmt.Sprintf("responsible: %s limit exceeded", e.Breach.Window)
}

// RuleError: a change the rules refuse (shortening an exclusion, revoking early, bad limits).
type RuleError struct{ Message string }

func (e *RuleError) Error() string { return e.Message }

const maxAttempts = 5

// CPFHasher returns ctech-account's keyed hash of the player's CPF (the key the
// carried exclusions use). Optional: without it the carried check is skipped.
type CPFHasher func(ctx context.Context, userID string) (string, error)

// Service is the responsible-gambling use cases over a Store.
type Service struct {
	store Store
	cpf   CPFHasher
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

// WithCPFHasher turns on the carried-exclusion check (a new account of someone who
// self-excluded and deleted the old one is excluded until the same date).
func (s *Service) WithCPFHasher(h CPFHasher) *Service { c := *s; c.cpf = h; return &c }

// WithClock replaces the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { c := *s; c.now = now; return &c }

// update runs fn on the player's state and stores the result, retrying when a
// concurrent writer got there first.
func (s *Service) update(ctx context.Context, userID string, fn func(st *State, now time.Time) error) (*State, error) {
	for range maxAttempts {
		st, err := s.store.Get(ctx, userID)
		if err != nil {
			return nil, err
		}
		if st == nil {
			st = &State{UserID: userID}
		}
		prev := st.Version
		if err := fn(st, s.now()); err != nil {
			return nil, err
		}
		switch err := s.store.Put(ctx, st, prev); {
		case err == nil:
			return st, nil
		case !errors.Is(err, ErrConflict):
			return nil, err
		}
	}
	return nil, ErrConflict
}

// AcceptGambling records consent to the current addendum.
func (s *Service) AcceptGambling(ctx context.Context, userID string) error {
	_, err := s.update(ctx, userID, func(st *State, now time.Time) error {
		st.GamblingAddendumVersion = CurrentGamblingAddendumVersion
		st.GamblingAcceptedAt = now.UTC().Format(time.RFC3339)
		return nil
	})
	if err != nil {
		return err
	}
	return s.store.Audit(ctx, userID, "gambling_accepted", CurrentGamblingAddendumVersion, s.now())
}

// SelfExclude records a self-exclusion. Extension-only: a new exclusion may never
// end earlier than the one it replaces, and nothing extends an indefinite one.
func (s *Service) SelfExclude(ctx context.Context, userID, period string) (*SelfExclusion, error) {
	var out *SelfExclusion
	_, err := s.update(ctx, userID, func(st *State, now time.Time) error {
		until, err := ExclusionUntil(period, now)
		if err != nil {
			return &RuleError{Message: err.Error()}
		}
		if st.SelfExclusion != nil && st.SelfExcluded(now) {
			cur := st.SelfExclusion
			if cur.Until == "" {
				return &RuleError{Message: "already self-excluded indefinitely"}
			}
			if until != "" {
				curUntil, err := time.Parse(time.RFC3339, cur.Until)
				newUntil, err2 := time.Parse(time.RFC3339, until)
				if err != nil || err2 != nil || !newUntil.After(curUntil) {
					return &RuleError{Message: "a self-exclusion can only be extended, never shortened"}
				}
			}
		}
		out = &SelfExclusion{Period: period, RequestedAt: now.Format(time.RFC3339), Until: until}
		st.SelfExclusion = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, s.store.Audit(ctx, userID, "self_excluded", period, s.now())
}

// RevokeSelfExclusion lifts an indefinite exclusion after its 90-day floor. Fixed
// periods are not revocable: they expire on their own.
func (s *Service) RevokeSelfExclusion(ctx context.Context, userID string) error {
	_, err := s.update(ctx, userID, func(st *State, now time.Time) error {
		if st.SelfExclusion == nil || !st.SelfExcluded(now) {
			return &RuleError{Message: "no active self-exclusion"}
		}
		if st.SelfExclusion.Until != "" {
			return &RuleError{Message: "a fixed-period self-exclusion expires on its own and cannot be revoked"}
		}
		requested, err := time.Parse(time.RFC3339, st.SelfExclusion.RequestedAt)
		if err != nil || now.Sub(requested) < exclusionRevokeFloor {
			return &RuleError{Message: "an indefinite self-exclusion can only be revoked after 90 days"}
		}
		st.SelfExclusion = nil
		return nil
	})
	if err != nil {
		return err
	}
	return s.store.Audit(ctx, userID, "self_exclusion_revoked", "", s.now())
}

// SetLimits stores new limits. The first configuration and decreases apply at
// once; any increase waits out its cooldown as a pending set (7 days; 14 when the
// monthly limit grows). One pending set at a time: a new call replaces it.
func (s *Service) SetLimits(ctx context.Context, userID string, daily, weekly, monthly int64) (*GameLimits, error) {
	if err := ValidateLimits(daily, weekly, monthly); err != nil {
		return nil, &RuleError{Message: err.Error()}
	}
	var out *GameLimits
	_, err := s.update(ctx, userID, func(st *State, now time.Time) error {
		cur, matured := st.EffectiveGameLimits(now)
		if matured { // promote before diffing so cooldowns compare against reality
			promoted := cur
			st.GameLimits = &promoted
		}
		next := GameLimits{Daily: daily, Weekly: weekly, Monthly: monthly}
		if st.LimitsConfigured() || matured {
			applied := GameLimits{Daily: min(daily, cur.Daily), Weekly: min(weekly, cur.Weekly), Monthly: min(monthly, cur.Monthly)}
			if daily > cur.Daily || weekly > cur.Weekly || monthly > cur.Monthly {
				cooldown := cooldownIncrease
				if monthly > cur.Monthly {
					cooldown = cooldownMonthly
				}
				applied.Pending = &PendingLimits{Daily: daily, Weekly: weekly, Monthly: monthly, AppliesAt: now.Add(cooldown).Format(time.RFC3339)}
			}
			next = applied
		}
		st.GameLimits = &next
		out = &next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, s.store.Audit(ctx, userID, "limits_changed", fmt.Sprintf("d=%d w=%d m=%d", daily, weekly, monthly), s.now())
}

// CancelPendingLimits drops a scheduled increase, keeping the stricter current limits.
func (s *Service) CancelPendingLimits(ctx context.Context, userID string) (*GameLimits, error) {
	var out *GameLimits
	_, err := s.update(ctx, userID, func(st *State, now time.Time) error {
		if st.GameLimits == nil {
			return ErrLimitsNotConfigured
		}
		if lim, matured := st.EffectiveGameLimits(now); matured {
			st.GameLimits = &lim // it already applied: promote, nothing left to cancel
		} else {
			cleared := *st.GameLimits
			cleared.Pending = nil
			st.GameLimits = &cleared
		}
		cp := *st.GameLimits
		out = &cp
		return nil
	})
	return out, err
}

// Usage is the current-window inflow and when each window resets.
type Usage struct {
	Daily, Weekly, Monthly                   int64
	DayResetsAt, WeekResetsAt, MonthResetsAt string
}

// Status is what the player's settings page shows.
type Status struct {
	Accepted bool
	Limits   *GameLimits
	Usage    Usage
	Excluded *SelfExclusion
}

// Status reads the player's state at now. Read-only: a matured pending increase is
// reported as applied and promoted by the next write.
func (s *Service) Status(ctx context.Context, userID string) (*Status, error) {
	st, err := s.store.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := &Status{}
	var counters GameDepositCounters
	if st != nil {
		out.Accepted = st.GamblingAccepted()
		if st.SelfExcluded(now) {
			out.Excluded = st.SelfExclusion
		}
		if st.GameLimits != nil {
			lim, _ := st.EffectiveGameLimits(now)
			out.Limits = &lim
		}
		if st.GameDepositCounters != nil {
			counters = *st.GameDepositCounters
		}
	}
	day, week, month := WindowKeys(now)
	d, w, m := counters.SumsFor(day, week, month)
	dr, wr, mr := WindowResets(now)
	out.Usage = Usage{Daily: d, Weekly: w, Monthly: m,
		DayResetsAt: dr.Format(time.RFC3339), WeekResetsAt: wr.Format(time.RFC3339), MonthResetsAt: mr.Format(time.RFC3339)}
	return out, nil
}

// Eligibility says whether the player may put amount centavos into a real-money
// table now. Read-only except for the one-time carried-exclusion lookup. The
// authoritative metering is RecordInflow; this refuses early, before any hold.
func (s *Service) Eligibility(ctx context.Context, userID string, amount int64) error {
	st, err := s.applyCarried(ctx, userID)
	if err != nil {
		return err
	}
	now := s.now()
	if !st.GamblingAccepted() {
		return ErrConsentRequired
	}
	if st.SelfExcluded(now) {
		return &ExcludedError{Until: st.SelfExclusion.Until}
	}
	if !st.LimitsConfigured() {
		return ErrLimitsNotConfigured
	}
	lim, _ := st.EffectiveGameLimits(now)
	var c GameDepositCounters
	if st.GameDepositCounters != nil {
		c = *st.GameDepositCounters
	}
	if b := CheckDeposit(lim, c, amount, now); b != nil {
		return &BreachError{Breach: *b}
	}
	return nil
}

// applyCarried returns the player's state (never nil), first copying onto it an
// exclusion carried over from an erased account of the same CPF. Done once per
// player; a lookup error fails closed rather than skipping the check.
func (s *Service) applyCarried(ctx context.Context, userID string) (*State, error) {
	st, err := s.store.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if st == nil {
		st = &State{UserID: userID}
	}
	if s.cpf == nil || st.CarriedChecked {
		return st, nil
	}
	hash, err := s.cpf(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("responsible: carried exclusion lookup: %w", err)
	}
	var carried *SelfExclusion
	if hash != "" {
		if carried, err = s.store.GetCarried(ctx, hash, s.now()); err != nil {
			return nil, err
		}
	}
	updated, err := s.update(ctx, userID, func(cur *State, now time.Time) error {
		cur.CarriedChecked = true
		if carried != nil && !(cur.SelfExcluded(now) && lastsLonger(cur.SelfExclusion, carried)) {
			ex := *carried
			cur.SelfExclusion = &ex
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if carried != nil {
		_ = s.store.Audit(ctx, userID, "self_exclusion_carried", carried.Until, s.now())
	}
	return updated, nil
}

// lastsLonger reports whether current ends no earlier than candidate.
func lastsLonger(current, candidate *SelfExclusion) bool {
	if current == nil {
		return false
	}
	if current.Until == "" {
		return true
	}
	if candidate.Until == "" {
		return false
	}
	cur, err1 := time.Parse(time.RFC3339, current.Until)
	cand, err2 := time.Parse(time.RFC3339, candidate.Until)
	return err1 == nil && err2 == nil && !cand.After(cur)
}

// CarryOnErase keeps an active self-exclusion past the erasure of the account that
// made it, keyed by the CPF keyed hash, so deleting an account is not a way out of
// an exclusion. Called by the account-deletion purge BEFORE it erases the state
// (carry first, erase second). A player with nothing to carry, or no CPF hash to key
// it by, carries nothing; a failed lookup is an error so the purge retries.
func (s *Service) CarryOnErase(ctx context.Context, userID string) (bool, error) {
	st, err := s.store.Get(ctx, userID)
	if err != nil || st == nil || st.SelfExclusion == nil {
		return false, err
	}
	ex, until, ok := CarriedSelfExclusion(st.SelfExclusion, s.now())
	if !ok {
		return false, nil
	}
	if s.cpf == nil {
		return false, errors.New("responsible: an active self-exclusion cannot be carried without the CPF hash lookup")
	}
	hash, err := s.cpf(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("responsible: carried exclusion key: %w", err)
	}
	if hash == "" {
		return false, nil
	}
	if err := s.store.PutCarried(ctx, hash, ex, until); err != nil {
		return false, err
	}
	return true, nil
}

// RecordInflow adds amount to the player's window counters, once per key (a retry
// of the same buy-in reproduces the key and counts nothing more). It re-checks the
// limits against the freshest counters, so two buy-ins racing past Eligibility
// cannot both fit: the loser gets the breach and the caller gives the hold back.
// Gross inflow by design: nothing here ever lowers a counter.
func (s *Service) RecordInflow(ctx context.Context, userID string, amount int64, key string) (*LimitBreach, error) {
	for range maxAttempts {
		// A retry of an inflow already counted must not meet the limit it helped fill.
		if counted, err := s.store.Counted(ctx, userID, key); err != nil {
			return nil, err
		} else if counted {
			return nil, nil
		}
		st, err := s.store.Get(ctx, userID)
		if err != nil {
			return nil, err
		}
		if st == nil {
			st = &State{UserID: userID}
		}
		prev := st.Version
		now := s.now()
		if !st.LimitsConfigured() {
			return nil, ErrLimitsNotConfigured
		}
		lim, matured := st.EffectiveGameLimits(now)
		if matured {
			promoted := lim
			st.GameLimits = &promoted
		}
		var cur GameDepositCounters
		if st.GameDepositCounters != nil {
			cur = *st.GameDepositCounters
		}
		if b := CheckDeposit(lim, cur, amount, now); b != nil {
			return b, nil
		}
		day, week, month := WindowKeys(now)
		d, w, m := cur.SumsFor(day, week, month)
		st.GameDepositCounters = &GameDepositCounters{
			DayKey: day, DaySum: d + amount, WeekKey: week, WeekSum: w + amount, MonthKey: month, MonthSum: m + amount,
		}
		_, err = s.store.CommitInflow(ctx, st, prev, key, now)
		if err == nil {
			return nil, nil
		}
		if !errors.Is(err, ErrConflict) {
			return nil, err
		}
	}
	return nil, ErrConflict
}
