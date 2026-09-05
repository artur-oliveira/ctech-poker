// Command poker is the CTech Poker terminal client.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is stamped by -ldflags at release time (see cli/.goreleaser.yaml).
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "poker",
		Short:   "CTech Poker — terminal client",
		Version: version,
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
