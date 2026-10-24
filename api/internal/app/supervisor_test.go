package app

import (
	"context"
	"testing"
	"time"

	"go.uber.org/fx/fxtest"
)

// The supervisor's OnStop must cancel every worker's ctx and wait for it, so no
// loop outlives shutdown (listeners, spot poll, erasure consumer).
func TestSupervisorStopsWorkersOnShutdown(t *testing.T) {
	var started, exited = make(chan struct{}), make(chan struct{})
	lc := fxtest.NewLifecycle(t)
	sup := newSupervisor(lc)
	lc.RequireStart()
	if err := sup.Run(context.Background(), "w", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(exited)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	lc.RequireStop()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("worker still running after shutdown")
	}
}
