package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/audit"
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

type gracefulShutdownManager interface {
	Shutdown(context.Context) error
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
	sessionMirror := startSessionIndexMirror(smap, slog.Default())
	defer sessionMirror.Stop()

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
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	reg := buildDefaultRegistry(rootCtx, cfg.Delegation, cfg.SkillsRoot(), hc, cfg.Hermes.Model)
	toolAudit := audit.NewJSONLWriter(config.ToolAuditLogPath())

	k := kernel.New(kernel.Config{
		Model:             cfg.Hermes.Model,
		Endpoint:          cfg.Hermes.Endpoint,
		Admission:         kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
		ToolAudit:         toolAudit,
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

	hooksRoot := config.HooksRoot()
	hooks, loadedHooks, err := gateway.LoadHookScripts(hooksRoot, slog.Default())
	if err != nil {
		slog.Warn("gateway hooks unavailable", "root", hooksRoot, "err", err)
		hooks = gateway.NewHooks()
	}

	mgr := gateway.NewManager(gateway.ManagerConfig{
		AllowedChats:   allowedChats,
		AllowDiscovery: allowDiscovery,
		CoalesceMs:     coalesceMs,
		SessionMap:     smap,
		Hooks:          hooks,
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
	bootPath := config.BootPath()
	bootQueued := gateway.StartBootHook(rootCtx, gateway.BootHookConfig{
		Path:   bootPath,
		Model:  cfg.Hermes.Model,
		Client: hc,
		Tools:  reg,
		Log:    slog.Default(),
	})
	go runGatewaySignalLoop(signals, kernel.ShutdownBudget, mgr, cancel, slog.Default(), os.Exit)

	slog.Info("gormes gateway starting", "channels", mgr.ChannelCount(), "endpoint", cfg.Hermes.Endpoint, "hooks_root", hooksRoot, "loaded_hooks", len(loadedHooks), "boot_path", bootPath, "boot_queued", bootQueued)
	return mgr.Run(rootCtx)
}

func runGatewaySignalLoop(signals <-chan os.Signal, budget time.Duration, mgr gracefulShutdownManager, cancel context.CancelFunc, log *slog.Logger, forceExit func(int)) {
	if log == nil {
		log = slog.Default()
	}
	if forceExit == nil {
		forceExit = os.Exit
	}

	sig, ok := <-signals
	if !ok {
		return
	}
	log.Info("gateway shutdown requested", "signal", sig.String())

	timer := time.AfterFunc(budget, func() {
		log.Error("shutdown budget exceeded; forcing exit")
		forceExit(3)
	})
	defer timer.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), budget)
	err := mgr.Shutdown(shutdownCtx)
	shutdownCancel()
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		log.Warn("gateway shutdown drain", "err", err)
	} else if err != nil {
		log.Warn("gateway shutdown drain", "err", err)
	}

	cancel()
}
