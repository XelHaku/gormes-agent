package gateway

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

// BootHookConfig configures the built-in BOOT.md startup hook.
type BootHookConfig struct {
	Path   string
	Model  string
	Client hermes.Client
	Tools  *tools.Registry
	Log    *slog.Logger
}

// StartBootHook starts a background BOOT.md run when the file exists and is
// non-empty. It returns false when there is nothing to do.
func StartBootHook(ctx context.Context, cfg BootHookConfig) bool {
	if cfg.Client == nil || strings.TrimSpace(cfg.Model) == "" {
		return false
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}

	content, err := loadBootContent(cfg.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		cfg.Log.Warn("boot-md unavailable", "path", cfg.Path, "err", err)
		return false
	}
	if content == "" {
		return false
	}

	go runBootHook(ctx, cfg, buildBootPrompt(content))
	return true
}

func loadBootContent(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func runBootHook(ctx context.Context, cfg BootHookConfig, prompt string) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	k := kernel.New(kernel.Config{
		Model:             cfg.Model,
		Endpoint:          "boot-md",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             cfg.Tools,
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
	}, cfg.Client, store.NewNoop(), telemetry.New(), cfg.Log)

	frames := k.Render()
	done := make(chan struct{}, 1)
	go func() {
		defer close(done)
		_ = k.Run(runCtx)
	}()

	if !drainInitialBootFrame(runCtx, frames) {
		return
	}
	if err := k.Submit(kernel.PlatformEvent{
		Kind: kernel.PlatformEventSubmit,
		Text: prompt,
	}); err != nil {
		cfg.Log.Warn("boot-md submit failed", "err", err)
		cancel()
		<-done
		return
	}

	for {
		select {
		case <-ctx.Done():
			cancel()
			<-done
			return
		case frame, ok := <-frames:
			if !ok {
				return
			}
			switch frame.Phase {
			case kernel.PhaseIdle:
				logBootCompletion(cfg.Log, frame.DraftText)
				cancel()
				<-done
				return
			case kernel.PhaseFailed:
				cfg.Log.Warn("boot-md failed", "err", frame.LastError)
				cancel()
				<-done
				return
			case kernel.PhaseCancelling:
				cfg.Log.Warn("boot-md cancelled")
				cancel()
				<-done
				return
			}
		}
	}
}

func drainInitialBootFrame(ctx context.Context, frames <-chan kernel.RenderFrame) bool {
	select {
	case _, ok := <-frames:
		return ok
	case <-ctx.Done():
		return false
	}
}

func buildBootPrompt(content string) string {
	return "You are running a startup boot checklist. Follow the BOOT.md instructions below exactly.\n\n---\n" +
		content +
		"\n---\n\nExecute each instruction. Use the available tools when helpful.\n" +
		"If nothing needs attention and there is nothing to report, reply with ONLY: [SILENT]"
}

func logBootCompletion(log *slog.Logger, response string) {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" || strings.Contains(trimmed, "[SILENT]") {
		log.Info("boot-md completed (nothing to report)")
		return
	}
	log.Info("boot-md completed", "response", truncateBootText(trimmed, 200))
}

func truncateBootText(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
