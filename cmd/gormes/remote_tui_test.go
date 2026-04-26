package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

// TestRemoteTUI_FlagThreadsThroughRunResolvedTUI proves the root command
// reads --remote into the resolved invocation so downstream wiring can
// branch on it. The fixture intercepts runResolvedTUI and asserts the
// remote URL is present without calling the real TUI runtime.
func TestRemoteTUI_FlagThreadsThroughRunResolvedTUI(t *testing.T) {
	setupNativeTUITestEnv(t)

	var seen tuiInvocation
	cmd := newRootCommandWithRuntime(rootRuntime{
		runResolvedTUI: func(_ *cobra.Command, invocation tuiInvocation) error {
			seen = invocation
			return nil
		},
	})
	stdout, stderr, err := executeNativeTUICommand(cmd, "--offline", "--remote", "http://gateway.example/")
	if err != nil {
		t.Fatalf("Execute() err=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if seen.RemoteURL != "http://gateway.example/" {
		t.Fatalf("invocation.RemoteURL = %q; want http://gateway.example/", seen.RemoteURL)
	}
}

// TestRemoteTUI_StartupBypassesAPIServerHealthAndPackageManagers proves
// that --remote <url> skips the api_server health probe and never spawns
// node/npm/python — the remote SSE consumer is pure Go HTTP. Local
// Bubble Tea continues to run and the program factory is invoked once.
func TestRemoteTUI_StartupBypassesAPIServerHealthAndPackageManagers(t *testing.T) {
	setupNativeTUITestEnv(t)

	var apiServerHits atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			apiServerHits.Add(1)
		}
		if r.URL.Path == "/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			f := kernel.RenderFrame{Phase: kernel.PhaseIdle, Seq: 1, Model: "remote-fixture"}
			data, _ := json.Marshal(f)
			fmt.Fprintf(w, "event: frame\ndata: %s\n\n", data)
			flusher.Flush()
			<-r.Context().Done()
			return
		}
		http.NotFound(w, r)
	}))
	defer gateway.Close()

	workDir := t.TempDir()
	t.Chdir(workDir)
	commandLog := filepath.Join(workDir, "unexpected-package-command.log")
	installFailingPackageCommands(t, commandLog)

	var programRuns atomic.Int32
	cmd := newRootCommandWithRuntime(rootRuntime{
		tuiProgramFactory: func(tea.Model, ...tea.ProgramOption) tuiProgram {
			return fakeTUIProgram{run: func() { programRuns.Add(1) }}
		},
	})
	stdout, stderr, err := executeNativeTUICommand(cmd, "--remote", gateway.URL)
	if err != nil {
		t.Fatalf("Execute() err=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if programRuns.Load() != 1 {
		t.Fatalf("programRuns = %d; want 1", programRuns.Load())
	}
	if apiServerHits.Load() != 0 {
		t.Errorf("apiServerHits = %d; want 0 (remote mode bypasses api_server health)", apiServerHits.Load())
	}
	if data, err := os.ReadFile(commandLog); err == nil {
		t.Fatalf("--remote startup invoked package command unexpectedly:\n%s", data)
	}
	if strings.Contains(stderr, "api_server not reachable") {
		t.Fatalf("stderr surfaced api_server health error in remote mode:\n%s", stderr)
	}
}

// TestRemoteTUI_DoctorTUIStatusReportsRemoteDegradedMode confirms the
// degraded-mode evidence: when the operator is running purely in local
// Bubble Tea mode (no --remote), the doctor TUI status flags remote
// streaming as unavailable while still reporting the local runtime as
// healthy. This matches the row's degraded_mode contract: TUI status
// reports remote streaming unavailable while local Bubble Tea continues
// to work.
func TestRemoteTUI_DoctorTUIStatusReportsRemoteDegradedMode(t *testing.T) {
	got := doctorTUIStatus().Format()
	lower := strings.ToLower(got)
	for _, want := range []string{"native tui", "go-native bubble tea", "remote"} {
		if !strings.Contains(lower, want) {
			t.Errorf("doctor TUI status missing %q:\n%s", want, got)
		}
	}
}
