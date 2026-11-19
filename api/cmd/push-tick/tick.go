package main

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/api-commons/pushsvc"
	"gopkg.aoctech.app/poker/api/internal/pokerpush"
)

const (
	// pageSize is one read of a slot's reminders.
	pageSize = 100
	// tickWorkers is how many reminders are decided at once.
	tickWorkers = 8
)

// Reminders is what the tick needs of pushsvc.ReminderStore.
type Reminders interface {
	ListSlot(ctx context.Context, slot, limit int, cursor map[string]types.AttributeValue) ([]pushsvc.Reminder, map[string]types.AttributeValue, error)
	MarkSent(ctx context.Context, sub, kind, day string) (bool, error)
	Put(ctx context.Context, r pushsvc.Reminder) error
}

// Devices lists a person's registered devices (pushsvc.Store).
type Devices interface {
	List(ctx context.Context, sub string) ([]pushsvc.Subscription, error)
}

// Rules is pokerpush.Rules.
type Rules interface {
	Decide(ctx context.Context, now time.Time, rem pushsvc.Reminder) (*pushsvc.Job, error)
}

// Sender hands a job to the push-sender Lambda (pushsvc.Invoker).
type Sender interface {
	Send(ctx context.Context, job pushsvc.Job) error
}

// Habits reads the start times of a player's newest sessions.
type Habits interface {
	RecentStarts(ctx context.Context, playerID string, n int) ([]time.Time, error)
}

// Tick runs every 15 minutes: it reads the reminders due in this slot (and the one before, to recover a skipped run), asks
// the rules whether each goes out, marks the day, and hands the push to the sender.
type Tick struct {
	reminders Reminders
	devices   Devices
	rules     Rules
	sender    Sender
	habits    Habits
}

// NewTick returns a Tick.
func NewTick(r Reminders, d Devices, rules Rules, s Sender, h Habits) *Tick {
	return &Tick{reminders: r, devices: d, rules: rules, sender: s, habits: h}
}

// Run processes the slots of `now`. A failure of one person is logged and counted, never stops the others; only a failure
// to read the slot itself returns an error, so the scheduler retries the run.
func (t *Tick) Run(ctx context.Context, now time.Time) error {
	var sent, skipped, failed atomic.Int64
	for _, slot := range pushsvc.SlotsToProcess(now) {
		var cursor map[string]types.AttributeValue
		for {
			page, next, err := t.reminders.ListSlot(ctx, slot, pageSize, cursor)
			if err != nil {
				return err
			}
			var wg sync.WaitGroup
			sem := make(chan struct{}, tickWorkers)
			for _, rem := range page {
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					switch t.one(ctx, now, rem) {
					case outcomeSent:
						sent.Add(1)
					case outcomeFailed:
						failed.Add(1)
					default:
						skipped.Add(1)
					}
				}()
			}
			wg.Wait()
			if next == nil {
				break
			}
			cursor = next
		}
	}
	slog.InfoContext(ctx, "push tick finished", "slots", pushsvc.SlotsToProcess(now), "sent", sent.Load(), "skipped", skipped.Load(), "failed", failed.Load())
	return nil
}

type outcome int

const (
	outcomeSkipped outcome = iota
	outcomeSent
	outcomeFailed
)

func (t *Tick) one(ctx context.Context, now time.Time, rem pushsvc.Reminder) outcome {
	log := slog.With("kind", rem.Kind, "slot", rem.Slot)
	devices, err := t.devices.List(ctx, rem.Sub)
	if err != nil {
		log.WarnContext(ctx, "push tick: devices not read", "error", err)
		return outcomeFailed
	}
	if len(devices) == 0 {
		return outcomeSkipped // nothing to send to, and the day stays open
	}
	job, err := t.rules.Decide(ctx, now, rem)
	if err != nil {
		log.WarnContext(ctx, "push tick: rule failed", "error", err)
		return outcomeFailed
	}
	if job == nil {
		return outcomeSkipped
	}
	// Mark first: a lost invoke is a push not sent, never one sent twice.
	won, err := t.reminders.MarkSent(ctx, rem.Sub, rem.Kind, pushsvc.Today(now))
	if err != nil {
		log.WarnContext(ctx, "push tick: day not marked", "error", err)
		return outcomeFailed
	}
	if !won {
		return outcomeSkipped
	}
	if err := t.sender.Send(ctx, *job); err != nil {
		log.ErrorContext(ctx, "push tick: sender not invoked; the day stays marked", "error", err)
		return outcomeFailed
	}
	if rem.Kind == pokerpush.KindPlay {
		t.relearn(ctx, rem)
	}
	return outcomeSent
}

// relearn moves a "come play" reminder to the hour the player usually starts a session. Best effort: a failure here
// keeps the old hour.
func (t *Tick) relearn(ctx context.Context, rem pushsvc.Reminder) {
	starts, err := t.habits.RecentStarts(ctx, rem.Sub, 14)
	if err != nil {
		slog.WarnContext(ctx, "push tick: sessions not read", "error", err)
		return
	}
	slot := pokerpush.TypicalSlot(starts)
	if slot == rem.Slot {
		return
	}
	if err := t.reminders.Put(ctx, pushsvc.Reminder{Sub: rem.Sub, Kind: rem.Kind, Enabled: true, Slot: slot}); err != nil {
		slog.WarnContext(ctx, "push tick: slot not moved", "error", err)
	}
}
