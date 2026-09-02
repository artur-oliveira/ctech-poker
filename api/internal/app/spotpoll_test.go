package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

func TestSpotTerminationNoticedTrueOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("stop"))
	}))
	defer srv.Close()

	orig := spotTerminationMetadataURL
	spotTerminationMetadataURL = srv.URL
	defer func() { spotTerminationMetadataURL = orig }()

	noticed, err := spotTerminationNoticed(context.Background(), srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !noticed {
		t.Fatal("expected a 200 response to be reported as a termination notice")
	}
}

func TestSpotTerminationNoticedFalseOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := spotTerminationMetadataURL
	spotTerminationMetadataURL = srv.URL
	defer func() { spotTerminationMetadataURL = orig }()

	noticed, err := spotTerminationNoticed(context.Background(), srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if noticed {
		t.Fatal("expected a 404 response to report no termination notice")
	}
}

func TestSpotTerminationNoticedErrorsWhenUnreachable(t *testing.T) {
	orig := spotTerminationMetadataURL
	// Reserved TEST-NET-1 address: never routable, fails fast rather than
	// hanging until the client timeout — keeps the test itself fast while
	// still exercising the "inconclusive" error path callers must treat as
	// no-notice.
	spotTerminationMetadataURL = "http://192.0.2.1/latest/meta-data/spot/instance-action"
	defer func() { spotTerminationMetadataURL = orig }()

	client := &http.Client{Timeout: 200 * time.Millisecond}
	if _, err := spotTerminationNoticed(context.Background(), client); err == nil {
		t.Fatal("expected an error when the metadata endpoint is unreachable")
	}
}

// TestPollSpotTerminationDrainsProactivelyOnNotice proves the app.go
// integration point (#33): once IMDS reports a termination notice, the
// background poller calls manager.DrainAndRelease itself — without waiting
// on the OnStop/SIGTERM path — releasing every lease this instance held.
func TestPollSpotTerminationDrainsProactivelyOnNotice(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	orig := spotTerminationMetadataURL
	spotTerminationMetadataURL = srv.URL
	defer func() { spotTerminationMetadataURL = orig }()

	backend := cache.NewMemoryBackend(16)
	leases := tablelease.NewService(backend)
	manager := tablemanager.NewManager(leases, nil, nil, nil)
	ctx := context.Background()
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := manager.GetOrCreateActor(ctx, "table-1", seed); err != nil {
		t.Fatalf("get or create table-1: %v", err)
	}

	origInterval := spotPollInterval
	spotPollInterval = 5 * time.Millisecond
	defer func() { spotPollInterval = origInterval }()

	pollCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		pollSpotTermination(pollCtx, manager, srv.Client())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pollSpotTermination did not return after a detected notice")
	}

	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("expected at least one metadata poll")
	}

	// Drained: a sibling instance can now acquire the same lease.
	leases2 := tablelease.NewService(backend)
	m2 := tablemanager.NewManager(leases2, nil, nil, nil)
	if _, err := m2.GetOrCreateActor(ctx, "table-1", seed); err != nil {
		t.Fatalf("expected sibling instance to acquire table-1 after proactive drain, got %v", err)
	}
}
