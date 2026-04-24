package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tui"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tuigateway"
)

// runRemoteTUI is the --remote code path: it opens a remote TUI surface
// against baseURL and runs a Bubble Tea program driven by it. No local
// kernel, no local health check, no session bolt — the remote gateway owns
// all of that. Failures from openRemoteTUISurface (invalid URL, unreachable
// gateway) surface synchronously so the startup path bails loudly.
func runRemoteTUI(baseURL string) error {
	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	frames, submit, cancelTurn, err := openRemoteTUISurface(rootCtx, baseURL, slog.Default())
	if err != nil {
		return err
	}

	model := tui.NewModel(frames, submit, cancelTurn)
	prog := tea.NewProgram(model, tea.WithAltScreen())

	go func() {
		<-rootCtx.Done()
		prog.Quit()
		select {
		case <-time.After(kernel.ShutdownBudget):
			slog.Error("shutdown budget exceeded; forcing exit")
			os.Exit(3)
		}
	}()

	_, err = prog.Run()
	return err
}

// openRemoteTUISurface wires a Bubble Tea client to a remote tuigateway
// mounted at baseURL. It is the "--remote <url>" port of entry called out as
// explicit 5.Q follow-on scope: the SSE + event-handler plumbing already
// exists in internal/tuigateway; this function threads it into the same
// frames-channel + Submitter/Canceller triple that tui.NewModel consumes in
// the local-kernel path, so downstream code is symmetric for both modes.
//
// baseURL is validated synchronously via tuigateway.NewRemoteClient — empty
// and scheme-less inputs are rejected at construction time, and a non-200
// SSE handshake is surfaced as a synchronous error so `cmd/gormes --remote`
// bails loudly instead of launching a TUI against an unreachable gateway.
//
// The returned Submitter / Canceller callbacks dispatch the POST to the
// gateway's EventsURL on a goroutine so the Bubble Tea Update loop is never
// blocked by network I/O. Errors are logged on the supplied logger because
// the TUI's Submitter/Canceller surface is fire-and-forget by design —
// matching the local path where `k.Submit(...)` errors are likewise
// discarded at the TUI seam.
func openRemoteTUISurface(ctx context.Context, baseURL string, logger *slog.Logger) (<-chan kernel.RenderFrame, tui.Submitter, tui.Canceller, error) {
	client, err := tuigateway.NewRemoteClient(baseURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("--remote: %w", err)
	}
	frames, err := client.Frames(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("--remote: dial frames: %w", err)
	}
	submit := tui.Submitter(func(text string) {
		go func() {
			if err := client.Submit(ctx, text); err != nil {
				logger.Warn("remote submit failed", "err", err)
			}
		}()
	})
	cancelTurn := tui.Canceller(func() {
		go func() {
			if err := client.Cancel(ctx); err != nil {
				logger.Warn("remote cancel failed", "err", err)
			}
		}()
	})
	return frames, submit, cancelTurn, nil
}
