package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/channels/discord"
	telegram "github.com/TrebuchetDynamics/gormes-agent/gormes/internal/channels/telegram"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

var gatewayCmd = &cobra.Command{
	Use:          "gateway",
	Short:        "Run Gormes as a multi-channel messaging gateway",
	Long:         "Runs every configured channel through one gateway.Manager that drives the same kernel + tool loop as the TUI.",
	SilenceUsage: true,
	RunE:         runGateway,
}

func runGateway(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.Telegram.BotToken == "" && !cfg.Discord.Enabled() {
		return fmt.Errorf("no channels configured — set at least one of [telegram] or [discord] in config.toml")
	}

	smap, err := session.OpenBolt(config.SessionDBPath())
	if err != nil {
		return fmt.Errorf("session map: %w", err)
	}
	defer smap.Close()

	mstore, err := memory.OpenSqlite(config.MemoryDBPath(), cfg.Telegram.MemoryQueueCap, slog.Default())
	if err != nil {
		return fmt.Errorf("memory store: %w", err)
	}
	defer func() {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
		defer cancelShutdown()
		if err := mstore.Close(shutdownCtx); err != nil {
			slog.Warn("memory store close", "err", err)
		}
	}()

	hc := hermes.NewHTTPClient(cfg.Hermes.Endpoint, cfg.Hermes.APIKey)
	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	k := kernel.New(kernel.Config{
		Model:             cfg.Hermes.Model,
		Endpoint:          cfg.Hermes.Endpoint,
		Admission:         kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
		Tools:             buildDefaultRegistry(rootCtx, cfg.Delegation),
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
	}, hc, mstore, telemetry.New(), slog.Default())

	allowedChats := map[string]string{}
	allowDiscovery := map[string]bool{}
	coalesceMs := 1000
	if cfg.Telegram.CoalesceMs > 0 {
		coalesceMs = cfg.Telegram.CoalesceMs
	}
	if cfg.Discord.CoalesceMs > 0 && (cfg.Telegram.BotToken == "" || cfg.Telegram.CoalesceMs <= 0) {
		coalesceMs = cfg.Discord.CoalesceMs
	}

	mgr := gateway.NewManager(gateway.ManagerConfig{
		AllowedChats:   allowedChats,
		AllowDiscovery: allowDiscovery,
		CoalesceMs:     coalesceMs,
		SessionMap:     smap,
	}, k, slog.Default())

	if cfg.Telegram.BotToken != "" {
		tc, err := telegram.NewRealClient(cfg.Telegram.BotToken)
		if err != nil {
			return err
		}
		tgBot := telegram.New(telegram.Config{
			AllowedChatID:     cfg.Telegram.AllowedChatID,
			FirstRunDiscovery: cfg.Telegram.FirstRunDiscovery,
		}, tc, slog.Default())
		if err := mgr.Register(tgBot); err != nil {
			return fmt.Errorf("register telegram: %w", err)
		}
		if cfg.Telegram.AllowedChatID != 0 {
			allowedChats["telegram"] = strconv.FormatInt(cfg.Telegram.AllowedChatID, 10)
		}
		allowDiscovery["telegram"] = cfg.Telegram.FirstRunDiscovery
		slog.Info("gateway: telegram channel enabled", "allowed_chat_id", cfg.Telegram.AllowedChatID)
	}

	if cfg.Discord.Enabled() {
		ds, err := discord.NewRealSession(cfg.Discord.Token)
		if err != nil {
			return err
		}
		dBot := discord.New(discord.Config{
			AllowedChannelID:  cfg.Discord.AllowedChannelID,
			FirstRunDiscovery: cfg.Discord.FirstRunDiscovery,
		}, ds, slog.Default())
		if err := mgr.Register(dBot); err != nil {
			return fmt.Errorf("register discord: %w", err)
		}
		if cfg.Discord.AllowedChannelID != "" {
			allowedChats["discord"] = cfg.Discord.AllowedChannelID
		}
		allowDiscovery["discord"] = cfg.Discord.FirstRunDiscovery
		slog.Info("gateway: discord channel enabled", "allowed_channel_id", cfg.Discord.AllowedChannelID)
	}

	go k.Run(rootCtx)

	go func() {
		<-rootCtx.Done()
		time.AfterFunc(kernel.ShutdownBudget, func() {
			slog.Error("shutdown budget exceeded; forcing exit")
			os.Exit(3)
		})
	}()

	slog.Info("gormes gateway starting", "channels", mgr.ChannelCount(), "endpoint", cfg.Hermes.Endpoint)
	return mgr.Run(rootCtx)
}
