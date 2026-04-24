package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"unknown"})
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	for _, want := range []string{
		"usage: autoloop",
		"audit",
		"service install",
		"service install-audit",
		"service disable legacy-timers",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run() error = %q, want %q", err, want)
		}
	}
}

func TestRunCommandDryRunPrintsSummary(t *testing.T) {
	repoRoot := t.TempDir()
	progressPath := filepath.Join(repoRoot, "progress.json")
	if err := os.WriteFile(progressPath, []byte(`{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"item_name": "planned CLI candidate", "status": "planned"}
						]
					}
				}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PROGRESS_JSON", progressPath)
	t.Setenv("RUN_ROOT", filepath.Join(repoRoot, "runs"))
	t.Setenv("BACKEND", "opencode")
	t.Setenv("MODE", "safe")
	t.Setenv("MAX_AGENTS", "1")

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"run", "--dry-run"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{"candidates: 1", "selected: 1", "planned CLI candidate"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
}

func TestRunCommandBackendFlagSetsBackend(t *testing.T) {
	repoRoot := t.TempDir()
	progressPath := filepath.Join(repoRoot, "progress.json")
	if err := os.WriteFile(progressPath, []byte(`{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"item_name": "backend flag candidate", "status": "planned"}
						]
					}
				}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PROGRESS_JSON", progressPath)
	t.Setenv("RUN_ROOT", filepath.Join(repoRoot, "runs"))
	t.Setenv("BACKEND", "codexu")
	t.Setenv("MODE", "safe")
	t.Setenv("MAX_AGENTS", "1")

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"run", "--dry-run", "--opencode"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "backend flag candidate") {
		t.Fatalf("stdout = %q, want dry-run summary with backend flag accepted", stdout.String())
	}
}

func TestRunCommandHelpPrintsUsage(t *testing.T) {
	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	if err := run([]string{"run", "--help"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "usage: autoloop") {
		t.Fatalf("stdout = %q, want usage", stdout.String())
	}
}

func TestDigestUsesConfiguredRunRoot(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := autoloop.AppendLedgerEvent(ledgerPath, autoloop.LedgerEvent{
		TS:    time.Now().UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	t.Setenv("AUDIT_DIR", filepath.Join(repoRoot, "audit"))

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"digest"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "runs: 1") {
		t.Fatalf("stdout = %q, want digest from configured RUN_ROOT", stdout.String())
	}
}

func TestDigestOutputWritesFile(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := autoloop.AppendLedgerEvent(ledgerPath, autoloop.LedgerEvent{
		TS:    time.Now().UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	outputPath := filepath.Join(repoRoot, "digest.md")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"digest", "--output", outputPath}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(raw), "runs: 1") {
		t.Fatalf("digest output = %q, want digest", raw)
	}
}

func TestAuditUsesConfiguredRunRoot(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := autoloop.AppendLedgerEvent(ledgerPath, autoloop.LedgerEvent{
		TS:    time.Now().UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	t.Setenv("AUDIT_DIR", filepath.Join(repoRoot, "audit"))

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"audit"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "run_started=1") {
		t.Fatalf("stdout = %q, want audit digest from configured RUN_ROOT", stdout.String())
	}
}

func TestAuditCreatesReportArtifacts(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	auditDir := filepath.Join(repoRoot, "audit")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := autoloop.AppendLedgerEvent(ledgerPath, autoloop.LedgerEvent{
		TS:    time.Now().UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	t.Setenv("AUDIT_DIR", auditDir)

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"audit"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "gormes-orchestrator-audit @") {
		t.Fatalf("stdout = %q, want audit summary", stdout.String())
	}
	for _, name := range []string{"cursor", "report.ndjson", "report.csv"} {
		if _, err := os.Stat(filepath.Join(auditDir, name)); err != nil {
			t.Fatalf("Stat(%s) error = %v", name, err)
		}
	}
}

func TestServiceInstallWritesUnitUnderXDGConfigHome(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("FORCE", "")
	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}, {}}}
	oldRunner := serviceRunner
	serviceRunner = runner
	t.Cleanup(func() {
		serviceRunner = oldRunner
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"service", "install"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	unitPath := filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator.service")
	unit, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(unit), "WorkingDirectory="+repoRoot) {
		t.Fatalf("unit = %q, want workdir %q", unit, repoRoot)
	}
	wantExec := "ExecStart=" + filepath.Join(repoRoot, "scripts", "gormes-auto-codexu-orchestrator.sh")
	if !strings.Contains(string(unit), wantExec) {
		t.Fatalf("unit = %q, want stable wrapper exec %q", unit, wantExec)
	}
	if strings.Contains(string(unit), "go-build") || strings.Contains(string(unit), " run") {
		t.Fatalf("unit = %q, want no temporary go-build path and no extra run arg", unit)
	}

	wantCommands := []autoloop.Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
		{Name: "systemctl", Args: []string{"--user", "enable", "--now", "gormes-orchestrator.service"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestServiceInstallAuditUsesAuditUnitName(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}, {}}}
	oldRunner := serviceRunner
	serviceRunner = runner
	t.Cleanup(func() {
		serviceRunner = oldRunner
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"service", "install-audit"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator-audit.service")); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator-audit.timer")); err != nil {
		t.Fatalf("Stat(timer) error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator-audit.service"))
	if err != nil {
		t.Fatalf("ReadFile(service) error = %v", err)
	}
	wantExec := "ExecStart=" + filepath.Join(repoRoot, "scripts", "orchestrator", "audit.sh")
	if !strings.Contains(string(raw), wantExec) {
		t.Fatalf("service unit = %q, want stable audit wrapper exec %q", raw, wantExec)
	}
	if strings.Contains(string(raw), "go-build") || strings.Contains(string(raw), " run") {
		t.Fatalf("service unit = %q, want no temporary go-build path and no run arg", raw)
	}
	wantEnable := autoloop.Command{Name: "systemctl", Args: []string{"--user", "enable", "--now", "gormes-orchestrator-audit.timer"}}
	if got := runner.Commands[len(runner.Commands)-1]; !reflect.DeepEqual(got, wantEnable) {
		t.Fatalf("last command = %#v, want %#v", got, wantEnable)
	}
}

func TestServiceInstallHonorsAutoStartZero(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("AUTO_START", "0")
	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}}}
	oldRunner := serviceRunner
	serviceRunner = runner
	t.Cleanup(func() {
		serviceRunner = oldRunner
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"service", "install"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	wantCommands := []autoloop.Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestServiceInstallAuditHonorsAutoStartZero(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("AUTO_START", "0")
	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}}}
	oldRunner := serviceRunner
	serviceRunner = runner
	t.Cleanup(func() {
		serviceRunner = oldRunner
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"service", "install-audit"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	wantCommands := []autoloop.Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestServiceDisableLegacyTimersUsesRunner(t *testing.T) {
	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}, {}, {}}}
	oldRunner := serviceRunner
	serviceRunner = runner
	t.Cleanup(func() {
		serviceRunner = oldRunner
	})

	if err := run([]string{"service", "disable", "legacy-timers"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	wantCommands := []autoloop.Command{
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner-tasks-manager.timer"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architectureplanneragent.timer"}},
	}
	if len(runner.Commands) != 3 || !reflect.DeepEqual(runner.Commands[:2], wantCommands) {
		t.Fatalf("commands = %#v, want systemd disables plus cron cleanup", runner.Commands)
	}
	cronCommand := runner.Commands[2]
	if cronCommand.Name != "sh" || len(cronCommand.Args) != 2 || !strings.Contains(cronCommand.Args[1], "landingpage-improver\\.sh") {
		t.Fatalf("cron cleanup command = %#v, want sh cleanup for legacy cron entries", cronCommand)
	}
}

func TestServiceInstallUsesHomeWhenXDGConfigHomeEmpty(t *testing.T) {
	repoRoot := t.TempDir()
	home := filepath.Join(repoRoot, "home")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}, {}}}
	oldRunner := serviceRunner
	serviceRunner = runner
	t.Cleanup(func() {
		serviceRunner = oldRunner
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"service", "install"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "gormes-orchestrator.service")); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
}

func TestDigestIgnoresRunOnlyEnvValidation(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := autoloop.AppendLedgerEvent(ledgerPath, autoloop.LedgerEvent{
		TS:    time.Unix(1, 0).UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	t.Setenv("MAX_AGENTS", "bad")

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run([]string{"digest"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "runs: 1") {
		t.Fatalf("stdout = %q, want digest from configured RUN_ROOT", stdout.String())
	}
}
