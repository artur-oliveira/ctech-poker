package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"testing"
	"time"
)

func TestGeneratePKCEChallengeIsS256OfVerifier(t *testing.T) {
	v, c, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(v))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); c != want {
		t.Fatalf("challenge = %q, want RawURLEncode(SHA256(verifier)) = %q", c, want)
	}
	if len(v) < 43 || len(v) > 128 {
		t.Fatalf("verifier length %d out of RFC 7636 range [43,128]", len(v))
	}
}

func TestGeneratePKCEProducesDistinctVerifiers(t *testing.T) {
	v1, _, _ := GeneratePKCE()
	v2, _, _ := GeneratePKCE()
	if v1 == v2 {
		t.Fatal("two calls produced the same verifier")
	}
}

func TestLoopbackReceiverReturnsCodeAndValidatesState(t *testing.T) {
	r, err := NewLoopbackReceiver(0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	go func() {
		time.Sleep(10 * time.Millisecond)
		http.Get(r.RedirectURI() + "?state=xyz&code=the-code")
	}()

	code, err := r.Wait(context.Background(), "xyz")
	if err != nil || code != "the-code" {
		t.Fatalf("code=%q err=%v", code, err)
	}
}

func TestLoopbackReceiverRejectsMismatchedState(t *testing.T) {
	r, err := NewLoopbackReceiver(0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	go func() {
		time.Sleep(10 * time.Millisecond)
		http.Get(r.RedirectURI() + "?state=WRONG&code=x")
	}()

	if _, err := r.Wait(context.Background(), "xyz"); err == nil {
		t.Fatal("mismatched state must error")
	}
}

func TestLoopbackReceiverSurfacesProviderError(t *testing.T) {
	r, err := NewLoopbackReceiver(0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	go func() {
		time.Sleep(10 * time.Millisecond)
		http.Get(r.RedirectURI() + "?state=xyz&error=access_denied&error_description=user+cancelled")
	}()

	if _, err := r.Wait(context.Background(), "xyz"); err == nil {
		t.Fatal("a provider error callback must surface as an error")
	}
}

func TestLoopbackReceiverRespectsContextDeadline(t *testing.T) {
	r, err := NewLoopbackReceiver(0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := r.Wait(ctx, "xyz"); err == nil {
		t.Fatal("Wait should have returned when the context deadline passed")
	}
}
