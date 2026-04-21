package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/cron"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telegram"
)

// telegramCmd runs Gormes as a Telegram bot — the adapter previously
// shipped as the standalone cmd/gormes-telegram binary (Phase 2.B.1
// through 3.A). Unified into cmd/gormes under the < 100 MB binary
// ceiling; see the unification commit's message for rationale.
var telegramCmd = &cobra.Command{
	Use:          "telegram",
	Short:        "Run Gormes as a Telegram bot adapter",
	Long:         "Long-polls Telegram for DMs from the allowlisted chat, drives the same kernel + tool loop as the TUI, and persists turns to the SQLite memory store.",
	SilenceUsage: true,
	RunE:         runTelegram,
}

func runTelegram(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if p, ok := config.LegacyHermesHome(); ok {
		slog.Info("detected upstream Hermes home — Gormes uses XDG paths and does NOT read state from it; run `gormes migrate --from-hermes` (planned Phase 5.O) to import sessions and memory", "hermes_home", p)
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

	key := ""
	if cfg.Telegram.AllowedChatID != 0 {
		key = session.TelegramKey(cfg.Telegram.AllowedChatID)
	}

	rt, err := openGatewayRuntime(cfg, gatewayRuntimeOptions{
		ChatKey:        key,
		ResumeOverride: cfg.Resume,
		RecallEnabled:  cfg.Telegram.RecallEnabled,
	}, slog.Default())
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
		defer cancelShutdown()
		rt.Close(shutdownCtx)
	}()

	tc, err := telegram.NewRealClient(cfg.Telegram.BotToken)
	if err != nil {
		return err
	}

	bot := telegram.New(telegram.Config{
		AllowedChatID:     cfg.Telegram.AllowedChatID,
		CoalesceMs:        cfg.Telegram.CoalesceMs,
		FirstRunDiscovery: cfg.Telegram.FirstRunDiscovery,
		SessionMap:        rt.SessionMap,
		SessionKey:        key,
	}, tc, rt.Kernel, slog.Default())

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rt.Start(rootCtx)

	// Phase 2.D — cron scheduler + executor + mirror (opt-in via
	// cfg.Cron.Enabled). No-op when disabled — zero goroutines, zero
	// bbolt bucket, zero RAM.
	if cfg.Cron.Enabled && cfg.Telegram.AllowedChatID != 0 {
		// Reuse the existing session.db for the cron_jobs bucket.
		cronStore, err := cron.NewStore(rt.SessionMap.DB())
		if err != nil {
			return fmt.Errorf("cron: init store: %w", err)
		}
		cronRunStore := cron.NewRunStore(rt.MemoryStore.DB())

		sink := newTelegramDeliverySink(bot, cfg.Telegram.AllowedChatID)

		cronExec := cron.NewExecutor(cron.ExecutorConfig{
			Kernel:      rt.Kernel,
			JobStore:    cronStore,
			RunStore:    cronRunStore,
			Sink:        sink,
			CallTimeout: cfg.Cron.CallTimeout,
		}, slog.Default())

		cronSched := cron.NewScheduler(cron.SchedulerConfig{
			Store:    cronStore,
			Executor: cronExec,
		}, slog.Default())

		if err := cronSched.Start(rootCtx); err != nil {
			return fmt.Errorf("cron: start scheduler: %w", err)
		}
		defer func() {
			shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
			defer cancelShutdown()
			cronSched.Stop(shutdownCtx)
		}()

		cronMirror := cron.NewMirror(cron.MirrorConfig{
			JobStore: cronStore,
			RunStore: cronRunStore,
			Path:     cfg.CronMirrorPath(),
			Interval: cfg.Cron.MirrorInterval,
		}, slog.Default())
		go cronMirror.Run(rootCtx)
	}

	go func() {
		<-rootCtx.Done()
		time.AfterFunc(kernel.ShutdownBudget, func() {
			slog.Error("shutdown budget exceeded; forcing exit")
			os.Exit(3)
		})
	}()

	slog.Info("gormes telegram starting",
		"endpoint", cfg.Hermes.Endpoint,
		"allowed_chat_id", cfg.Telegram.AllowedChatID,
		"discovery", cfg.Telegram.FirstRunDiscovery,
		"sessions_db", config.SessionDBPath(),
		"memory_db", config.MemoryDBPath(),
		"extractor_batch_size", cfg.Telegram.ExtractorBatchSize,
		"extractor_poll_interval", cfg.Telegram.ExtractorPollInterval,
		"semantic_enabled", cfg.Telegram.SemanticEnabled,
		"semantic_model", cfg.Telegram.SemanticModel)
	return bot.Run(rootCtx)
}

// recallAdapter bridges *memory.Provider (which uses memory.RecallInput)
// to kernel.RecallProvider (which uses kernel.RecallParams). Same
// fields, distinct types — the adapter preserves package dependency
// isolation.
type recallAdapter struct {
	p *memory.Provider
}

func (a *recallAdapter) GetContext(ctx context.Context, params kernel.RecallParams) string {
	return a.p.GetContext(ctx, memory.RecallInput{
		UserMessage: params.UserMessage,
		ChatKey:     params.ChatKey,
		SessionID:   params.SessionID,
	})
}

// telegramBotSender is the narrow interface newTelegramDeliverySink
// needs — matches what *telegram.Bot exposes.
type telegramBotSender interface {
	SendToChat(ctx context.Context, chatID int64, text string) error
}

// newTelegramDeliverySink wraps the running Telegram bot as a
// cron.DeliverySink. Every cron-fired output is sent to the
// operator's configured AllowedChatID.
func newTelegramDeliverySink(bot telegramBotSender, chatID int64) cron.DeliverySink {
	return cron.FuncSink(func(ctx context.Context, text string) error {
		return bot.SendToChat(ctx, chatID, text)
	})
}
