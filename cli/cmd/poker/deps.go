package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"gopkg.aoctech.app/poker/cli/internal/auth"
	"gopkg.aoctech.app/poker/cli/internal/config"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

// newClients wires config + a Session + a REST client for a command's RunE.
func newClients(cmd *cobra.Command) (*auth.Session, *rest.Client, config.Settings, error) {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return nil, nil, config.Settings{}, err
	}
	session := auth.NewSession(cfg, http.DefaultClient)
	client := rest.New(cfg.APIBaseURL, session.Token, http.DefaultClient)
	return session, client, cfg, nil
}

// explainAPIError turns the CLI's most common failure modes into a message
// that tells the user what to actually do next, instead of a raw error.
func explainAPIError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, auth.ErrLoggedOut) {
		return errors.New("not logged in — run `poker login`")
	}
	if rest.IsStatus(err, http.StatusForbidden) {
		return fmt.Errorf("%w\nthis environment may not have enabled the poker-cli client for interactive use (see docs/specs/2026-09-05-poker-cli.md §2 and cli/CLAUDE.md)", err)
	}
	if rest.IsStatus(err, http.StatusUnauthorized) {
		return errors.New("session expired — run `poker login`")
	}
	return err
}
