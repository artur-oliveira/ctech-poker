package handshare

import (
	"testing"
)

func TestNewTokenIsOpaqueAndURLSafe(t *testing.T) {
	first, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || first == second {
		t.Fatalf("tokens must be independent 192-bit URL-safe values: %q %q", first, second)
	}
}
