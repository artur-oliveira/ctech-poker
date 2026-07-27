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
