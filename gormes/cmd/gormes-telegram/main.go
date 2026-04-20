// Command gormes-telegram is the Phase-2.B.1 Telegram adapter binary.
// Phase 2.C adds persistent session-id resume via internal/session.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/config"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/session"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telegram"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gormes-telegram:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if cfg.Telegram.BotToken == "" {
		return fmt.Errorf("no Telegram bot token — set GORMES_TELEGRAM_TOKEN env or [telegram].bot_token in config.toml")
	}
	if cfg.Telegram.AllowedChatID == 0 && !cfg.Telegram.FirstRunDiscovery {
		return fmt.Errorf("no chat allowlist and discovery disabled — set one of [telegram].allowed_chat_id or [telegram].first_run_discovery = true")
	}
	if os.Getenv("GORMES_TELEGRAM_TOKEN") == "" {
		slog.Warn("bot_token read from config.toml; prefer GORMES_TELEGRAM_TOKEN env var for secrets")
	}

	// Phase 2.C — open the session map before the kernel so we can prime it.
	smap, err := session.OpenBolt(config.SessionDBPath())
	if err != nil {
		return fmt.Errorf("session map: %w", err)
	}
	defer smap.Close()

	ctx := context.Background()
	var key string
	if cfg.Telegram.AllowedChatID != 0 {
		key = session.TelegramKey(cfg.Telegram.AllowedChatID)
		if cfg.Resume != "" {
			if err := smap.Put(ctx, key, cfg.Resume); err != nil {
				slog.Warn("failed to apply --resume override", "err", err)
			}
		}
	}
	var initialSID string
	if key != "" {
		if sid, err := smap.Get(ctx, key); err != nil {
			slog.Warn("could not load initial session_id", "key", key, "err", err)
		} else {
			initialSID = sid
			if sid != "" {
				slog.Info("resuming persisted session", "key", key, "session_id", sid)
			}
		}
	}

	hc := hermes.NewHTTPClient(cfg.Hermes.Endpoint, cfg.Hermes.APIKey)

	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	reg.MustRegister(&tools.NowTool{})
	reg.MustRegister(&tools.RandIntTool{})

	tm := telemetry.New()
	k := kernel.New(kernel.Config{
		Model:             cfg.Hermes.Model,
		Endpoint:          cfg.Hermes.Endpoint,
		Admission:         kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
		InitialSessionID:  initialSID,
	}, hc, store.NewNoop(), tm, slog.Default())

	tc, err := telegram.NewRealClient(cfg.Telegram.BotToken)
	if err != nil {
		return err
	}

	bot := telegram.New(telegram.Config{
		AllowedChatID:     cfg.Telegram.AllowedChatID,
		CoalesceMs:        cfg.Telegram.CoalesceMs,
		FirstRunDiscovery: cfg.Telegram.FirstRunDiscovery,
		SessionMap:        smap,
		SessionKey:        key,
	}, tc, k, slog.Default())

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go k.Run(rootCtx)
	go func() {
		<-rootCtx.Done()
		time.AfterFunc(kernel.ShutdownBudget, func() {
			slog.Error("shutdown budget exceeded; forcing exit")
			os.Exit(3)
		})
	}()

	slog.Info("gormes-telegram starting",
		"endpoint", cfg.Hermes.Endpoint,
		"allowed_chat_id", cfg.Telegram.AllowedChatID,
		"discovery", cfg.Telegram.FirstRunDiscovery,
		"sessions_db", config.SessionDBPath())
	return bot.Run(rootCtx)
}
