package handhook

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A failing consumer must not stop the ones behind it: the hand is already
// complete and claimed, so an abort silently drops bookkeeping nobody retries.
func TestDispatchRunsEveryConsumerAndReportsFailures(t *testing.T) {
	var ran []string
	failure := errors.New("boom")
	consumers := []Consumer{
		{Name: "first", Run: func(context.Context, Event) error { ran = append(ran, "first"); return failure }},
		{Name: "nilrun"},
		{Name: "second", Run: func(context.Context, Event) error { ran = append(ran, "second"); return nil }},
	}

	observed := map[string]error{}
	Dispatch(context.Background(), Event{SchemaVersion: EventSchemaVersion}, consumers,
		func(name string, started time.Time, err error) { observed[name] = err })

	if len(ran) != 2 || ran[0] != "first" || ran[1] != "second" {
		t.Fatalf("ran=%v", ran)
	}
	if !errors.Is(observed["first"], failure) {
		t.Fatalf("first err=%v", observed["first"])
	}
	if err, ok := observed["second"]; !ok || err != nil {
		t.Fatalf("second observed=%v ok=%v", err, ok)
	}
	if _, ok := observed["nilrun"]; ok {
		t.Fatal("a consumer with no Run must not be observed")
	}
}
