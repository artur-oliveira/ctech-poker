package tablenotify

import (
	"context"
	"testing"
	"time"
)

// Without a Valkey client there is only one process, so there is nothing to
// notify and nothing to listen for — both must be silent no-ops rather than
// panicking, matching handhook.Service/tableconn.Service's nil-degrades
// convention.
func TestNotifyAndListenAreNoOpsWithoutAClient(t *testing.T) {
	for name, svc := range map[string]*Service{
		"nil service": nil,
		"nil client":  NewService(nil),
	} {
		svc.Notify(context.Background(), "table-1")

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		done := make(chan struct{})
		go func() {
			svc.Listen(ctx, func(string) { t.Errorf("%s: onChange called with no client", name) })
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("%s: Listen did not return promptly with no client", name)
		}
	}
}
