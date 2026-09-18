package leaderboard

import (
	"fmt"
	"time"
)

// A board is scoped by period as well as by currency mode. PeriodAll is the
// lifetime board this package has always served; PeriodMonth is the calendar
// month the player is in right now — the one the web client makes default, so
// a player who started this week competes against this week's hands instead of
// against every hand ever dealt.
const (
	PeriodAll   = "all"
	PeriodMonth = "month"
)

// brt is the wall clock a month turns on, matching dailyreward's cooldown key
// (dailyreward/store.go). In UTC the month would roll at 21:00 of the previous
// day for every player, so a hand played at 22:00 on the last day of the month
// would already count for the next one.
var brt = time.FixedZone("BRT", -3*60*60)

// MonthKey is the bucket a moment falls in, "2026-09".
func MonthKey(now time.Time) string { return now.In(brt).Format("2006-01") }

// normalizePeriod defaults an empty period to the lifetime board — the public
// API's historical behaviour, which the CLI and the mobile client still rely
// on — and rejects anything else outright rather than silently serving a board
// the caller did not ask for.
func normalizePeriod(period string) (string, error) {
	if period == "" {
		period = PeriodAll
	}
	if period != PeriodAll && period != PeriodMonth {
		return "", fmt.Errorf("leaderboard: unsupported period %q", period)
	}
	return period, nil
}

// periodKey resolves a validated period to the bucket key that scopes the
// stored row. "" is the lifetime board: it keeps the exact sort key and GSI
// partition value rows were written with before periods existed, so no
// backfill or migration is involved.
func periodKey(period string, now time.Time) string {
	if period == PeriodMonth {
		return MonthKey(now)
	}
	return ""
}

// boardKey is the GSI partition every row of one board shares. Scoping by
// period is deliberately expressed here, in the partition value, rather than
// in a new index: the three existing GSIs rank a monthly board exactly as they
// rank the lifetime one, and each closed month stays queryable forever.
func boardKey(mode, periodKey string) string {
	if periodKey == "" {
		return mode
	}
	return mode + "#" + periodKey
}

// statsSKFor is the sort key of one player's row on one board.
func statsSKFor(mode, periodKey string) string {
	if periodKey == "" {
		return statsSK + "#" + mode
	}
	return statsSK + "#" + mode + "#" + periodKey
}
