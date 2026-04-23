package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage provider authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login <provider>",
	Short: "Run an interactive provider login flow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := normalizeAuthProvider(args[0])
		switch provider {
		case "google-gemini-cli":
			accepted, _ := cmd.Flags().GetBool("accept-google-oauth-risk")
			if !accepted {
				return fmt.Errorf("google OAuth login requires --accept-google-oauth-risk")
			}
			noBrowser, _ := cmd.Flags().GetBool("no-browser")
			result, err := config.LoginGoogleOAuth(cmd.Context(), config.GoogleOAuthLoginOptions{
				OpenBrowser: !noBrowser,
				NotifyURL: func(authURL string) {
					fmt.Fprintf(cmd.OutOrStdout(), "Visit:\n  %s\n", authURL)
				},
			})
			if err != nil {
				return err
			}
			if result.Email != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Signed in as %s\n", result.Email)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Signed in to Google Code Assist.")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved credentials to %s\n", result.Path)
			return nil
		default:
			return fmt.Errorf("unsupported auth provider %q", args[0])
		}
	},
}

func init() {
	authLoginCmd.Flags().Bool("accept-google-oauth-risk", false, "acknowledge the Google Gemini CLI OAuth policy warning")
	authLoginCmd.Flags().Bool("no-browser", false, "print the OAuth URL instead of opening a local browser")
	authCmd.AddCommand(authLoginCmd)
}

func normalizeAuthProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "google-gemini-cli", "gemini-cli", "gemini-oauth", "google-code-assist", "google_code_assist":
		return "google-gemini-cli"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}
