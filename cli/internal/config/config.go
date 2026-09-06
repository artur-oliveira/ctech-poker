// Package config resolves CLI settings from defaults, an optional TOML file,
// environment variables, and explicit flags (lowest to highest precedence).
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	// confirm real hostnames during rollout (docs/specs/2026-09-05-poker-cli.md §10).
	defaultAPIBaseURL     = "https://poker-api.aoctech.app"
	defaultAccountBaseURL = "https://accounts-api.aoctech.app"
	defaultClientID       = "poker-cli"
)

// Settings is the fully-resolved configuration for one CLI invocation.
type Settings struct {
	APIBaseURL     string
	AccountBaseURL string
	ClientID       string
	CardMode       string // "color" or "ascii"
	ConfigDir      string
}

// Flags carries explicit overrides from command-line flags. A zero value
// field means "not set" and falls through to env/file/default.
type Flags struct {
	ConfigPath     string
	APIBaseURL     string
	AccountBaseURL string
	ClientID       string
	CardMode       string
}

type fileConfig struct {
	APIBaseURL     string `toml:"api_base_url"`
	AccountBaseURL string `toml:"account_base_url"`
	ClientID       string `toml:"client_id"`
	CardMode       string `toml:"card_mode"`
}

func defaultConfigDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "ctech-poker")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ctech-poker")
}

// Load merges the built-in defaults, an optional config.toml, environment
// variables, and f (in ascending precedence). A missing config file is not an
// error — it just means every setting falls through to env/default.
func Load(f Flags) (Settings, error) {
	s := Settings{
		APIBaseURL:     defaultAPIBaseURL,
		AccountBaseURL: defaultAccountBaseURL,
		ClientID:       defaultClientID,
		CardMode:       "color",
		ConfigDir:      defaultConfigDir(),
	}

	path := f.ConfigPath
	if path == "" {
		path = filepath.Join(s.ConfigDir, "config.toml")
	}
	var fc fileConfig
	if _, err := toml.DecodeFile(path, &fc); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Settings{}, err
	}

	apply(&s.APIBaseURL, fc.APIBaseURL, os.Getenv("CTECH_POKER_API_URL"), f.APIBaseURL)
	apply(&s.AccountBaseURL, fc.AccountBaseURL, os.Getenv("CTECH_POKER_ACCOUNT_URL"), f.AccountBaseURL)
	apply(&s.ClientID, fc.ClientID, os.Getenv("CTECH_POKER_CLIENT_ID"), f.ClientID)
	apply(&s.CardMode, fc.CardMode, "", f.CardMode)
	if os.Getenv("NO_COLOR") != "" {
		s.CardMode = "ascii"
	}
	return s, nil
}

// apply sets *dst to the last non-empty value among vals, in order — each
// later argument wins over an earlier one.
func apply(dst *string, vals ...string) {
	for _, v := range vals {
		if v != "" {
			*dst = v
		}
	}
}

// CredentialsPath is where the CLI stores OAuth/API-key credentials for s.
func CredentialsPath(s Settings) string { return filepath.Join(s.ConfigDir, "credentials.json") }

// LogPath is where the CLI appends its error log (internal/applog).
func LogPath(s Settings) string { return filepath.Join(s.ConfigDir, "poker.log") }
