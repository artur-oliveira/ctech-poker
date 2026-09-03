package table

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// unreachableStore is a real Store whose every call fails, so handleNextHand
// can be driven through its ordinary (non-panic) failure branch without a
// DynamoDB Local. Paired with a cancelled context the load fails immediately,
// which is exactly the shape seen in production: the post-hand timer fires
// while the request context that carried it is already gone.
func unreachableStore() *tablestore.Store {
	db := dynamodb.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
	}, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://127.0.0.1:1") })
	return tablestore.NewStore(db, "test")
}

// A transient failure when the next-hand timer fires used to leave the table
// on Complete with no pending countdown at all: the timer that dispatched the
// command was already spent, and on a quiet table nothing else arrives to
// re-derive it, so the hand stalled silently (#136).
func TestHandleNextHandReArmsTheTimerAfterATransientFailure(t *testing.T) {
	a := New("table-1", unreachableStore(), true, func(string, hand.Snapshot) {})
	t.Cleanup(func() { a.afkSweepTimer.Stop() })
	a.nextHandRetryDelay = time.Millisecond
	a.nextHandArmedFor = "hand-1"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.handleNextHand(ctx, nextHandCmd{}); err == nil {
		t.Fatal("expected the cancelled-context load to fail")
	}
	t.Cleanup(func() { a.nextHandTimer.Stop() })
	if a.nextHandTimer == nil {
		t.Fatal("no next-hand timer after a transient failure — the hand can never restart")
	}
	if a.nextHandRetries != 1 {
		t.Fatalf("retries = %d, want 1", a.nextHandRetries)
	}

	select {
	case cmd := <-a.cmds:
		if _, ok := cmd.(nextHandCmd); !ok {
			t.Fatalf("got command %T, want nextHandCmd", cmd)
		}
		cmd.reply() <- nil
	case <-time.After(2 * time.Second):
		t.Fatal("the re-armed timer never dispatched another nextHandCmd")
	}
}

// The re-arm is bounded: a permanently broken store must degrade to the AFK
// sweep's watchdog rather than spin a timer forever.
func TestNextHandRetriesAreBounded(t *testing.T) {
	a := New("table-1", nil, true, func(string, hand.Snapshot) {})
	t.Cleanup(func() { a.afkSweepTimer.Stop() })
	a.nextHandRetryDelay = time.Hour

	boom := errors.New("boom")
	for i := 1; i <= MaxNextHandRetries; i++ {
		if err := a.retryNextHand(boom); !errors.Is(err, boom) {
			t.Fatalf("attempt %d returned %v, want the original error", i, err)
		}
		if a.nextHandRetries != i {
			t.Fatalf("retries = %d after attempt %d", a.nextHandRetries, i)
		}
	}
	a.nextHandTimer.Stop()
	a.nextHandTimer = nil

	if err := a.retryNextHand(boom); !errors.Is(err, boom) {
		t.Fatalf("exhausted attempt returned %v, want the original error", err)
	}
	if a.nextHandTimer != nil {
		t.Fatal("a timer was armed past MaxNextHandRetries")
	}
	if a.nextHandRetries != 0 {
		t.Fatalf("retries = %d after exhaustion, want the counter reset", a.nextHandRetries)
	}
}

func TestArmNextHandTimerEnqueuesNextHandCmdWhenComplete(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), nextHandDelay: time.Millisecond, handID: "h1"}
	t.Cleanup(func() { close(a.done) })
	a.armNextHandTimer(true)

	select {
	case cmd := <-a.cmds:
		if _, ok := cmd.(nextHandCmd); !ok {
			t.Fatalf("got command %T, want nextHandCmd", cmd)
		}
		cmd.reply() <- nil
	case <-time.After(200 * time.Millisecond):
		t.Fatal("next-hand timer did not enqueue nextHandCmd")
	}
}

func TestArmNextHandTimerIsIdempotentForTheSameHandID(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), nextHandDelay: time.Hour, handID: "h1"}
	t.Cleanup(func() { close(a.done) })
	a.armNextHandTimer(true)
	first := a.nextHandDeadline
	a.armNextHandTimer(true) // same handID — must not restart the countdown
	if !a.nextHandDeadline.Equal(first) {
		t.Fatal("re-arming for the same handID must not restart the 5s countdown")
	}
}

func TestArmNextHandTimerClearsWhenNotComplete(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), nextHandDelay: time.Hour, handID: "h1"}
	t.Cleanup(func() { close(a.done) })
	a.armNextHandTimer(true)
	a.armNextHandTimer(false)
	if a.nextHandArmedFor != "" {
		t.Fatal("expected nextHandArmedFor cleared once the hand is no longer Complete")
	}
	if a.nextHandArmGuardHand != "" || a.nextHandArmsForHand != 0 {
		t.Fatal("expected the arm-storm guard reset once the hand is no longer Complete")
	}
}

// A burst of rearmTimersFromCache calls for one stuck hand (handleNextHand
// clears nextHandArmedFor on entry, so the same-hand idempotence check stops
// throttling once the timer fires) must not become an unbounded burst of
// next-hand dispatches. Past MaxNextHandArmsPerHand the timer is left
// un-armed: a table this stuck recovers via tablecleanup or an operator, not
// by retrying a transaction that keeps being rejected. Regression for the
// 2026-09-02 DynamoDB write-storm incident.
func TestArmNextHandTimerStopsReArmingAfterTheCap(t *testing.T) {
	a := &Actor{
		cmds: make(chan Command, 64), done: make(chan struct{}),
		nextHandDelay: time.Hour, handID: "stuck",
	}
	t.Cleanup(func() { close(a.done) })

	for i := 0; i < MaxNextHandArmsPerHand+50; i++ {
		a.nextHandArmedFor = "" // what handleNextHand does on every dispatch
		a.armNextHandTimer(true)
	}
	if a.nextHandArmsForHand != MaxNextHandArmsPerHand+1 {
		t.Fatalf("arms counter = %d, want it pinned at the cap (+1 for the once-only log marker)", a.nextHandArmsForHand)
	}
	a.nextHandTimer.Stop()
	a.nextHandTimer = nil
	a.nextHandArmedFor = ""
	a.armNextHandTimer(true)
	if a.nextHandTimer != nil {
		t.Fatal("a timer was armed past MaxNextHandArmsPerHand")
	}

	// A new hand ID resets the guard — the next real hand still gets its timer.
	a.handID = "fresh"
	a.armNextHandTimer(true)
	if a.nextHandTimer == nil {
		t.Fatal("guard did not reset when the hand ID changed")
	}
	a.nextHandTimer.Stop()
}
