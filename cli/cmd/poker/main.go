// Command poker is the CTech Poker terminal client. Running it with no
// arguments launches the interactive shell: a login gate followed by a
// `/command` home REPL (profile, achievements, and — once wired — play and
// enter a table).
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"gopkg.aoctech.app/poker/cli/internal/tui"
)

// version is stamped by -ldflags at release time (see cli/.goreleaser.yaml).
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "poker",
		Short:   "CTech Poker — terminal client",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			_, err = tea.NewProgram(tui.NewShell(cfg), tea.WithAltScreen()).Run()
			return err
		},
	}
	root.PersistentFlags().String("config", "", "path to config.toml")
	root.PersistentFlags().String("api-url", "", "override the poker API base URL")
	root.PersistentFlags().String("account-url", "", "override the account base URL")
	root.PersistentFlags().String("cards", "", "card style: color|ascii")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
