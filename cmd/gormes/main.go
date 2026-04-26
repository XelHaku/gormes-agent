// Command gormes is the Go frontend for Hermes Agent (Phase 1).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tui"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			dumpCrash(r)
			os.Exit(2)
		}
	}()

	root := newRootCommand()
	if err := root.Execute(); err != nil {
		os.Exit(exitCodeFromError(err))
	}
}

func newRootCommand() *cobra.Command {
	return newRootCommandWithRuntime(rootRuntime{})
}

type rootRuntime struct {
	runTUI                 func(*cobra.Command, []string) error
	runResolvedTUI         func(*cobra.Command, tuiInvocation) error
	runOneshot             func(*cobra.Command, oneshotInvocation) error
	newOneshotClient       oneshotClientFactory
	configureOneshotKernel oneshotKernelConfigurer
	tuiProgramFactory      tuiProgramFactory
}

type tuiInvocation struct {
	Inference config.TUIInferenceResolution
	Config    config.Config
}

type oneshotInvocation struct {
	Prompt    string
	Inference config.OneshotInferenceResolution
	Config    config.Config
}

func newRootCommandWithRuntime(runtime rootRuntime) *cobra.Command {
	if runtime.tuiProgramFactory == nil {
		runtime.tuiProgramFactory = defaultTUIProgramFactory
	}
	if runtime.runResolvedTUI == nil {
		if runtime.runTUI != nil {
			runLegacyTUI := runtime.runTUI
			runtime.runResolvedTUI = func(cmd *cobra.Command, _ tuiInvocation) error {
				return runLegacyTUI(cmd, nil)
			}
		} else {
			runtime.runResolvedTUI = func(cmd *cobra.Command, invocation tuiInvocation) error {
				return runResolvedTUIWithRuntime(cmd, invocation, runtime)
			}
		}
	}
	if runtime.newOneshotClient == nil {
		runtime.newOneshotClient = newOneshotHTTPClient
	}
	if runtime.runOneshot == nil {
		newClient := runtime.newOneshotClient
		configureKernel := runtime.configureOneshotKernel
		runtime.runOneshot = func(cmd *cobra.Command, invocation oneshotInvocation) error {
			return runResolvedOneshotWithClient(cmd, invocation, newClient, configureKernel)
		}
	}
	resetGonchoDoctorFlags()
	root := &cobra.Command{
		Use:          "gormes",
		Short:        "Go frontend for Hermes Agent",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRootCommand(cmd, args, runtime)
		},
	}
	root.Flags().StringP("oneshot", "z", "", "one-shot mode: send a single prompt and resolve model/provider selection without starting the TUI")
	root.Flags().StringP("model", "m", "", "model override for --oneshot or TUI startup; also settable via GORMES_INFERENCE_MODEL")
	root.Flags().String("provider", "", "provider override for --oneshot or TUI startup; also settable via GORMES_INFERENCE_PROVIDER")
	root.Flags().Bool("offline", false, "skip startup api_server health check (dev only — turns the TUI into a cosmetic smoke-tester)")
	root.Flags().String("resume", "", "override persisted session_id for the TUI's default key")
	root.AddCommand(doctorCmd, versionCmd, telegramCmd, gatewayCmd, sessionCmd, memoryCmd, gonchoCmd)
	return root
}

func runRootCommand(cmd *cobra.Command, args []string, runtime rootRuntime) error {
	if cmd.Flags().Changed("oneshot") {
		invocation, err := resolveOneshotInvocation(cmd)
		if err != nil {
			return err
		}
		return runtime.runOneshot(cmd, invocation)
	}
	invocation, err := resolveTUIInvocation(cmd)
	if err != nil {
		return err
	}
	return runtime.runResolvedTUI(cmd, invocation)
}

func resolveOneshotInvocation(cmd *cobra.Command) (oneshotInvocation, error) {
	prompt, _ := cmd.Flags().GetString("oneshot")
	modelFlag, _ := cmd.Flags().GetString("model")
	providerFlag, _ := cmd.Flags().GetString("provider")

	cfg, err := config.Load(nil)
	if err != nil {
		return oneshotInvocation{Prompt: prompt}, err
	}
	resolution, err := config.ResolveOneshotInference(config.OneshotInferenceRequest{
		Config:       cfg,
		ModelFlag:    modelFlag,
		ProviderFlag: providerFlag,
	})
	resolution = resolveStaticStartupInference(resolution)
	invocation := oneshotInvocation{
		Prompt:    prompt,
		Inference: resolution,
		Config:    cfg,
	}
	if err != nil {
		return invocation, newExitCodeError(2, err)
	}
	return invocation, nil
}

func resolveTUIInvocation(cmd *cobra.Command) (tuiInvocation, error) {
	modelFlag, _ := cmd.Flags().GetString("model")
	providerFlag, _ := cmd.Flags().GetString("provider")

	cfg, err := config.Load(nil)
	if err != nil {
		return tuiInvocation{}, err
	}
	resolution, err := config.ResolveTUIInference(config.TUIInferenceRequest{
		Config:       cfg,
		ModelFlag:    modelFlag,
		ProviderFlag: providerFlag,
	})
	resolution = resolveStaticStartupInference(resolution)
	invocation := tuiInvocation{
		Inference: resolution,
		Config:    cfg,
	}
	if err != nil {
		return invocation, newExitCodeError(2, err)
	}
	return invocation, nil
}

func resolveStaticStartupInference(resolution config.InferenceResolution) config.InferenceResolution {
	if resolution.Model == "" {
		return resolution
	}
	metadata := hermes.LookupModelMetadata(hermes.ModelRegistryQuery{
		Provider: resolution.Provider,
		Model:    resolution.Model,
	})
	if !metadata.Found {
		return resolution
	}
	resolution.Model = metadata.Model
	if resolution.Provider == "" {
		resolution.Provider = metadata.Provider
		resolution.ProviderAutoDetectRequired = false
	}
	return resolution
}

type oneshotClientFactory func(context.Context, config.Config, oneshotInvocation) (hermes.Client, error)
type oneshotKernelConfigurer func(*kernel.Config)

func newOneshotHTTPClient(_ context.Context, cfg config.Config, invocation oneshotInvocation) (hermes.Client, error) {
	return hermes.NewHTTPClientWithProvider(cfg.Hermes.Endpoint, cfg.Hermes.APIKey, invocation.Inference.Provider), nil
}

func runResolvedOneshot(cmd *cobra.Command, invocation oneshotInvocation) error {
	return runResolvedOneshotWithClient(cmd, invocation, newOneshotHTTPClient)
}

func runResolvedOneshotWithClient(cmd *cobra.Command, invocation oneshotInvocation, newClient oneshotClientFactory, configureKernel ...oneshotKernelConfigurer) error {
	if newClient == nil {
		newClient = newOneshotHTTPClient
	}
	cfg := invocation.Config
	model := invocation.Inference.Model
	if model == "" {
		model = cfg.Hermes.Model
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := newClient(rootCtx, cfg, invocation)
	if err != nil {
		return newExitCodeError(1, fmt.Errorf("gormes -z: provider setup failed: %w", err))
	}
	if client == nil {
		return newExitCodeError(1, fmt.Errorf("gormes -z: provider setup failed: %w", errors.New("nil hermes client")))
	}

	toolSafety, err := kernel.NewOneshotToolSafetyPolicy(kernel.OneshotToolSafetyOptions{
		TrustClass: kernel.TrustClassOperator,
	})
	if err != nil {
		return newExitCodeError(1, fmt.Errorf("gormes -z: safety policy setup failed: %w", err))
	}
	kernelCfg := kernel.Config{
		Model:      model,
		Endpoint:   cfg.Hermes.Endpoint,
		Admission:  kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
		ToolAudit:  audit.NewJSONLWriter(config.ToolAuditLogPath()),
		ToolSafety: toolSafety,
	}
	if len(configureKernel) > 0 && configureKernel[0] != nil {
		configureKernel[0](&kernelCfg)
	}
	k := kernel.New(kernelCfg, client, store.NewNoop(), telemetry.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	runDone := make(chan error, 1)
	go func() {
		runDone <- k.Run(rootCtx)
	}()
	defer func() {
		stop()
		select {
		case <-runDone:
		case <-time.After(kernel.ShutdownBudget):
		}
	}()

	initial, err := readOneshotFrame(rootCtx, k.Render())
	if err != nil {
		return newExitCodeError(1, fmt.Errorf("gormes -z: kernel startup failed: %w", err))
	}
	if err := k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: invocation.Prompt}); err != nil {
		return newExitCodeError(1, fmt.Errorf("gormes -z: submit failed: %w", err))
	}
	final, err := waitForOneshotFinalFrame(rootCtx, k.Render(), initial.Seq)
	if err != nil {
		return newExitCodeError(1, fmt.Errorf("gormes -z: kernel turn failed: %w", err))
	}
	if final.LastError != "" {
		return newExitCodeError(1, fmt.Errorf("gormes -z: %s", final.LastError))
	}
	content, ok := finalAssistantContent(final.History)
	if !ok {
		return newExitCodeError(1, errors.New("gormes -z: no final assistant content"))
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), content); err != nil {
		return newExitCodeError(1, fmt.Errorf("gormes -z: write stdout: %w", err))
	}
	return nil
}

func readOneshotFrame(ctx context.Context, frames <-chan kernel.RenderFrame) (kernel.RenderFrame, error) {
	select {
	case frame, ok := <-frames:
		if !ok {
			return kernel.RenderFrame{}, errors.New("render stream closed")
		}
		return frame, nil
	case <-ctx.Done():
		return kernel.RenderFrame{}, ctx.Err()
	}
}

func waitForOneshotFinalFrame(ctx context.Context, frames <-chan kernel.RenderFrame, initialSeq uint64) (kernel.RenderFrame, error) {
	for {
		frame, err := readOneshotFrame(ctx, frames)
		if err != nil {
			return kernel.RenderFrame{}, err
		}
		if frame.LastError != "" || frame.Phase == kernel.PhaseFailed {
			return frame, nil
		}
		if frame.Phase == kernel.PhaseIdle && frame.Seq > initialSeq {
			return frame, nil
		}
	}
}

func finalAssistantContent(history []hermes.Message) (string, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "assistant" {
			return history[i].Content, true
		}
	}
	return "", false
}

func runTUI(cmd *cobra.Command, _ []string) error {
	invocation, err := resolveTUIInvocation(cmd)
	if err != nil {
		return err
	}
	return runResolvedTUI(cmd, invocation)
}

func runResolvedTUI(cmd *cobra.Command, invocation tuiInvocation) error {
	return runResolvedTUIWithRuntime(cmd, invocation, rootRuntime{})
}

type tuiProgram interface {
	Run() (tea.Model, error)
	Quit()
}

type tuiProgramFactory func(tea.Model, ...tea.ProgramOption) tuiProgram

func defaultTUIProgramFactory(model tea.Model, options ...tea.ProgramOption) tuiProgram {
	return tea.NewProgram(model, options...)
}

func runResolvedTUIWithRuntime(cmd *cobra.Command, invocation tuiInvocation, runtime rootRuntime) error {
	runNativeTUIStartupPreflight(context.Background(), tuiStartupPreflightOptions{})
	if runtime.tuiProgramFactory == nil {
		runtime.tuiProgramFactory = defaultTUIProgramFactory
	}

	cfg := invocation.Config
	if p, ok := config.LegacyHermesHome(); ok {
		slog.Info("detected upstream Hermes home — Gormes uses XDG paths and does NOT read state from it; run `gormes migrate --from-hermes` (planned Phase 5.O) to import sessions and memory", "hermes_home", p)
	}

	modelName := invocation.Inference.Model
	if modelName == "" {
		modelName = cfg.Hermes.Model
	}
	c := hermes.NewHTTPClientWithProvider(cfg.Hermes.Endpoint, cfg.Hermes.APIKey, invocation.Inference.Provider)

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

	// Phase 2.C — open the session map; honor --resume.
	smap, err := session.OpenBolt(config.SessionDBPath())
	if err != nil {
		return fmt.Errorf("session map: %w", err)
	}
	defer smap.Close()
	sessionMirror := startSessionIndexMirror(smap, slog.Default())
	defer sessionMirror.Stop()

	resumeFlag, _ := cmd.Flags().GetString("resume")
	pctx := context.Background()
	key := session.TUIKey()
	if resumeFlag != "" {
		if err := smap.Put(pctx, key, resumeFlag); err != nil {
			slog.Warn("failed to apply --resume override", "err", err)
		}
	}
	var initialSID string
	if sid, err := smap.Get(pctx, key); err != nil {
		slog.Warn("could not load initial session_id", "key", key, "err", err)
	} else {
		initialSID = sid
		if sid != "" {
			slog.Info("resuming persisted session", "key", key, "session_id", sid)
		}
	}

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tm := telemetry.New()
	toolAudit := audit.NewJSONLWriter(config.ToolAuditLogPath())
	k := kernel.New(kernel.Config{
		Model:             modelName,
		Endpoint:          cfg.Hermes.Endpoint,
		Admission:         kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
		Tools:             buildDefaultRegistry(rootCtx, cfg.Delegation, cfg.SkillsRoot(), c, modelName),
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
		InitialSessionID:  initialSID,
		ToolAudit:         toolAudit,
	}, c, store.NewNoop(), tm, slog.Default())

	go k.Run(rootCtx)

	// Fan-through: read every frame from the kernel, persist its SessionID
	// when it changes, then forward to the TUI. Single consumer invariant
	// preserved — internal/tui's Model remains the only reader of the
	// downstream channel. Buffered cap 1 matches kernel.RenderMailboxCap.
	hookedFrames := make(chan kernel.RenderFrame, 1)
	go func() {
		defer close(hookedFrames)
		var lastSID string
		raw := k.Render()
		for f := range raw {
			if f.SessionID != lastSID {
				if err := smap.Put(rootCtx, key, f.SessionID); err != nil {
					slog.Warn("tui: failed to persist session_id", "key", key, "err", err)
				} else {
					lastSID = f.SessionID
				}
			}
			select {
			case hookedFrames <- f:
			case <-rootCtx.Done():
				return
			}
		}
	}()

	submit := func(text string) {
		_ = k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: text})
	}
	cancelTurn := func() {
		_ = k.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel})
	}

	model := tui.NewModelWithOptions(hookedFrames, submit, cancelTurn, tui.Options{
		MouseTracking: cfg.TUI.MouseTracking,
	})
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if cfg.TUI.MouseTracking {
		programOptions = append(programOptions, tea.WithMouseAllMotion())
	}
	prog := runtime.tuiProgramFactory(model, programOptions...)

	// Signal → shutdown-budget force-exit watcher.
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
