package main

import (
	"github.com/spf13/cobra"
	"gopkg.aoctech.app/poker/cli/internal/config"
)

// loadConfig resolves settings from the root command's persistent flags,
// layered over env vars, config.toml, and the built-in defaults.
func loadConfig(cmd *cobra.Command) (config.Settings, error) {
	get := func(name string) string {
		v, _ := cmd.Root().PersistentFlags().GetString(name)
		return v
	}
	return config.Load(config.Flags{
		ConfigPath:     get("config"),
		APIBaseURL:     get("api-url"),
		AccountBaseURL: get("account-url"),
		CardMode:       get("cards"),
	})
}
