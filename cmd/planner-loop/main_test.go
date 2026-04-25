package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/cmdrunner"
	"github.com/TrebuchetDynamics/gormes-agent/internal/plannerloop"
)

func TestRunDryRunPrintsPlannerSummary(t *testing.T) {
	repoRoot := writeCommandFixture(t)
	t.Setenv("RUN_ROOT", filepath.Join(repoRoot, ".codex", "planner"))

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	withWorkingDir(t, repoRoot)

	if err := run(context.Background(), []string{"run", "--dry-run"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"architecture planner dry-run",
		"backend: codexu",
		"progress items: 1",
		"docs/content/building-gormes/architecture_plan/progress.json",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
}

func TestRunBackendFlagUsesClaudeu(t *testing.T) {
	repoRoot := writeCommandFixture(t)
	t.Setenv("RUN_ROOT", filepath.Join(repoRoot, ".codex", "planner"))
	t.Setenv("PLANNER_VALIDATE", "0")
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}, {}, {}, {}}}
	oldRunner := commandRunner
	commandRunner = runner
	t.Cleanup(func() {
		commandRunner = oldRunner
	})

	withWorkingDir(t, repoRoot)

	if err := run(context.Background(), []string{"run", "--backend", "claudeu"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if got, want := len(runner.Commands), 4; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	if runner.Commands[3].Name != "claudeu" {
		t.Fatalf("Command.Name = %q, want claudeu", runner.Commands[3].Name)
	}
}

func TestRunStatusAndShowReportUseConfiguredRunRoot(t *testing.T) {
	repoRoot := writeCommandFixture(t)
	runRoot := filepath.Join(repoRoot, ".codex", "planner")
	t.Setenv("RUN_ROOT", runRoot)
	t.Setenv("PLANNER_VALIDATE", "0")
	withWorkingDir(t, repoRoot)

	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}, {}, {}, {}}}
	oldRunner := commandRunner
	commandRunner = runner
	t.Cleanup(func() {
		commandRunner = oldRunner
	})
	if err := run(context.Background(), []string{"run"}); err != nil {
		t.Fatalf("run() setup error = %v", err)
	}

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	if err := run(context.Background(), []string{"status"}); err != nil {
		t.Fatalf("status error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Last run UTC:") {
		t.Fatalf("status output missing last run:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := run(context.Background(), []string{"show-report"}); err != nil {
		t.Fatalf("show-report error = %v", err)
	}
	if !strings.Contains(stdout.String(), "# Architecture Planner Loop Run") {
		t.Fatalf("show-report output missing report:\n%s", stdout.String())
	}
}

func TestServiceInstallWritesUnits(t *testing.T) {
	repoRoot := writeCommandFixture(t)
	xdg := filepath.Join(repoRoot, ".config")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("AUTO_START", "0")
	t.Setenv("PLANNER_INTERVAL", "6h")
	withWorkingDir(t, repoRoot)

	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}}}
	oldRunner := commandRunner
	commandRunner = runner
	t.Cleanup(func() { commandRunner = oldRunner })

	if err := run(context.Background(), []string{"service", "install"}); err != nil {
		t.Fatalf("service install error = %v", err)
	}

	servicePath := filepath.Join(xdg, "systemd", "user", "gormes-planner-loop.service")
	timerPath := filepath.Join(xdg, "systemd", "user", "gormes-planner-loop.timer")
	for _, path := range []string{servicePath, timerPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected unit at %s: %v", path, err)
		}
	}

	timerBody, err := os.ReadFile(timerPath)
	if err != nil {
		t.Fatalf("read timer: %v", err)
	}
	if !strings.Contains(string(timerBody), "OnUnitActiveSec=6h") {
		t.Fatalf("timer body missing 6h cadence:\n%s", timerBody)
	}

	if got, want := len(runner.Commands), 1; got != want {
		t.Fatalf("Commands length = %d, want %d (daemon-reload only when AUTO_START=0)", got, want)
	}
	if runner.Commands[0].Name != "systemctl" || runner.Commands[0].Args[0] != "--user" || runner.Commands[0].Args[1] != "daemon-reload" {
		t.Fatalf("Commands[0] = %#v, want systemctl --user daemon-reload", runner.Commands[0])
	}
}

func TestParseRunOptions_PositionalKeywords(t *testing.T) {
	opts, err := parseRunOptions([]string{"--backend", "codexu", "honcho", "memory"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.backend != "codexu" {
		t.Errorf("backend = %q", opts.backend)
	}
	want := []string{"honcho", "memory"}
	if !reflect.DeepEqual(opts.keywords, want) {
		t.Errorf("keywords = %v, want %v", opts.keywords, want)
	}
}

func TestParseRunOptions_BackendFlagRequiresValue(t *testing.T) {
	if _, err := parseRunOptions([]string{"--backend"}); err == nil {
		t.Fatal("parseRunOptions(--backend with no value) error = nil, want error")
	}
}

func TestParseRunOptions_BackendFlagRejectsUnsupported(t *testing.T) {
	if _, err := parseRunOptions([]string{"--backend", "ollama"}); err == nil {
		t.Fatal("parseRunOptions(--backend ollama) error = nil, want error")
	}
}

func TestParseRunOptions_QuotedMultiwordKeywordsSplitOnWhitespace(t *testing.T) {
	opts, err := parseRunOptions([]string{"skills tools"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"skills", "tools"}
	if !reflect.DeepEqual(opts.keywords, want) {
		t.Errorf("keywords = %v, want %v", opts.keywords, want)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run(context.Background(), []string{"unknown"})
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !errors.Is(err, errParse) {
		t.Fatalf("err = %v, want errors.Is(errParse) = true", err)
	}
	if !strings.Contains(err.Error(), "usage: planner-loop") {
		t.Fatalf("run() error = %q, want usage", err)
	}
}

func TestPrintRunSummaryEchoesKeywords(t *testing.T) {
	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() { commandStdout = oldStdout })

	if err := printRunSummary(plannerloop.RunSummary{Backend: "codexu", Mode: "safe"}, true, []string{"hermes-issues", "memory"}); err != nil {
		t.Fatalf("printRunSummary error = %v", err)
	}

	if !strings.Contains(stdout.String(), "keywords: hermes-issues memory") {
		t.Fatalf("stdout = %q, want keywords echo", stdout.String())
	}
}

func TestPrintRunSummaryOmitsKeywordsWhenEmpty(t *testing.T) {
	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() { commandStdout = oldStdout })

	if err := printRunSummary(plannerloop.RunSummary{Backend: "codexu", Mode: "safe"}, true, nil); err != nil {
		t.Fatalf("printRunSummary error = %v", err)
	}

	if strings.Contains(stdout.String(), "keywords:") {
		t.Fatalf("stdout = %q, expected no keywords line", stdout.String())
	}
}

func TestResolveRepoRootPrefersFlag(t *testing.T) {
	dir := t.TempDir()
	args, root, err := resolveRepoRoot([]string{"run", "--repo-root", dir})
	if err != nil {
		t.Fatalf("resolveRepoRoot error = %v", err)
	}
	if root != dir {
		t.Fatalf("root = %q, want %q", root, dir)
	}
	if got, want := args, []string{"run"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestSubcommandHelpPrintsScopedUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "run", args: []string{"run", "--help"}, want: "usage: planner-loop run"},
		{name: "doctor", args: []string{"doctor", "-h"}, want: "usage: planner-loop doctor"},
		{name: "trigger", args: []string{"trigger", "--help"}, want: "usage: planner-loop trigger <reason>"},
		{name: "service", args: []string{"service", "--help"}, want: "usage: planner-loop service install"},
		{name: "service install", args: []string{"service", "install", "--help"}, want: "usage: planner-loop service install"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			oldStdout := commandStdout
			commandStdout = &stdout
			t.Cleanup(func() { commandStdout = oldStdout })
			withWorkingDir(t, t.TempDir())

			if err := run(context.Background(), tc.args); err != nil {
				t.Fatalf("run() error = %v", err)
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout = %q, want substring %q", stdout.String(), tc.want)
			}
		})
	}
}

func TestTriggerAppendsManualEvent(t *testing.T) {
	repoRoot := writeCommandFixture(t)
	triggersPath := filepath.Join(repoRoot, "triggers.jsonl")
	t.Setenv("PLANNER_TRIGGERS_PATH", triggersPath)
	withWorkingDir(t, repoRoot)

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() { commandStdout = oldStdout })

	if err := run(context.Background(), []string{"trigger", "operator-asked-for-refresh"}); err != nil {
		t.Fatalf("run(trigger) error = %v", err)
	}

	body, err := os.ReadFile(triggersPath)
	if err != nil {
		t.Fatalf("read triggers.jsonl: %v", err)
	}
	if !strings.Contains(string(body), `"reason":"operator-asked-for-refresh"`) {
		t.Fatalf("triggers.jsonl missing reason:\n%s", body)
	}
	if !strings.Contains(string(body), `"kind":"manual"`) {
		t.Fatalf("triggers.jsonl missing kind=manual:\n%s", body)
	}
	if !strings.Contains(stdout.String(), "trigger: appended manual event") {
		t.Fatalf("stdout = %q, want trigger confirmation", stdout.String())
	}
}

func TestTriggerRequiresReason(t *testing.T) {
	repoRoot := writeCommandFixture(t)
	withWorkingDir(t, repoRoot)
	if err := run(context.Background(), []string{"trigger"}); !errors.Is(err, errParse) {
		t.Fatalf("err = %v, want errParse", err)
	}
}

func writeCommandFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	t.Setenv("PROGRESS_JSON", filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json"))
	writeCommandFile(t, filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json"), `{
  "phases": {
    "2": {
      "subphases": {
        "2.A": {"items": [{"name": "Gateway task", "status": "planned"}]}
      }
    }
  }
}`)
	for _, path := range []string{
		filepath.Join(root, "..", "hermes-agent", ".git", "HEAD"),
		filepath.Join(root, "..", "hermes-agent", "README.md"),
		filepath.Join(root, "..", "gbrain", ".git", "HEAD"),
		filepath.Join(root, "..", "gbrain", "README.md"),
		filepath.Join(root, "..", "honcho", ".git", "HEAD"),
		filepath.Join(root, "..", "honcho", "README.md"),
		filepath.Join(root, "docs", "content", "upstream-hermes", "_index.md"),
		filepath.Join(root, "docs", "content", "upstream-gbrain", "_index.md"),
		filepath.Join(root, "docs", "content", "building-gormes", "_index.md"),
	} {
		writeCommandFile(t, path, "# fixture\n")
	}
	return root
}

func writeCommandFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s) error = %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})
}
