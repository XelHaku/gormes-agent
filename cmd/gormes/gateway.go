package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/internal/channels/discord"
	telegram "github.com/TrebuchetDynamics/gormes-agent/internal/channels/telegram"
	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/internal/slack"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
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

type gatewayChannelFactory func(config.Config, *slog.Logger) (gateway.Channel, error)

type gatewayChannelFactories struct {
	Telegram gatewayChannelFactory
	Discord  gatewayChannelFactory
	Slack    gatewayChannelFactory
}

func runGateway(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.Telegram.BotToken == "" && !cfg.Discord.Enabled() && !cfg.Slack.Enabled {
		return fmt.Errorf("no channels configured — set at least one of [telegram], [discord], or [slack] in config.toml")
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
	coalesceMs := gatewayCoalesceMs(cfg)

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
		RuntimeStatus:  gateway.NewRuntimeStatusStore(config.GatewayRuntimeStatusPath()),
	}, k, slog.Default())

	registeredChannels, err := registerConfiguredGatewayChannels(mgr, cfg, allowedChats, allowDiscovery, defaultGatewayChannelFactories(), gateway.NewRuntimeStatusStore(config.GatewayRuntimeStatusPath()), slog.Default())
	if err != nil {
		return err
	}
	if registeredChannels == 0 {
		return fmt.Errorf("no runnable channels configured — complete at least one of [telegram], [discord], or [slack] in config.toml")
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

func defaultGatewayChannelFactories() gatewayChannelFactories {
	return gatewayChannelFactories{
		Telegram: func(cfg config.Config, log *slog.Logger) (gateway.Channel, error) {
			tc, err := telegram.NewRealClient(cfg.Telegram.BotToken)
			if err != nil {
				return nil, err
			}
			return telegram.New(telegram.Config{
				AllowedChatID:     cfg.Telegram.AllowedChatID,
				FirstRunDiscovery: cfg.Telegram.FirstRunDiscovery,
			}, tc, log), nil
		},
		Discord: func(cfg config.Config, log *slog.Logger) (gateway.Channel, error) {
			ds, err := discord.NewRealSession(cfg.Discord.Token)
			if err != nil {
				return nil, err
			}
			return discord.New(discord.Config{
				AllowedChannelID:  cfg.Discord.AllowedChannelID,
				FirstRunDiscovery: cfg.Discord.FirstRunDiscovery,
			}, ds, log), nil
		},
		Slack: func(cfg config.Config, log *slog.Logger) (gateway.Channel, error) {
			return slack.NewChannel(slack.NewRealClient(cfg.Slack.BotToken, cfg.Slack.AppToken), log), nil
		},
	}
}

func gatewayCoalesceMs(cfg config.Config) int {
	coalesceMs := 1000
	if cfg.Telegram.CoalesceMs > 0 {
		coalesceMs = cfg.Telegram.CoalesceMs
	}
	if cfg.Discord.CoalesceMs > 0 && (cfg.Telegram.BotToken == "" || cfg.Telegram.CoalesceMs <= 0) {
		coalesceMs = cfg.Discord.CoalesceMs
	}
	if cfg.Slack.Enabled && cfg.Slack.CoalesceMs > 0 && cfg.Telegram.BotToken == "" && !cfg.Discord.Enabled() {
		coalesceMs = cfg.Slack.CoalesceMs
	}
	return coalesceMs
}

func registerConfiguredGatewayChannels(mgr *gateway.Manager, cfg config.Config, allowedChats map[string]string, allowDiscovery map[string]bool, factories gatewayChannelFactories, status gateway.RuntimeStatusWriter, log *slog.Logger) (int, error) {
	if log == nil {
		log = slog.Default()
	}
	registered := 0

	if cfg.Telegram.BotToken != "" {
		if factories.Telegram == nil {
			return registered, fmt.Errorf("register telegram: missing channel factory")
		}
		ch, err := factories.Telegram(cfg, log)
		if err != nil {
			return registered, err
		}
		if err := mgr.Register(ch); err != nil {
			return registered, fmt.Errorf("register telegram: %w", err)
		}
		if cfg.Telegram.AllowedChatID != 0 {
			allowedChats["telegram"] = strconv.FormatInt(cfg.Telegram.AllowedChatID, 10)
		}
		allowDiscovery["telegram"] = cfg.Telegram.FirstRunDiscovery
		registered++
		log.Info("gateway: telegram channel enabled", "allowed_chat_id", cfg.Telegram.AllowedChatID)
	}

	if cfg.Discord.Enabled() {
		if factories.Discord == nil {
			return registered, fmt.Errorf("register discord: missing channel factory")
		}
		ch, err := factories.Discord(cfg, log)
		if err != nil {
			return registered, err
		}
		if err := mgr.Register(ch); err != nil {
			return registered, fmt.Errorf("register discord: %w", err)
		}
		if cfg.Discord.AllowedChannelID != "" {
			allowedChats["discord"] = cfg.Discord.AllowedChannelID
		}
		allowDiscovery["discord"] = cfg.Discord.FirstRunDiscovery
		registered++
		log.Info("gateway: discord channel enabled", "allowed_channel_id", cfg.Discord.AllowedChannelID)
	}

	if !cfg.Slack.Enabled {
		return registered, nil
	}
	if cfg.Slack.AllowedChannelID != "" {
		allowedChats["slack"] = cfg.Slack.AllowedChannelID
	}
	allowDiscovery["slack"] = cfg.Slack.FirstRunDiscovery

	if missing := missingSlackCredentials(cfg.Slack); len(missing) > 0 {
		errText := "slack: missing " + strings.Join(missing, ",")
		writeGatewayChannelDegraded(status, "slack", errText)
		log.Warn("gateway: slack channel disabled by missing credentials", "missing", strings.Join(missing, ","))
		return registered, nil
	}
	if factories.Slack == nil {
		return registered, fmt.Errorf("register slack: missing channel factory")
	}
	ch, err := factories.Slack(cfg, log)
	if err != nil {
		errText := "slack: startup failed: " + err.Error()
		writeGatewayChannelDegraded(status, "slack", errText)
		log.Warn("gateway: slack channel startup failed", "err", err)
		return registered, nil
	}
	if err := mgr.Register(ch); err != nil {
		return registered, fmt.Errorf("register slack: %w", err)
	}
	registered++
	log.Info("gateway: slack channel enabled", "allowed_channel_id", cfg.Slack.AllowedChannelID)
	return registered, nil
}

func writeGatewayChannelDegraded(status gateway.RuntimeStatusWriter, platform, errText string) {
	if status == nil {
		return
	}
	_ = status.UpdateRuntimeStatus(context.Background(), gateway.RuntimeStatusUpdate{
		Platform:      platform,
		PlatformState: gateway.PlatformStateFailed,
		ErrorMessage:  errText,
	})
}

func missingSlackCredentials(cfg config.SlackCfg) []string {
	missing := []string{}
	if strings.TrimSpace(cfg.BotToken) == "" {
		missing = append(missing, "bot_token")
	}
	if strings.TrimSpace(cfg.AppToken) == "" {
		missing = append(missing, "app_token")
	}
	return missing
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
