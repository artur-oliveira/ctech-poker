package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrecedenceFlagOverEnvOverFileOverDefault(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("api_base_url = \"https://file.example\"\nclient_id = \"from-file\"\n"), 0o600)
	t.Setenv("CTECH_POKER_API_URL", "https://env.example")

	got, err := Load(Flags{ConfigPath: filepath.Join(dir, "config.toml"), ClientID: "from-flag"})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIBaseURL != "https://env.example" {
		t.Errorf("api url: want env value, got %q", got.APIBaseURL)
	}
	if got.ClientID != "from-flag" {
		t.Errorf("client id: want flag value, got %q", got.ClientID)
	}
	if got.AccountBaseURL == "" {
		t.Error("account url should fall back to the built-in default")
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	if _, err := Load(Flags{ConfigPath: filepath.Join(t.TempDir(), "nope.toml")}); err != nil {
		t.Fatalf("a missing config file must not fail Load: %v", err)
	}
}

func TestNoColorEnvForcesASCIICardMode(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	got, err := Load(Flags{ConfigPath: filepath.Join(t.TempDir(), "nope.toml")})
	if err != nil {
		t.Fatal(err)
	}
	if got.CardMode != "ascii" {
		t.Errorf("card mode = %q, want ascii when NO_COLOR is set", got.CardMode)
	}
}

func TestCredentialsPathIsUnderConfigDir(t *testing.T) {
	s := Settings{ConfigDir: "/tmp/xyz"}
	if got := CredentialsPath(s); got != "/tmp/xyz/credentials.json" {
		t.Errorf("CredentialsPath = %q", got)
	}
}
