package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"gopkg.aoctech.app/poker/cli/internal/auth"
)

func newLoginCmd() *cobra.Command {
	var apiKey string
	var device bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with a browser (PKCE) or an API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			if device {
				return errors.New("device authorization grant is not supported by the provider yet — use `poker login` (browser) or --api-key")
			}
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			s := auth.NewSession(cfg, http.DefaultClient)

			if apiKey != "" {
				if err := s.LoginAPIKey(cmd.Context(), apiKey); err != nil {
					return err
				}
				fmt.Println("Logged in with an API key.")
				return nil
			}

			fmt.Println("Opening your browser to log in...")
			if err := s.LoginPKCE(cmd.Context(), auth.OpenBrowser); err != nil {
				return err
			}
			fmt.Println("Logged in.")
			return nil
		},
	}
	cmd.Flags().StringVar(&apiKey, "api-key", "", "log in with a long-lived API key instead of a browser")
	cmd.Flags().BoolVar(&device, "device", false, "use the device authorization grant (not yet supported)")
	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			if err := auth.NewSession(cfg, http.DefaultClient).Logout(); err != nil {
				return err
			}
			fmt.Println("Logged out.")
			return nil
		},
	}
}
