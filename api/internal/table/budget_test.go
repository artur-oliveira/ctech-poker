package table

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// TestCommandBudgetClassSplit pins the split #223 asks for: a command a player
// is waiting on must not share a ceiling with one that can move money.
func TestCommandBudgetClassSplit(t *testing.T) {
	a := New("budget-classes", nil, true, nil)
	settlement := []Command{JoinCmd{}, LeaveCmd{}, kickTimeoutCmd{}, afkSweepCmd{}, nextHandCmd{}}
	for _, cmd := range settlement {
		if got := a.budgetFor(cmd); got != settlementCommandBudget {
			t.Fatalf("%T budget = %v, want the safe-completion budget %v", cmd, got, settlementCommandBudget)
		}
	}
	interactive := []Command{ActCmd{}, ChatCmd{}, SnapshotCmd{}, ReadyCmd{}, ConnectCmd{}, turnTimeoutCmd{}}
	for _, cmd := range interactive {
		if got := a.budgetFor(cmd); got != defaultCommandBudget {
			t.Fatalf("%T budget = %v, want the interactive budget %v", cmd, got, defaultCommandBudget)
		}
	}
	if defaultCommandBudget >= settlementCommandBudget {
		t.Fatalf("the interactive budget (%v) must be shorter than the settlement one (%v)",
			defaultCommandBudget, settlementCommandBudget)
	}
}

// TestDispatchRejectsWhenMailboxStaysFull proves the queue budget: past it the
// caller is told the table is unavailable instead of being parked forever on a
// wedged actor. The error must be recoverable (ErrUnavailable => the gateway
// answers "unavailable", the client resyncs and resubmits the same action ID),
// never an invalid_action verdict about the player's command.
func TestDispatchRejectsWhenMailboxStaysFull(t *testing.T) {
	a := New("budget-saturated", nil, true, nil)
	a.queueBudget = 20 * time.Millisecond
	// No Run goroutine: nothing drains the mailbox, so fill it directly.
	for len(a.cmds) < cap(a.cmds) {
		a.cmds <- SnapshotCmd{Reply: make(chan error, 1)}
	}

	started := time.Now()
	err := a.Dispatch(ActCmd{PlayerID: "p1", Reply: make(chan error, 1)})
	elapsed := time.Since(started)

	if !errors.Is(err, ErrQueueSaturated) {
		t.Fatalf("Dispatch err = %v, want ErrQueueSaturated", err)
	}
	if !errors.Is(err, tablestore.ErrUnavailable) {
		t.Fatalf("ErrQueueSaturated must wrap tablestore.ErrUnavailable, got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("Dispatch waited %v, want it bounded by the queue budget", elapsed)
	}
	if got := a.BudgetSnapshot(); got.QueueSaturations != 1 || got.QueueWait <= 0 {
		t.Fatalf("BudgetSnapshot = %+v, want one saturation and a non-zero queue wait", got)
	}
}

// TestDispatchStillWaitsOutABriefBacklog is the other half: the budget must not
// turn ordinary backpressure into a rejection. A mailbox that frees up inside
// the budget still delivers the command.
func TestDispatchStillWaitsOutABriefBacklog(t *testing.T) {
	a := New("budget-backlog", nil, true, nil)
	a.queueBudget = 2 * time.Second
	for len(a.cmds) < cap(a.cmds) {
		a.cmds <- SnapshotCmd{Reply: make(chan error, 1)}
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		for cmd := range a.cmds {
			cmd.reply() <- nil
		}
	}()

	if err := a.Dispatch(ActCmd{PlayerID: "p1", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("Dispatch err = %v, want the command to go through once the mailbox drains", err)
	}
	if got := a.BudgetSnapshot(); got.QueueSaturations != 0 {
		t.Fatalf("BudgetSnapshot = %+v, want no saturation", got)
	}
}

// hangingRoundTripper answers nothing until the request's own context expires —
// a stand-in for the hung dependency that used to pin a table's actor goroutine
// for as long as it stayed hung, because Run handed every handler the actor's
// process-lifetime context.
type hangingRoundTripper struct{ reached chan struct{} }

func (h *hangingRoundTripper) Do(req *http.Request) (*http.Response, error) {
	select {
	case h.reached <- struct{}{}:
	default:
	}
	<-req.Context().Done()
	return nil, req.Context().Err()
}

// TestCommandDeadlineBoundsHungStoreIO runs a real command against a DynamoDB
// client that never answers and asserts the command's own deadline — not the
// dependency — decides when the actor goroutine is free again, and that the
// failure surfaces as unavailable (resync) rather than invalid_action.
func TestCommandDeadlineBoundsHungStoreIO(t *testing.T) {
	stub := &hangingRoundTripper{reached: make(chan struct{}, 1)}
	db := dynamodb.New(dynamodb.Options{
		Region:           "us-east-1",
		Credentials:      credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
		HTTPClient:       stub,
		BaseEndpoint:     aws.String("https://dynamodb.us-east-1.amazonaws.com"),
		RetryMaxAttempts: 1,
	})
	a := New("budget-hung-io", tablestore.NewStore(db, "budget_test"), true, func(string, hand.Snapshot) {})
	a.commandBudget = 100 * time.Millisecond

	started := time.Now()
	err := a.handleWithBudget(context.Background(), SnapshotCmd{
		PlayerID: "p1", Snapshot: make(chan hand.Snapshot, 1), Reply: make(chan error, 1),
	})
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("want an error from a command whose store never answered")
	}
	if !errors.Is(err, tablestore.ErrUnavailable) {
		t.Fatalf("err = %v, want it to wrap tablestore.ErrUnavailable so the gateway answers unavailable", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("the command took %v, want it bounded by its %v budget", elapsed, a.commandBudget)
	}
	select {
	case <-stub.reached:
	default:
		t.Fatal("the store was never called, so this proved nothing about the deadline")
	}
	if got := a.BudgetSnapshot(); got.HandlerOverruns != 1 {
		t.Fatalf("BudgetSnapshot = %+v, want exactly one handler overrun counted", got)
	}
}
