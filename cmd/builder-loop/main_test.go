package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"unknown"})
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	for _, want := range []string{
		"usage: builder-loop",
		"progress validate",
		"progress write",
		"repo benchmark record",
		"repo readme update",
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

func TestProgressValidateValidatesCanonicalProgress(t *testing.T) {
	repoRoot := t.TempDir()
	writeMinimalProgressRepo(t, repoRoot)
	withTempCwd(t, repoRoot)

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	if err := run([]string{"progress", "validate"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "progress: validated 1 phases") {
		t.Fatalf("stdout = %q, want validation summary", stdout.String())
	}
}

func TestProgressWriteRegeneratesDocsAndSiteProgress(t *testing.T) {
	repoRoot := t.TempDir()
	writeMinimalProgressRepo(t, repoRoot)
	withTempCwd(t, repoRoot)

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	if err := run([]string{"progress", "write"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	indexRaw, err := os.ReadFile(filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "_index.md"))
	if err != nil {
		t.Fatalf("ReadFile(_index.md) error = %v", err)
	}
	if !strings.Contains(string(indexRaw), "Phase 1 — Test Phase") || !strings.Contains(string(indexRaw), "- [x] First shipped item") {
		t.Fatalf("_index.md was not regenerated with progress content:\n%s", indexRaw)
	}

	siteRaw, err := os.ReadFile(filepath.Join(repoRoot, "www.gormes.ai", "internal", "site", "data", "progress.json"))
	if err != nil {
		t.Fatalf("ReadFile(site progress) error = %v", err)
	}
	sourceRaw, err := os.ReadFile(filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json"))
	if err != nil {
		t.Fatalf("ReadFile(source progress) error = %v", err)
	}
	if string(siteRaw) != string(sourceRaw) {
		t.Fatalf("site progress mirror mismatch")
	}
	if !strings.Contains(stdout.String(), "progress: _index.md regenerated") {
		t.Fatalf("stdout = %q, want generation summary", stdout.String())
	}
}

func TestRepoBenchmarkRecordUpdatesBenchmarks(t *testing.T) {
	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "-c", "user.email=test@example.com", "-c", "user.name=Test User", "commit", "-m", "initial")

	bin := filepath.Join(repoRoot, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, make([]byte, 1024*1024), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "benchmarks.json"), []byte(`{"binary":{"name":"gormes"},"history":[]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withTempCwd(t, repoRoot)

	if err := run([]string{"repo", "benchmark", "record"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	var got struct {
		Binary struct {
			Name      string `json:"name"`
			SizeMB    string `json:"size_mb"`
			SizeBytes int64  `json:"size_bytes"`
			Commit    string `json:"commit"`
		} `json:"binary"`
		History []map[string]any `json:"history"`
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Binary.Name != "gormes" || got.Binary.SizeMB != "1.0" || got.Binary.SizeBytes != 1024*1024 || got.Binary.Commit == "" {
		t.Fatalf("binary = %+v", got.Binary)
	}
	if len(got.History) != 1 {
		t.Fatalf("history = %+v", got.History)
	}
}

func TestRepoReadmeUpdateUpdatesReadme(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "benchmarks.json"), []byte(`{"binary":{"size_mb":"16.2"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(readme, []byte("Binary size: ~99.9 MB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withTempCwd(t, repoRoot)

	if err := run([]string{"repo", "readme", "update"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	raw, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "~16.2 MB") {
		t.Fatalf("README not updated:\n%s", raw)
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
							{
								"item_name": "planned CLI candidate",
								"status": "planned",
								"priority": "P0",
								"contract": "CLI execution contract",
								"contract_status": "draft",
								"slice_size": "small",
								"execution_owner": "orchestrator",
								"ready_when": ["dry-run fixture exists"],
								"write_scope": ["cmd/builder-loop/"],
								"test_commands": ["go test ./cmd/builder-loop -count=1"],
								"done_signal": ["dry-run output names metadata"]
							}
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
	t.Setenv("MAX_PHASE", "12")

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
	for _, want := range []string{
		"candidates: 1",
		"selected: 1",
		"planned CLI candidate",
		"owner=orchestrator",
		"size=small",
		"reason=P0 handoff",
	} {
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
							{"item_name": "backend flag candidate", "status": "planned", "contract": "backend flag contract", "contract_status": "draft"}
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
	t.Setenv("MAX_PHASE", "12")

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

	if !strings.Contains(stdout.String(), "usage: builder-loop") {
		t.Fatalf("stdout = %q, want usage", stdout.String())
	}
}

func TestAutoloopEnvIncludesPlannerTriggersPath(t *testing.T) {
	t.Setenv("PLANNER_TRIGGERS_PATH", filepath.Join(t.TempDir(), "triggers.jsonl"))

	env := autoloopEnv()

	if got, want := env["PLANNER_TRIGGERS_PATH"], os.Getenv("PLANNER_TRIGGERS_PATH"); got != want {
		t.Fatalf("PLANNER_TRIGGERS_PATH = %q, want %q", got, want)
	}
}

func TestDigestUsesConfiguredRunRoot(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
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
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
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
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
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
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
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
	runner := &builderloop.FakeRunner{Results: []builderloop.Result{{}, {}}}
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

	wantCommands := []builderloop.Command{
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
	runner := &builderloop.FakeRunner{Results: []builderloop.Result{{}, {}}}
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
	wantEnable := builderloop.Command{Name: "systemctl", Args: []string{"--user", "enable", "--now", "gormes-orchestrator-audit.timer"}}
	if got := runner.Commands[len(runner.Commands)-1]; !reflect.DeepEqual(got, wantEnable) {
		t.Fatalf("last command = %#v, want %#v", got, wantEnable)
	}
}

func TestServiceInstallHonorsAutoStartZero(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("AUTO_START", "0")
	runner := &builderloop.FakeRunner{Results: []builderloop.Result{{}}}
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

	wantCommands := []builderloop.Command{
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
	runner := &builderloop.FakeRunner{Results: []builderloop.Result{{}}}
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

	wantCommands := []builderloop.Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestServiceDisableLegacyTimersUsesRunner(t *testing.T) {
	runner := &builderloop.FakeRunner{Results: []builderloop.Result{{}, {}, {}}}
	oldRunner := serviceRunner
	serviceRunner = runner
	t.Cleanup(func() {
		serviceRunner = oldRunner
	})

	if err := run([]string{"service", "disable", "legacy-timers"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	wantCommands := []builderloop.Command{
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
	runner := &builderloop.FakeRunner{Results: []builderloop.Result{{}, {}}}
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
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
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

func writeMinimalProgressRepo(t *testing.T, root string) {
	t.Helper()

	progressPath := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	progressJSON := `{
  "meta": {
    "version": "2.0",
    "last_updated": "2026-04-24"
  },
  "phases": {
    "1": {
      "name": "Phase 1 — Test Phase",
      "deliverable": "Test deliverable",
      "subphases": {
        "1.A": {
          "name": "Test Subphase",
          "items": [
            {"name": "First shipped item", "status": "complete"}
          ]
        }
      }
    }
  }
}
`
	if err := os.MkdirAll(filepath.Dir(progressPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(progressPath, []byte(progressJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	markers := map[string]string{
		"README.md": "readme-rollup",
		"docs/content/building-gormes/architecture_plan/_index.md":  "docs-full-checklist",
		"docs/content/building-gormes/contract-readiness.md":        "contract-readiness",
		"docs/content/building-gormes/autoloop/autoloop-handoff.md": "autoloop-handoff",
		"docs/content/building-gormes/autoloop/agent-queue.md":      "agent-queue",
		"docs/content/building-gormes/autoloop/next-slices.md":      "next-slices",
		"docs/content/building-gormes/autoloop/blocked-slices.md":   "blocked-slices",
		"docs/content/building-gormes/autoloop/umbrella-cleanup.md": "umbrella-cleanup",
		"docs/content/building-gormes/autoloop/progress-schema.md":  "progress-schema",
	}
	for rel, kind := range markers {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "before\n<!-- PROGRESS:START kind=" + kind + " -->\nstale\n<!-- PROGRESS:END -->\nafter\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "www.gormes.ai", "internal", "site", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

func withTempCwd(t *testing.T, dir string) {
	t.Helper()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}
