package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
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

	if err := run([]string{"run", "--dry-run"}); err != nil {
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
	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}, {}, {}, {}}}
	oldRunner := commandRunner
	commandRunner = runner
	t.Cleanup(func() {
		commandRunner = oldRunner
	})

	withWorkingDir(t, repoRoot)

	if err := run([]string{"run", "--claudeu"}); err != nil {
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

	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}, {}, {}, {}}}
	oldRunner := commandRunner
	commandRunner = runner
	t.Cleanup(func() {
		commandRunner = oldRunner
	})
	if err := run([]string{"run"}); err != nil {
		t.Fatalf("run() setup error = %v", err)
	}

	var stdout bytes.Buffer
	oldStdout := commandStdout
	commandStdout = &stdout
	t.Cleanup(func() {
		commandStdout = oldStdout
	})

	if err := run([]string{"status"}); err != nil {
		t.Fatalf("status error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Last run UTC:") {
		t.Fatalf("status output missing last run:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := run([]string{"show-report"}); err != nil {
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

	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}}}
	oldRunner := commandRunner
	commandRunner = runner
	t.Cleanup(func() { commandRunner = oldRunner })

	if err := run([]string{"service", "install"}); err != nil {
		t.Fatalf("service install error = %v", err)
	}

	servicePath := filepath.Join(xdg, "systemd", "user", "gormes-architecture-planner.service")
	timerPath := filepath.Join(xdg, "systemd", "user", "gormes-architecture-planner.timer")
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
	opts, err := parseRunOptions([]string{"--codexu", "honcho", "memory"})
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
	err := run([]string{"unknown"})
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "usage: architecture-planner-loop") {
		t.Fatalf("run() error = %q, want usage", err)
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
