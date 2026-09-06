package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveThenLoadRoundTrips(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "credentials.json")
	want := Credentials{
		AccessToken:  "at",
		RefreshToken: "rt",
		ExpiresAt:    time.Unix(1800000000, 0).UTC(),
		TokenType:    "Bearer",
		ObtainedVia:  "pkce",
	}
	if err := SaveCredentials(p, want); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v err=%v, want 0600", fi.Mode().Perm(), err)
	}
	got, ok, err := LoadCredentials(p)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.AccessToken != "at" || got.ObtainedVia != "pkce" || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestLoadCorruptFileReadsAsLoggedOut(t *testing.T) {
	p := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadCredentials(p)
	if err != nil || ok || got.AccessToken != "" {
		t.Fatalf("corrupt file must read as logged out: ok=%v err=%v got=%+v", ok, err, got)
	}
}

func TestLoadMissingFileReadsAsLoggedOut(t *testing.T) {
	got, ok, err := LoadCredentials(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || ok || got.AccessToken != "" {
		t.Fatalf("missing file must read as logged out: ok=%v err=%v", ok, err)
	}
}

func TestClearCredentialsRemovesFileAndIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "credentials.json")
	if err := SaveCredentials(p, Credentials{AccessToken: "at"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearCredentials(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file should be gone, stat err = %v", err)
	}
	if err := ClearCredentials(p); err != nil {
		t.Fatalf("clearing an already-absent file must not error: %v", err)
	}
}

func TestNeedsRefresh(t *testing.T) {
	now := time.Unix(1000, 0)
	if !(Credentials{ExpiresAt: time.Unix(1030, 0)}).NeedsRefresh(now) {
		t.Error("30s to expiry should need refresh (60s skew window)")
	}
	if (Credentials{ExpiresAt: time.Unix(5000, 0)}).NeedsRefresh(now) {
		t.Error("plenty of time left should not need refresh")
	}
	if !(Credentials{ExpiresAt: time.Unix(500, 0)}).NeedsRefresh(now) {
		t.Error("an already-past expiry should need refresh")
	}
}
