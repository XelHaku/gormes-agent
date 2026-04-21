package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	discordadapter "github.com/TrebuchetDynamics/gormes-agent/gormes/internal/discord"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

var discordCmd = &cobra.Command{
	Use:          "discord",
	Short:        "Run Gormes as a Discord bot adapter",
	Long:         "Long-polls Discord for the configured channel or guild, drives the same kernel + tool loop as the TUI, and persists turns to the SQLite memory store.",
	SilenceUsage: true,
	RunE:         runDiscord,
}

func validateDiscordConfig(cfg config.Config) error {
	if cfg.Discord.BotToken == "" {
		return fmt.Errorf("no Discord bot token — set GORMES_DISCORD_TOKEN env or [discord].bot_token in config.toml")
	}
	if cfg.Discord.AllowedGuildID != "" && cfg.Discord.AllowedChannelID == "" {
		return fmt.Errorf("discord: allowed_channel_id is required when allowed_guild_id is set")
	}
	return nil
}

func runDiscord(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if p, ok := config.LegacyHermesHome(); ok {
		slog.Info("detected upstream Hermes home — Gormes uses XDG paths and does NOT read state from it; run `gormes migrate --from-hermes` (planned Phase 5.O) to import sessions and memory", "hermes_home", p)
	}

	if err := validateDiscordConfig(cfg); err != nil {
		return err
	}

	chatKey := ""
	if cfg.Discord.AllowedChannelID != "" {
		chatKey = discordadapter.SessionKey(cfg.Discord.AllowedChannelID)
	}

	rt, err := openGatewayRuntime(cfg, gatewayRuntimeOptions{
		ChatKey:        chatKey,
		ResumeOverride: cfg.Resume,
		RecallEnabled:  chatKey != "",
	}, slog.Default())
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
		defer cancelShutdown()
		rt.Close(shutdownCtx)
	}()

	client, err := discordadapter.NewRealClient(cfg.Discord.BotToken)
	if err != nil {
		return err
	}

	bot := discordadapter.New(discordadapter.Config{
		AllowedGuildID:   cfg.Discord.AllowedGuildID,
		AllowedChannelID: cfg.Discord.AllowedChannelID,
		MentionRequired:  cfg.Discord.MentionRequired,
		CoalesceMs:       cfg.Discord.CoalesceMs,
		SessionMap:       rt.SessionMap,
	}, client, rt.Kernel, slog.Default())

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rt.Start(rootCtx)
	return bot.Run(rootCtx)
}
