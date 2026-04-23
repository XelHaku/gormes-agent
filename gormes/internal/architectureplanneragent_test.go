package internal_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	architectureTaskManagerScript = "gormes-architecture-task-manager.sh"
	legacyPlannerWrapperScript    = "architectureplanneragent.sh"
	architectureTaskManagerUnit   = "gormes-architecture-task-manager"
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
		filepath.Join(repoRoot, "scripts", architectureTaskManagerScript),
		filepath.Join(gormesRepo, "scripts", architectureTaskManagerScript),
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
		filepath.Join(gormesRepo, "cmd", "progress-gen", "main.go"),
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
  "run ./cmd/progress-gen -write") echo "progress-gen: _index.md regenerated" ;;
  "run ./cmd/progress-gen -validate") echo "progress-gen: validated 2 phases" ;;
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
	}, "bash", "scripts/"+architectureTaskManagerScript)

	outputText := string(out)
	for _, want := range []string{
		"1/5 preflight",
		"2/5 context",
		"3/5 planning",
		"4/5 validation",
		"5/5 schedule",
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
	if strings.Contains(outputText, "Upstream commit:") || strings.Contains(outputText, "Local commit:") {
		t.Fatalf("output too verbose for normal progress mode:\n%s", outputText)
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

	tasksPath := filepath.Join(plannerDir, "tasks.md")
	tasksData, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatalf("read tasks.md: %v", err)
	}
	tasksText := string(tasksData)
	for _, want := range []string{
		"# Gormes Architecture Tasks",
		"- Planned: 2",
		"- [ ] Phase 2 / 2.B.4: Pairing, reconnect, and send contract",
		"- [ ] Phase 3 / 3.E.8: parent_session_id lineage for compression splits",
	} {
		if !strings.Contains(tasksText, want) {
			t.Fatalf("tasks.md missing %q:\n%s", want, tasksText)
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
	if !strings.Contains(serviceText, architectureTaskManagerScript) {
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
		"run ./cmd/progress-gen -write",
		"run ./cmd/progress-gen -validate",
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
		{name: "help", args: []string{"--help"}, want: "gormes-architecture-task-manager.sh"},
		{name: "legacy wrapper help", args: []string{"legacy-help"}, want: "gormes-architecture-task-manager.sh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := "scripts/" + architectureTaskManagerScript
			args := tc.args
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
