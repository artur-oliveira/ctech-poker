package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

func runProfile(w io.Writer, cmd *cobra.Command, rc *rest.Client) error {
	p, err := rc.Me(cmd.Context())
	if err != nil {
		return explainAPIError(err)
	}
	name := p.Name
	if name == "" {
		name = "(sem nome)"
	}
	fmt.Fprintf(w, "%s\nfriend code: %s\nwallet mode: %s\nsandbox balance: %d\ngame balance: %d\n",
		name, p.FriendCode, p.WalletMode, p.SandboxBalance, p.GameBalance)
	return nil
}

func newProfileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "profile",
		Short: "Show your poker profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, rc, _, err := newClients(cmd)
			if err != nil {
				return err
			}
			return runProfile(os.Stdout, cmd, rc)
		},
	}
}

func runAchievements(w io.Writer, cmd *cobra.Command, rc *rest.Client) error {
	s, err := rc.Achievements(cmd.Context())
	if err != nil {
		return explainAPIError(err)
	}
	fmt.Fprintf(w, "%d/%d desbloqueadas, %d completas, %d/%d estrelas\n",
		s.Totals.Unlocked, s.Totals.Revealed, s.Totals.Completed, s.Totals.Stars, s.Totals.MaxStars)
	for _, a := range s.Achievements {
		mark := " "
		if a.Completed {
			mark = "✓"
		} else if a.Unlocked {
			mark = "•"
		}
		fmt.Fprintf(w, "[%s] %-24s %d★  progresso %d\n", mark, a.Key, a.Stars, a.Progress)
	}
	return nil
}

func newAchievementsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "achievements",
		Short: "Show your achievement progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, rc, _, err := newClients(cmd)
			if err != nil {
				return err
			}
			return runAchievements(os.Stdout, cmd, rc)
		},
	}
}
