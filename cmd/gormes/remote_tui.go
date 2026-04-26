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
	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tui"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tuigateway"
)

// runRemoteTUIWithRuntime is the --remote <url> startup path. It does not
// instantiate a kernel, hermes HTTP client, or session DB; instead it
// dials the remote gateway's SSE event stream and forwards Bubble Tea
// submit/cancel callbacks to the gateway over plain HTTP. The api_server
// health probe is intentionally skipped because the operator's brain
// runs on the remote side.
//
// This keeps the local Bubble Tea path entirely intact when --remote is
// not set: the function is only reachable via runResolvedTUIWithRuntime
// when invocation.RemoteURL is non-empty, so existing fixtures continue
// through the kernel-backed branch.
func runRemoteTUIWithRuntime(cmd *cobra.Command, invocation tuiInvocation, runtime rootRuntime) error {
	if runtime.tuiProgramFactory == nil {
		runtime.tuiProgramFactory = defaultTUIProgramFactory
	}

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dialCtx, dialCancel := context.WithTimeout(rootCtx, 5*time.Second)
	client, err := tuigateway.DialSSE(dialCtx, invocation.RemoteURL)
	dialCancel()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"remote streaming unavailable at %s: %v\n\nLocal Bubble Tea mode still works: re-run gormes without --remote.\n",
			invocation.RemoteURL, err,
		)
		return err
	}
	defer client.Close()

	frames := client.Frames()
	submit := func(text string) {
		if err := client.Submit(rootCtx, text); err != nil {
			slog.Warn("remote submit failed", "err", err)
		}
	}
	cancelTurn := func() {
		if err := client.Cancel(rootCtx); err != nil {
			slog.Warn("remote cancel failed", "err", err)
		}
	}

	model := tui.NewModelWithOptions(frames, submit, cancelTurn, tui.Options{
		MouseTracking: invocation.Config.TUI.MouseTracking,
	})
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if invocation.Config.TUI.MouseTracking {
		programOptions = append(programOptions, tea.WithMouseAllMotion())
	}
	prog := runtime.tuiProgramFactory(model, programOptions...)

	programDone := make(chan struct{})
	go func() {
		<-rootCtx.Done()
		prog.Quit()
		select {
		case <-programDone:
		case <-time.After(kernel.ShutdownBudget):
			slog.Error("shutdown budget exceeded; forcing exit")
			os.Exit(3)
		}
	}()

	_, err = prog.Run()
	close(programDone)
	return err
}
