// Package pokerbot contains the local, server-side policy used by automated
// sandbox seats. It deliberately knows nothing about transport or persistence:
// callers provide only the bot's private strength estimate and public table
// context, then validate the returned move through hand.Table as usual.
package pokerbot

import (
	"fmt"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type Random interface {
	Float64() float64
	IntN(n int) int
}

type Profile struct {
	ID                string
	VPIP              float64
	Aggression        float64
	Bluff             float64
	CallTolerance     float64
	Trap              float64
	Tempo             float64
	RevealOne         float64
	RevealBoth        float64
	AllInCashOut      float64
	SessionCashOut    float64
	PreferSmallSizing bool
}

var Profiles = []Profile{
	{ID: "tight_passive", VPIP: .24, Aggression: .22, Bluff: .03, CallTolerance: .34, Tempo: 1.12, RevealOne: .04, RevealBoth: .03, AllInCashOut: .76, SessionCashOut: .05},
	{ID: "tag", VPIP: .31, Aggression: .62, Bluff: .10, CallTolerance: .46, Tempo: .95, RevealOne: .03, RevealBoth: .05, AllInCashOut: .68, SessionCashOut: .04},
	{ID: "lag", VPIP: .52, Aggression: .76, Bluff: .22, CallTolerance: .56, Tempo: .86, RevealOne: .08, RevealBoth: .08, AllInCashOut: .65, SessionCashOut: .03},
	{ID: "calling_station", VPIP: .64, Aggression: .16, Bluff: .02, CallTolerance: .78, Tempo: .82, RevealOne: .05, RevealBoth: .04, AllInCashOut: .82, SessionCashOut: .06},
	{ID: "small_ball", VPIP: .47, Aggression: .49, Bluff: .15, CallTolerance: .54, Tempo: .92, RevealOne: .06, RevealBoth: .05, AllInCashOut: .72, SessionCashOut: .04, PreferSmallSizing: true},
	{ID: "trapper", VPIP: .35, Aggression: .43, Bluff: .08, CallTolerance: .58, Trap: .48, Tempo: 1.18, RevealOne: .04, RevealBoth: .07, AllInCashOut: .79, SessionCashOut: .05},
	{ID: "short_stack", VPIP: .38, Aggression: .71, Bluff: .09, CallTolerance: .42, Tempo: .78, RevealOne: .02, RevealBoth: .04, AllInCashOut: .85, SessionCashOut: .07},
	{ID: "volatile", VPIP: .61, Aggression: .84, Bluff: .28, CallTolerance: .61, Tempo: 1.06, RevealOne: .12, RevealBoth: .10, AllInCashOut: .69, SessionCashOut: .08},
}

func PickProfile(r Random) Profile { return Profiles[r.IntN(len(Profiles))] }

type Context struct {
	Legal    *hand.LegalActions
	Strength float64 // caller-owned estimate in [0,1]
	Pot      int64
	Stack    int64
}

type Decision struct {
	Action string
	Amount int64
}

func Decide(p Profile, c Context, r Random) (Decision, error) {
	if c.Legal == nil || len(c.Legal.Actions) == 0 {
		return Decision{}, fmt.Errorf("pokerbot: no legal action")
	}
	legal := make(map[string]bool, len(c.Legal.Actions))
	for _, action := range c.Legal.Actions {
		legal[action] = true
	}
	strength := clamp(c.Strength)

	if legal["raise"] {
		raiseChance := p.Aggression*(.18+.72*strength) + p.Bluff*(1-strength)*.35
		if strength > .78 && r.Float64() < p.Trap {
			raiseChance *= .25
		}
		if r.Float64() < clamp(raiseChance) {
			return Decision{Action: "raise", Amount: raiseAmount(p, c.Legal, r)}, nil
		}
	}
	if legal["check"] {
		return Decision{Action: "check"}, nil
	}
	if legal["call"] {
		price := float64(c.Legal.CallAmount) / float64(max64(1, c.Pot+c.Legal.CallAmount))
		continueChance := p.CallTolerance + p.VPIP*.2 + strength*.55 - price*.8
		if r.Float64() < clamp(continueChance) {
			return Decision{Action: "call"}, nil
		}
	}
	if legal["fold"] {
		return Decision{Action: "fold"}, nil
	}
	return Decision{Action: c.Legal.Actions[0]}, nil
}

func raiseAmount(p Profile, legal *hand.LegalActions, r Random) int64 {
	options := []int64{legal.OneThirdPotRaiseTo, legal.HalfPotRaiseTo, legal.TwoThirdsPotRaiseTo, legal.PotRaiseTo}
	start := 0
	if !p.PreferSmallSizing {
		start = 1
	}
	for attempts := 0; attempts < len(options); attempts++ {
		amount := options[(start+r.IntN(len(options)-start))%len(options)]
		if amount >= legal.MinRaiseTo && amount <= legal.MaxRaiseTo {
			return amount
		}
	}
	return legal.MinRaiseTo
}

type ThinkKind uint8

const (
	ThinkPreselected ThinkKind = iota
	ThinkRoutine
	ThinkNormal
	ThinkLarge
	ThinkHesitation
)

var thinkRanges = map[ThinkKind][2]time.Duration{
	ThinkPreselected: {150 * time.Millisecond, 450 * time.Millisecond},
	ThinkRoutine:     {650 * time.Millisecond, 1800 * time.Millisecond},
	ThinkNormal:      {1200 * time.Millisecond, 4 * time.Second},
	ThinkLarge:       {2500 * time.Millisecond, 7500 * time.Millisecond},
	ThinkHesitation:  {6 * time.Second, 11 * time.Second},
}

// ThinkDelay depends on public complexity and profile tempo, never cards or
// the chosen action, so elapsed time does not become a hand-strength oracle.
func ThinkDelay(p Profile, kind ThinkKind, deadlineRemaining time.Duration, r Random) time.Duration {
	span, ok := thinkRanges[kind]
	if !ok {
		span = thinkRanges[ThinkNormal]
	}
	base := span[0] + time.Duration(r.Float64()*float64(span[1]-span[0]))
	delay := time.Duration(float64(base) * p.Tempo)
	latest := deadlineRemaining - 1500*time.Millisecond
	if latest < 100*time.Millisecond {
		latest = 100 * time.Millisecond
	}
	if delay > latest {
		delay = latest
	}
	return delay
}

type Reveal uint8

const (
	Muck Reveal = iota
	RevealOne
	RevealBoth
)

func RevealChoice(p Profile, r Random) Reveal {
	x := r.Float64()
	if x < p.RevealBoth {
		return RevealBoth
	}
	if x < p.RevealBoth+p.RevealOne {
		return RevealOne
	}
	return Muck
}

type ExitContext struct {
	WonAllIn      bool
	HitAndRun     bool
	StackTooShort bool
}

func ShouldCashOut(p Profile, c ExitContext, r Random) bool {
	if c.StackTooShort {
		return true
	}
	if c.WonAllIn && c.HitAndRun {
		return true
	}
	chance := p.SessionCashOut
	if c.WonAllIn {
		chance = maxFloat(chance, p.AllInCashOut)
	}
	return r.Float64() < clamp(chance)
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
