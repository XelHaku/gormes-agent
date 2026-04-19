// Command gormes is the Go frontend for Hermes Agent (Phase 1).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/config"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tui"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			dumpCrash(r)
			os.Exit(2)
		}
	}()

	root := &cobra.Command{
		Use:          "gormes",
		Short:        "Go frontend for Hermes Agent",
		SilenceUsage: true,
		RunE:         runTUI,
	}
	root.Flags().Bool("offline", false, "skip startup api_server health check (dev only — turns the TUI into a cosmetic smoke-tester)")
	root.AddCommand(doctorCmd, versionCmd)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(nil)
	if err != nil {
		return err
	}

	c := hermes.NewHTTPClient(cfg.Hermes.Endpoint, cfg.Hermes.APIKey)

	// Health check: 2s budget. Surface an actionable error if unreachable.
	offline, _ := cmd.Flags().GetBool("offline")
	if !offline {
		healthCtx, healthCancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := c.Health(healthCtx); err != nil {
			healthCancel()
			fmt.Fprintf(os.Stderr,
				"api_server not reachable at %s: %v\n\nStart it with:\n  API_SERVER_ENABLED=true hermes gateway start\n\nOr pass --offline to render the TUI without a live server (dev only).\n",
				cfg.Hermes.Endpoint, err)
			return err
		}
		healthCancel()
	}

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tm := telemetry.New()
	k := kernel.New(kernel.Config{
		Model:     cfg.Hermes.Model,
		Endpoint:  cfg.Hermes.Endpoint,
		Admission: kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
	}, c, store.NewNoop(), tm, slog.Default())

	go k.Run(rootCtx)

	submit := func(text string) {
		_ = k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: text})
	}
	cancelTurn := func() {
		_ = k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel})
	}

	model := tui.NewModel(k.Render(), submit, cancelTurn)
	prog := tea.NewProgram(model, tea.WithAltScreen())

	// Signal → shutdown-budget force-exit watcher.
	go func() {
		<-rootCtx.Done()
		time.AfterFunc(kernel.ShutdownBudget, func() {
			slog.Error("shutdown budget exceeded; forcing exit")
			os.Exit(3)
		})
		prog.Quit()
	}()

	_, err = prog.Run()
	return err
}

func dumpCrash(r any) {
	dir := config.CrashLogDir()
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, fmt.Sprintf("crash-%d.log", time.Now().Unix()))
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "panic:", r)
		fmt.Fprintln(os.Stderr, string(debug.Stack()))
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "panic: %v\n\n%s\n", r, debug.Stack())
	fmt.Fprintln(os.Stderr, "gormes crashed — log at "+path)
}
