package tablehandoff

import (
	"context"
	"testing"
	"time"
)

// A nil Service must degrade to a no-op both ways, same convention as
// tablenotify.Service and tableconn.Service.
func TestRequestCloseAndListenAreNoOpsWithoutAClient(t *testing.T) {
	for name, svc := range map[string]*Service{
		"nil service": nil,
		"nil client":  NewService(nil),
	} {
		svc.RequestClose(context.Background(), "table-1", []string{"conn-a"})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		done := make(chan struct{})
		go func() {
			svc.Listen(ctx, func([]string) { t.Errorf("%s: onClose called with no client", name) })
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("%s: Listen did not return promptly with no client", name)
		}
	}
}
