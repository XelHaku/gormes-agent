package internal_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	architecturePlannerTasksManagerScript = "gormes-architecture-planner-tasks-manager.sh"
	architectureTaskManagerWrapperScript  = "gormes-architecture-task-manager.sh"
	legacyPlannerWrapperScript            = "architectureplanneragent.sh"
	documentationImproverScript           = "documentation-improver.sh"
	architectureTaskManagerUnit           = "gormes-architecture-planner-tasks-manager"
)

func TestArchitecturePlannerAgentRunsCodexuAndInstallsPeriodicTimer(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	repoRoot := filepath.Dir(filepath.Dir(file))

	tmpRoot := t.TempDir()
	parentRepo := filepath.Join(tmpRoot, "golang-hermes-agent")
	gormesRepo := filepath.Join(parentRepo, "gormes")

	copyPlannerTestFile(t,
		filepath.Join(repoRoot, "scripts", architecturePlannerTasksManagerScript),
		filepath.Join(gormesRepo, "scripts", architecturePlannerTasksManagerScript),
		0o755,
	)
	copyPlannerTestFile(t,
		filepath.Join(repoRoot, "scripts", architectureTaskManagerWrapperScript),
		filepath.Join(gormesRepo, "scripts", architectureTaskManagerWrapperScript),
		0o755,
	)
	copyPlannerTestFile(t,
		filepath.Join(repoRoot, "scripts", legacyPlannerWrapperScript),
		filepath.Join(gormesRepo, "scripts", legacyPlannerWrapperScript),
		0o755,
	)

	writePlannerTestFile(t,
		filepath.Join(parentRepo, "run_agent.py"),
		[]byte("print('upstream hermes stub')\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(parentRepo, "gateway", "platforms", ".keep"),
		[]byte{},
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(parentRepo, "tools", "registry.py"),
		[]byte("REGISTRY = {}\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(parentRepo, "tests", "e2e", ".keep"),
		[]byte{},
		0o644,
	)

	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "docs", "content", "building-gormes", "architecture_plan", "progress.json"),
		[]byte("{\n  \"meta\": {\"version\": \"2.0\", \"last_updated\": \"2026-04-22\", \"links\": {}},\n  \"phases\": {\n    \"2\": {\n      \"name\": \"Phase 2\",\n      \"deliverable\": \"Gateway\",\n      \"subphases\": {\n        \"2.B.4\": {\n          \"name\": \"WhatsApp Adapter\",\n          \"priority\": \"P1\",\n          \"items\": [\n            {\"name\": \"Pairing, reconnect, and send contract\", \"status\": \"planned\"}\n          ]\n        }\n      }\n    },\n    \"3\": {\n      \"name\": \"Phase 3\",\n      \"deliverable\": \"Memory\",\n      \"subphases\": {\n        \"3.E.8\": {\n          \"name\": \"Identity + Lineage\",\n          \"priority\": \"P0\",\n          \"items\": [\n            {\"name\": \"parent_session_id lineage for compression splits\", \"status\": \"planned\"}\n          ]\n        }\n      }\n    }\n  }\n}\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "docs", "content", "building-gormes", "architecture_plan", "_index.md"),
		[]byte("# Architecture Plan\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "docs", "content", "building-gormes", "architecture_plan", "phase-2-gateway.md"),
		[]byte("# Phase 2\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "docs", "content", "building-gormes", "architecture_plan", "phase-3-memory.md"),
		[]byte("# Phase 3\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "docs", "content", "building-gormes", "architecture_plan", "subsystem-inventory.md"),
		[]byte("# Inventory\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "docs", "content", "building-gormes", "core-systems", "gateway.md"),
		[]byte("# Gateway\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "docs", "content", "building-gormes", "core-systems", "memory.md"),
		[]byte("# Memory\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "cmd", "autoloop", "progress.go"),
		[]byte("package main\nfunc main() {}\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "internal", "progress", "doc.go"),
		[]byte("package progress\n"),
		0o644,
	)
	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "docs", "doc.go"),
		[]byte("package docs\n"),
		0o644,
	)

	runPlannerTestCommand(t, parentRepo, "git", "init")
	runPlannerTestCommand(t, parentRepo, "git", "config", "user.name", "Test User")
	runPlannerTestCommand(t, parentRepo, "git", "config", "user.email", "test@example.com")
	runPlannerTestCommand(t, parentRepo, "git", "add", ".")
	runPlannerTestCommand(t, parentRepo, "git", "commit", "-m", "init")

	binDir := filepath.Join(tmpRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	writePlannerTestFile(t, filepath.Join(binDir, "codexu"), []byte(`#!/usr/bin/env bash
set -euo pipefail
log_file="${CODEXU_LOG:?}"
printf '%q ' "$@" >> "$log_file"
printf '\n' >> "$log_file"
final_file=""
for ((i=1; i<=$#; i++)); do
  if [[ "${!i}" == "--output-last-message" ]]; then
    next=$((i + 1))
    final_file="${!next}"
    break
  fi
done
if [[ -z "$final_file" ]]; then
  echo "missing --output-last-message" >&2
  exit 1
fi
cat > "$final_file" <<'EOF'
1. **Scope scanned**

2. **Upstream Hermes delta summary**

3. **Our repo status summary**

4. **Plan quality problems**

5. **Proposed changes**

6. **Actual changes written**

7. **Recommended next execution tasks**

8. **Risks / ambiguities**
EOF
printf '{"type":"thread.started","thread_id":"thread-123"}\n'
`), 0o755)

	writePlannerTestFile(t, filepath.Join(binDir, "go"), []byte(`#!/usr/bin/env bash
set -euo pipefail
log_file="${GO_LOG:?}"
printf '%q ' "$@" >> "$log_file"
printf '\n' >> "$log_file"
case "$*" in
  "run ./cmd/autoloop progress write") echo "progress: _index.md regenerated" ;;
  "run ./cmd/autoloop progress validate") echo "progress: validated 2 phases" ;;
  "test ./internal/progress -count=1") echo "ok github.com/example/internal/progress 0.001s" ;;
  "test ./docs -count=1") echo "ok github.com/example/docs 0.001s" ;;
  *) echo "unexpected go invocation: $*" >&2; exit 1 ;;
esac
`), 0o755)

	writePlannerTestFile(t, filepath.Join(binDir, "systemctl"), []byte(`#!/usr/bin/env bash
set -euo pipefail
log_file="${SYSTEMCTL_LOG:?}"
printf '%q ' "$@" >> "$log_file"
printf '\n' >> "$log_file"
exit 0
`), 0o755)

	logPath := filepath.Join(tmpRoot, "codexu.log")
	goLogPath := filepath.Join(tmpRoot, "go.log")
	systemctlLogPath := filepath.Join(tmpRoot, "systemctl.log")
	xdgConfigHome := filepath.Join(tmpRoot, "xdg")
	homeDir := filepath.Join(tmpRoot, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(xdgConfigHome, 0o755); err != nil {
		t.Fatalf("mkdir xdg: %v", err)
	}

	out := runPlannerTestCommandEnv(t, gormesRepo, []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"CODEXU_LOG=" + logPath,
		"GO_LOG=" + goLogPath,
		"SYSTEMCTL_LOG=" + systemctlLogPath,
		"HOME=" + homeDir,
		"XDG_CONFIG_HOME=" + xdgConfigHome,
	}, "bash", "scripts/"+architecturePlannerTasksManagerScript)

	outputText := string(out)
	for _, want := range []string{
		"1/5 preflight",
		"Repo root:",
		"Upstream Hermes:",
		"Upstream commit:",
		"Local commit:",
		"2/5 context",
		"Progress items:",
		"Architecture docs:",
		"Task Markdown:",
		"3/5 planning",
		"4/5 validation",
		"Validation log:",
		"5/5 schedule",
		"Schedule method: systemd",
	} {
		if !strings.Contains(outputText, want) {
			t.Fatalf("output missing progress checkpoint %q:\n%s", want, outputText)
		}
	}
	if !strings.Contains(string(out), "Planner report:") {
		t.Fatalf("output missing planner report line:\n%s", string(out))
	}
	if !strings.Contains(string(out), "Planner state:") {
		t.Fatalf("output missing planner state line:\n%s", string(out))
	}
	if !strings.Contains(string(out), "Periodic schedule: systemd") {
		t.Fatalf("output missing periodic schedule line:\n%s", string(out))
	}

	rawCodexuLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read codexu log: %v", err)
	}
	codexuLog := string(rawCodexuLog)
	if !strings.Contains(codexuLog, "exec") || !strings.Contains(codexuLog, "--json") {
		t.Fatalf("codexu log missing exec/json:\n%s", codexuLog)
	}
	if strings.Contains(codexuLog, " codex ") {
		t.Fatalf("unexpected codex command in log:\n%s", codexuLog)
	}

	plannerDir := filepath.Join(gormesRepo, ".codex", "planner")
	reportPath := filepath.Join(plannerDir, "latest_planner_report.md")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read planner report: %v", err)
	}
	reportText := string(reportData)
	for _, section := range []string{
		"# Architecture Planner Run",
		"1. **Scope scanned**",
		"2. **Upstream Hermes delta summary**",
		"8. **Risks / ambiguities**",
	} {
		if !strings.Contains(reportText, section) {
			t.Fatalf("planner report missing %q:\n%s", section, reportText)
		}
	}

	statePath := filepath.Join(plannerDir, "planner_state.json")
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read planner_state.json: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("parse planner_state.json: %v", err)
	}
	if got := state["schedule_method"]; got != "systemd" {
		t.Fatalf("schedule_method = %#v, want %q", got, "systemd")
	}
	if got := state["upstream_hermes_dir"]; got != parentRepo {
		t.Fatalf("upstream_hermes_dir = %#v, want %q", got, parentRepo)
	}

	tasksPath := filepath.Join(plannerDir, "architecture-planner-tasks.md")
	tasksData, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatalf("read architecture-planner-tasks.md: %v", err)
	}
	tasksText := string(tasksData)
	for _, want := range []string{
		"# Gormes Architecture Planner Tasks",
		"- Planned: 2",
		"- [ ] Phase 2 / 2.B.4: Pairing, reconnect, and send contract",
		"- [ ] Phase 3 / 3.E.8: parent_session_id lineage for compression splits",
	} {
		if !strings.Contains(tasksText, want) {
			t.Fatalf("architecture-planner-tasks.md missing %q:\n%s", want, tasksText)
		}
	}

	timerPath := filepath.Join(xdgConfigHome, "systemd", "user", architectureTaskManagerUnit+".timer")
	timerData, err := os.ReadFile(timerPath)
	if err != nil {
		t.Fatalf("read timer unit: %v", err)
	}
	timerText := string(timerData)
	if !strings.Contains(timerText, "OnUnitActiveSec=4h") {
		t.Fatalf("timer unit missing interval:\n%s", timerText)
	}

	servicePath := filepath.Join(xdgConfigHome, "systemd", "user", architectureTaskManagerUnit+".service")
	serviceData, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("read service unit: %v", err)
	}
	serviceText := string(serviceData)
	if !strings.Contains(serviceText, architecturePlannerTasksManagerScript) {
		t.Fatalf("service unit missing script path:\n%s", serviceText)
	}
	if !strings.Contains(serviceText, gormesRepo) {
		t.Fatalf("service unit missing working directory:\n%s", serviceText)
	}

	goLogData, err := os.ReadFile(goLogPath)
	if err != nil {
		t.Fatalf("read go log: %v", err)
	}
	goLog := string(goLogData)
	for _, want := range []string{
		"run ./cmd/autoloop progress write",
		"run ./cmd/autoloop progress validate",
		"test ./internal/progress -count=1",
		"test ./docs -count=1",
	} {
		if !strings.Contains(goLog, want) {
			t.Fatalf("go log missing %q:\n%s", want, goLog)
		}
	}

	systemctlData, err := os.ReadFile(systemctlLogPath)
	if err != nil {
		t.Fatalf("read systemctl log: %v", err)
	}
	systemctlLog := string(systemctlData)
	for _, want := range []string{
		"--user daemon-reload",
		"--user enable --now " + architectureTaskManagerUnit + ".timer",
	} {
		if !strings.Contains(systemctlLog, want) {
			t.Fatalf("systemctl log missing %q:\n%s", want, systemctlLog)
		}
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "status", args: []string{"status"}, want: "Last run UTC:"},
		{name: "show report", args: []string{"show-report"}, want: "# Architecture Planner Run"},
		{name: "doctor", args: []string{"doctor"}, want: "doctor: ok"},
		{name: "help", args: []string{"--help"}, want: "gormes-architecture-planner-tasks-manager.sh"},
		{name: "task manager wrapper help", args: []string{"task-manager-help"}, want: "gormes-architecture-planner-tasks-manager.sh"},
		{name: "legacy wrapper help", args: []string{"legacy-help"}, want: "gormes-architecture-planner-tasks-manager.sh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := "scripts/" + architecturePlannerTasksManagerScript
			args := tc.args
			if len(args) == 1 && args[0] == "task-manager-help" {
				script = "scripts/" + architectureTaskManagerWrapperScript
				args = []string{"--help"}
			}
			if len(args) == 1 && args[0] == "legacy-help" {
				script = "scripts/" + legacyPlannerWrapperScript
				args = []string{"--help"}
			}
			out := runPlannerTestCommandEnv(t, gormesRepo, []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"HOME=" + homeDir,
				"XDG_CONFIG_HOME=" + xdgConfigHome,
			}, "bash", append([]string{script}, args...)...)
			if !strings.Contains(string(out), tc.want) {
				t.Fatalf("%s output missing %q:\n%s", tc.name, tc.want, string(out))
			}
		})
	}
}

func TestDocumentationImproverRunsAndWritesState(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	repoRoot := filepath.Dir(filepath.Dir(file))

	tmpRoot := t.TempDir()
	gormesRepo := filepath.Join(tmpRoot, "gormes")

	copyPlannerTestFile(t,
		filepath.Join(repoRoot, "scripts", documentationImproverScript),
		filepath.Join(gormesRepo, "scripts", documentationImproverScript),
		0o755,
	)

	writePlannerTestFile(t,
		filepath.Join(gormesRepo, "docs", "content", "building-gormes", "architecture_plan", "progress.json"),
		[]byte("{\n  \"phases\": {\n    \"2\": {\n      \"name\": \"Phase 2\",\n      \"subphases\": {\n        \"2.F\": {\n          \"items\": [{\"name\": \"Channel directory persistence + lookup contract\", \"status\": \"in_progress\"}]\n        }\n      }\n    }\n  }\n}\n"),
		0o644,
	)
	writePlannerTestFile(t, filepath.Join(gormesRepo, "docs", "content", "building-gormes", "architecture_plan", "_index.md"), []byte("# Architecture Plan\n"), 0o644)
	writePlannerTestFile(t, filepath.Join(gormesRepo, "docs", "content", "building-gormes", "core-systems", "gateway.md"), []byte("# Gateway\n"), 0o644)
	writePlannerTestFile(t, filepath.Join(gormesRepo, "www.gormes.ai", "content", "_index.md"), []byte("# Gormes site\n"), 0o644)
	writePlannerTestFile(t, filepath.Join(gormesRepo, "www.gormes.ai", "internal", "site", "data", "progress.json"), []byte("{}\n"), 0o644)
	writePlannerTestFile(t, filepath.Join(gormesRepo, "cmd", "autoloop", "progress.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	writePlannerTestFile(t, filepath.Join(gormesRepo, "internal", "progress", "doc.go"), []byte("package progress\n"), 0o644)
	writePlannerTestFile(t, filepath.Join(gormesRepo, "docs", "doc.go"), []byte("package docs\n"), 0o644)

	runPlannerTestCommand(t, gormesRepo, "git", "init")
	runPlannerTestCommand(t, gormesRepo, "git", "config", "user.name", "Test User")
	runPlannerTestCommand(t, gormesRepo, "git", "config", "user.email", "test@example.com")
	runPlannerTestCommand(t, gormesRepo, "git", "add", ".")
	runPlannerTestCommand(t, gormesRepo, "git", "commit", "-m", "init")

	binDir := filepath.Join(tmpRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	writePlannerTestFile(t, filepath.Join(binDir, "codexu"), []byte(`#!/usr/bin/env bash
set -euo pipefail
log_file="${CODEXU_LOG:?}"
printf '%q ' "$@" >> "$log_file"
printf '\n' >> "$log_file"
final_file=""
for ((i=1; i<=$#; i++)); do
  if [[ "${!i}" == "--output-last-message" ]]; then
    next=$((i + 1))
    final_file="${!next}"
    break
  fi
done
if [[ -z "$final_file" ]]; then
  echo "missing --output-last-message" >&2
  exit 1
fi
cat > "$final_file" <<'EOF'
1) Scope and baseline
2) Feature/doc drift found
3) Documentation updates applied
4) Website updates applied
5) README + install.sh updates
6) Validation evidence
7) Risks / follow-ups
EOF
printf '{"type":"thread.started","thread_id":"thread-docs-123"}\n'
`), 0o755)

	writePlannerTestFile(t, filepath.Join(binDir, "go"), []byte(`#!/usr/bin/env bash
set -euo pipefail
log_file="${GO_LOG:?}"
printf '%q ' "$@" >> "$log_file"
printf '\n' >> "$log_file"
case "$*" in
  "run ./cmd/autoloop progress write") echo "progress: _index.md regenerated" ;;
  "run ./cmd/autoloop progress validate") echo "progress: validated 2 phases" ;;
  "test ./internal/progress -count=1") echo "ok github.com/example/internal/progress 0.001s" ;;
  "test ./docs -count=1") echo "ok github.com/example/docs 0.001s" ;;
  *) echo "unexpected go invocation: $*" >&2; exit 1 ;;
esac
`), 0o755)

	logPath := filepath.Join(tmpRoot, "codexu.log")
	goLogPath := filepath.Join(tmpRoot, "go.log")

	out := runPlannerTestCommandEnv(t, gormesRepo, []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"CODEXU_LOG=" + logPath,
		"GO_LOG=" + goLogPath,
	}, "bash", "scripts/"+documentationImproverScript)

	if !strings.Contains(string(out), "Documentation report:") {
		t.Fatalf("output missing report path:\n%s", string(out))
	}
	if !strings.Contains(string(out), "Documentation state:") {
		t.Fatalf("output missing state path:\n%s", string(out))
	}
	for _, want := range []string{
		"Run UTC:",
		"Repo root:",
		"Doc root:",
		"Step 1/7: checking prerequisites",
		"Step 2/7: collecting documentation context",
		"Step 3/7: writing Codex prompt",
		"Step 4/7: running Codex documentation pass",
		"Step 5/7: validating docs/progress artifacts",
		"Step 6/7: writing final report",
		"Step 7/7: writing state",
		"Codex session: thread-docs-123",
		"Validation log:",
		"Complete.",
	} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("verbose output missing %q:\n%s", want, string(out))
		}
	}

	docRoot := filepath.Join(gormesRepo, ".codex", "doc-improver")
	statePath := filepath.Join(docRoot, "documentation_state.json")
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read documentation_state.json: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("parse documentation_state.json: %v", err)
	}
	if got := state["session_id"]; got != "thread-docs-123" {
		t.Fatalf("session_id = %#v, want %q", got, "thread-docs-123")
	}

	reportPath := filepath.Join(docRoot, "latest_documentation_report.md")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	reportText := string(reportData)
	if !strings.Contains(reportText, "# Documentation Improver Run") {
		t.Fatalf("report missing header:\n%s", reportText)
	}
	if !strings.Contains(reportText, "1) Scope and baseline") {
		t.Fatalf("report missing required section:\n%s", reportText)
	}

	rawCodexuLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read codexu log: %v", err)
	}
	if !strings.Contains(string(rawCodexuLog), "--output-last-message") {
		t.Fatalf("codexu log missing output-last-message:\n%s", string(rawCodexuLog))
	}

	goLogData, err := os.ReadFile(goLogPath)
	if err != nil {
		t.Fatalf("read go log: %v", err)
	}
	goLog := string(goLogData)
	for _, want := range []string{
		"run ./cmd/autoloop progress write",
		"run ./cmd/autoloop progress validate",
		"test ./internal/progress -count=1",
		"test ./docs -count=1",
	} {
		if !strings.Contains(goLog, want) {
			t.Fatalf("go log missing %q:\n%s", want, goLog)
		}
	}
}

func TestDocumentationImproverReportsActiveLockOwner(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	repoRoot := filepath.Dir(filepath.Dir(file))

	tmpRoot := t.TempDir()
	gormesRepo := filepath.Join(tmpRoot, "gormes")

	copyPlannerTestFile(t,
		filepath.Join(repoRoot, "scripts", documentationImproverScript),
		filepath.Join(gormesRepo, "scripts", documentationImproverScript),
		0o755,
	)

	lockDir := filepath.Join(gormesRepo, ".codex", "doc-improver", "run.lock")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("mkdir lock: %v", err)
	}
	pid := strconv.Itoa(os.Getpid())
	writePlannerTestFile(t, filepath.Join(lockDir, "pid"), []byte(pid+"\n"), 0o644)
	writePlannerTestFile(t, filepath.Join(lockDir, "started_at"), []byte("2026-04-23T00:00:00Z\n"), 0o644)
	writePlannerTestFile(t, filepath.Join(lockDir, "command"), []byte("documentation-improver.sh run\n"), 0o644)

	cmd := exec.Command("bash", "scripts/"+documentationImproverScript)
	cmd.Dir = gormesRepo
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("documentation improver unexpectedly acquired active lock:\n%s", string(out))
	}
	output := string(out)
	for _, want := range []string{
		"active documentation-improver run",
		"PID: " + pid,
		"Started: 2026-04-23T00:00:00Z",
		"Command: documentation-improver.sh run",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("active-lock output missing %q:\n%s", want, output)
		}
	}
}

func copyPlannerTestFile(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func writePlannerTestFile(t *testing.T, dst string, data []byte, mode os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func runPlannerTestCommand(t *testing.T, dir string, name string, args ...string) []byte {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\noutput:\n%s", name, args, err, string(out))
	}
	return out
}

func runPlannerTestCommandEnv(t *testing.T, dir string, env []string, name string, args ...string) []byte {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\noutput:\n%s", name, args, err, string(out))
	}
	return out
}
