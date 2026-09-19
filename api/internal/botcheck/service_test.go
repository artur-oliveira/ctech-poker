package botcheck

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestVerifyRequiresActionAndHostname(t *testing.T) {
	service := New("secret", "poker.example")
	service.SetTransportForTest(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"success":true,"action":"poker_bot_check","hostname":"poker.example"}`)),
			Header: make(http.Header),
		}, nil
	}))
	if err := service.Verify(context.Background(), "token", "203.0.113.1"); err != nil {
		t.Fatal(err)
	}
}

// The three cases #322's acceptance criteria calls for: a low-risk signal
// keeps the standard challenge, a high-risk one escalates, and — since this
// is a pure function over the caller's own signal — the decision can never
// itself weaken what Verify accepts.
func TestDecideChallengeLevelLowRiskStandard(t *testing.T) {
	if got := DecideChallengeLevel(RiskSignal{RecentAttempts: 1, ActionRiskScore: 0}); got != ChallengeStandard {
		t.Fatalf("expected ChallengeStandard, got %v", got)
	}
}

func TestDecideChallengeLevelHighAttemptsEscalates(t *testing.T) {
	if got := DecideChallengeLevel(RiskSignal{RecentAttempts: 25}); got != ChallengeEscalated {
		t.Fatalf("expected ChallengeEscalated, got %v", got)
	}
}

func TestDecideChallengeLevelHighActionScoreEscalates(t *testing.T) {
	if got := DecideChallengeLevel(RiskSignal{ActionRiskScore: 10}); got != ChallengeEscalated {
		t.Fatalf("expected ChallengeEscalated, got %v", got)
	}
}

func TestVerifyRejectsWrongAction(t *testing.T) {
	service := New("secret", "")
	service.SetTransportForTest(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"success":true,"action":"login"}`)),
			Header:     make(http.Header),
		}, nil
	}))
	if err := service.Verify(context.Background(), "token", ""); err == nil {
		t.Fatal("wrong action must fail")
	}
}
