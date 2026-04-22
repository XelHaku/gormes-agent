package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/channels/discord"
	telegram "github.com/TrebuchetDynamics/gormes-agent/gormes/internal/channels/telegram"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/doctor"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
)

func init() {
	doctorCmd.Flags().Bool("offline", false, "skip the api_server health check and only validate the local tool registry")
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Verify Gormes runtime: api_server reachability + built-in tools",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(nil)
		if err != nil {
			return err
		}

		offline, _ := cmd.Flags().GetBool("offline")

		if !offline {
			c := hermes.NewHTTPClient(cfg.Hermes.Endpoint, cfg.Hermes.APIKey)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := c.Health(ctx); err != nil {
				fmt.Fprintf(os.Stderr,
					"[FAIL] api_server: NOT reachable at %s: %v\n\nStart it with:\n  API_SERVER_ENABLED=true hermes gateway start\n\nOr pass --offline to validate only the local tool registry.\n",
					cfg.Hermes.Endpoint, err)
				os.Exit(1)
			}
			fmt.Printf("[PASS] api_server: reachable at %s\n", cfg.Hermes.Endpoint)
		} else {
			fmt.Println("[SKIP] api_server: skipped (--offline)")
		}

		// Toolbox section — inspect the built-in registry. Runs in both modes.
		reg := buildDefaultRegistry(context.Background(), cfg.Delegation, cfg.SkillsRoot(), nil, cfg.Hermes.Model)
		result := doctor.CheckTools(reg)
		fmt.Print(result.Format())

		if cfg.Telegram.BotToken == "" && !cfg.Discord.Enabled() {
			fmt.Println("[WARN] gateway: no channels configured ([telegram] or [discord])")
		} else {
			if cfg.Telegram.BotToken != "" {
				if _, err := telegram.NewRealClient(cfg.Telegram.BotToken); err != nil {
					fmt.Printf("[FAIL] gateway/telegram: %v\n", err)
					os.Exit(2)
				}
				fmt.Printf("[PASS] gateway/telegram: allowed_chat_id=%d\n", cfg.Telegram.AllowedChatID)
			} else {
				fmt.Println("[SKIP] gateway/telegram: disabled")
			}

			if cfg.Discord.Enabled() {
				if _, err := discord.NewRealSession(cfg.Discord.Token); err != nil {
					fmt.Printf("[FAIL] gateway/discord: %v\n", err)
					os.Exit(2)
				}
				fmt.Printf("[PASS] gateway/discord: allowed_channel_id=%s\n", cfg.Discord.AllowedChannelID)
			} else {
				fmt.Println("[SKIP] gateway/discord: disabled")
			}
		}

		if result.Status == doctor.StatusFail {
			os.Exit(2)
		}
		return nil
	},
}
