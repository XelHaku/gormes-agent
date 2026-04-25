package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
)

func TestGonchoDoctorCommand_TextZeroStateReportsOperatorLadder(t *testing.T) {
	seedGonchoDoctorZeroStateDB(t)

	stdout, stderr, err := runGonchoDoctorCommand(t, "goncho", "doctor")
	if err != nil {
		t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	for _, want := range []string{
		"Goncho doctor",
		"status: degraded",
		"Config",
		"config_path:",
		"memory_db_path: " + config.MemoryDBPath(),
		"workspace: gormes",
		"observer_peer: gormes",
		"Schema",
		"schema_version: " + memory.CurrentSchemaVersion(),
		"Session catalog",
		"no session catalog data",
		"Tool registration",
		"honcho_context",
		"Context dry-run",
		"No stored representation for operator:diagnostic.",
		"Queue status (observability/debugging only; not synchronization; do not wait for empty queue)",
		"extractor_queue_depth: 0",
		"representation: total=0 pending=0 in_progress=0 completed=0",
		"summary: total=0 pending=0 in_progress=0 completed=0",
		"dream: total=0 pending=0 in_progress=0 completed=0",
		"Conclusion availability",
		"conclusion_count: 0",
		"Summary availability",
		"summary_table: available",
		"summary_count: 0",
		"Provider readiness",
		"optional_provider_checks: degraded",
		"Degraded modes",
		"goncho_task_queue",
		"missing optional model/provider features are degraded",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestGonchoDoctorCommand_JSONZeroStateIsMachineReadable(t *testing.T) {
	seedGonchoDoctorZeroStateDB(t)

	stdout, stderr, err := runGonchoDoctorCommand(t,
		"goncho", "doctor", "--json", "--peer=user-juan", "--session=telegram:1",
	)
	if err != nil {
		t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	var got struct {
		Service  string `json:"service"`
		Status   string `json:"status"`
		ExitCode int    `json:"exit_code"`
		Config   struct {
			MemoryDBPath string `json:"memory_db_path"`
			Workspace    string `json:"workspace"`
			ObserverPeer string `json:"observer_peer"`
		} `json:"config"`
		Schema struct {
			Version string          `json:"version"`
			Tables  map[string]bool `json:"tables"`
		} `json:"schema"`
		ToolRegistration struct {
			Registered []string `json:"registered"`
		} `json:"tool_registration"`
		ContextDryRun struct {
			Peer       string `json:"peer"`
			SessionKey string `json:"session_key"`
		} `json:"context_dry_run"`
		QueueStatus struct {
			ObservabilityOnly bool `json:"observability_only"`
			WorkUnits         map[string]struct {
				TotalWorkUnits int `json:"total_work_units"`
			} `json:"work_units"`
		} `json:"queue_status"`
		ConclusionAvailability struct {
			Total int `json:"total"`
		} `json:"conclusion_availability"`
		SummaryAvailability struct {
			Status       string `json:"status"`
			TablePresent bool   `json:"table_present"`
		} `json:"summary_availability"`
		ProviderReadiness struct {
			Status   string `json:"status"`
			Required bool   `json:"required"`
			Checked  bool   `json:"checked"`
		} `json:"provider_readiness"`
		DegradedModes []struct {
			Capability string `json:"capability"`
			Severity   string `json:"severity"`
		} `json:"degraded_modes"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v\nstdout=%s", err, stdout)
	}

	if got.Service != "goncho" || got.Status != "degraded" || got.ExitCode != 0 {
		t.Fatalf("header = service %q status %q exit %d, want goncho/degraded/0", got.Service, got.Status, got.ExitCode)
	}
	if got.Config.MemoryDBPath != config.MemoryDBPath() || got.Config.Workspace != "gormes" || got.Config.ObserverPeer != "gormes" {
		t.Fatalf("config = %+v", got.Config)
	}
	if got.Schema.Version != memory.CurrentSchemaVersion() || !got.Schema.Tables["goncho_conclusions"] {
		t.Fatalf("schema = %+v", got.Schema)
	}
	if !slices.Contains(got.ToolRegistration.Registered, "honcho_context") {
		t.Fatalf("registered tools = %#v, want honcho_context", got.ToolRegistration.Registered)
	}
	if got.ContextDryRun.Peer != "user-juan" || got.ContextDryRun.SessionKey != "telegram:1" {
		t.Fatalf("context dry-run = %+v", got.ContextDryRun)
	}
	if !got.QueueStatus.ObservabilityOnly {
		t.Fatal("queue_status.observability_only = false, want true")
	}
	for _, taskType := range []string{"representation", "summary", "dream"} {
		if got.QueueStatus.WorkUnits[taskType].TotalWorkUnits != 0 {
			t.Fatalf("%s total_work_units = %d, want zero-state", taskType, got.QueueStatus.WorkUnits[taskType].TotalWorkUnits)
		}
	}
	if got.ConclusionAvailability.Total != 0 {
		t.Fatalf("conclusion total = %d, want 0", got.ConclusionAvailability.Total)
	}
	if got.SummaryAvailability.Status != "zero_state" || !got.SummaryAvailability.TablePresent {
		t.Fatalf("summary availability = %+v, want zero_state present table", got.SummaryAvailability)
	}
	if got.ProviderReadiness.Status != "degraded" || got.ProviderReadiness.Required || got.ProviderReadiness.Checked {
		t.Fatalf("provider readiness = %+v, want optional degraded without network check", got.ProviderReadiness)
	}
	if len(got.DegradedModes) == 0 {
		t.Fatal("degraded_modes empty, want visible degraded capabilities")
	}
}

func TestGonchoDoctorCommand_ExitCodeMapping(t *testing.T) {
	t.Run("healthy_or_degraded_is_zero", func(t *testing.T) {
		seedGonchoDoctorZeroStateDB(t)
		stdout, stderr, err := runGonchoDoctorCommand(t, "goncho", "doctor")
		if code := commandExitCode(err); code != 0 {
			t.Fatalf("exit code = %d, want 0\nstdout=%s\nstderr=%s\nerr=%v", code, stdout, stderr, err)
		}
	})

	t.Run("missing_memory_database_is_client_config_issue", func(t *testing.T) {
		setupGonchoDoctorEnv(t)
		stdout, stderr, err := runGonchoDoctorCommand(t, "goncho", "doctor")
		if code := commandExitCode(err); code != 1 {
			t.Fatalf("exit code = %d, want 1\nstdout=%s\nstderr=%s\nerr=%v", code, stdout, stderr, err)
		}
	})

	t.Run("corrupt_memory_database_is_runtime_storage_issue", func(t *testing.T) {
		setupGonchoDoctorEnv(t)
		if err := os.MkdirAll(filepath.Dir(config.MemoryDBPath()), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(config.MemoryDBPath(), []byte("not sqlite"), 0o644); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, err := runGonchoDoctorCommand(t, "goncho", "doctor")
		if code := commandExitCode(err); code != 2 {
			t.Fatalf("exit code = %d, want 2\nstdout=%s\nstderr=%s\nerr=%v", code, stdout, stderr, err)
		}
	})

	t.Run("required_provider_without_key_is_auth_provider_issue", func(t *testing.T) {
		seedGonchoDoctorZeroStateDB(t)
		stdout, stderr, err := runGonchoDoctorCommand(t, "goncho", "doctor", "--require-provider")
		if code := commandExitCode(err); code != 3 {
			t.Fatalf("exit code = %d, want 3\nstdout=%s\nstderr=%s\nerr=%v", code, stdout, stderr, err)
		}
	})
}

func runGonchoDoctorCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var coded interface {
		ExitCode() int
	}
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return 1
}

func seedGonchoDoctorZeroStateDB(t *testing.T) {
	t.Helper()
	setupGonchoDoctorEnv(t)

	store, err := memory.OpenSqlite(config.MemoryDBPath(), 8, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	if err := store.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func setupGonchoDoctorEnv(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HERMES_HOME", filepath.Join(root, "hermes"))
	t.Setenv("GORMES_API_KEY", "")
}
