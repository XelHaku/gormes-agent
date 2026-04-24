package architectureplanner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

func TestRunDryRunCollectsContextWithoutBackend(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	runner := &autoloop.FakeRunner{}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config: mustConfig(t, repoRoot),
		Runner: runner,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if summary.Backend != "codexu" {
		t.Fatalf("Backend = %q, want codexu", summary.Backend)
	}
	if len(summary.SourceRoots) != 8 {
		t.Fatalf("SourceRoots length = %d, want 8: %#v", len(summary.SourceRoots), summary.SourceRoots)
	}
	if summary.ProgressItems != 2 {
		t.Fatalf("ProgressItems = %d, want 2", summary.ProgressItems)
	}
	if len(runner.Commands) != 0 {
		t.Fatalf("Commands length = %d, want 0", len(runner.Commands))
	}
	if _, err := os.Stat(filepath.Join(summary.RunRoot, "context.json")); err != nil {
		t.Fatalf("context.json missing after dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(summary.RunRoot, "latest_prompt.txt")); err != nil {
		t.Fatalf("latest_prompt.txt missing after dry-run: %v", err)
	}
}

func TestRunOnceSendsPlannerPromptToBackendAndWritesArtifacts(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	runner := &autoloop.FakeRunner{
		Results: []autoloop.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Updating abc123..def456\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: `{"type":"thread.started","thread_id":"thread-arch-1"}` + "\n"},
		},
	}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config:         mustConfig(t, repoRoot),
		Runner:         runner,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if got, want := len(runner.Commands), 4; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	command := runner.Commands[3]
	if command.Name != "codexu" {
		t.Fatalf("Command.Name = %q, want codexu", command.Name)
	}
	prompt := command.Args[len(command.Args)-1]
	for _, want := range []string{
		"Gormes Architecture Planner Loop",
		"hermes-agent",
		"gbrain",
		"upstream-hermes",
		"upstream-gbrain",
		"building-gormes",
		"www.gormes.ai",
		"Hugo docs",
		"landing page",
		"docs/hugo.toml",
		"goncho",
		"progress.json",
		"only long-term prompt agent",
		"Sync results:",
		"gbrain: pull",
		"Updating abc123..def456",
		"Synchronize progress.json with the current Gormes implementation",
		"Synchronize landing page, Hugo docs, generated pages, and progress.json",
		"Do not implement runtime feature code",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if _, err := os.Stat(filepath.Join(summary.RunRoot, "latest_planner_report.md")); err != nil {
		t.Fatalf("latest_planner_report.md missing: %v", err)
	}

	stateData, err := os.ReadFile(filepath.Join(summary.RunRoot, "planner_state.json"))
	if err != nil {
		t.Fatalf("ReadFile(planner_state.json) error = %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("planner_state.json parse error = %v", err)
	}
	if state["backend"] != "codexu" {
		t.Fatalf("state backend = %#v, want codexu", state["backend"])
	}
	contextData, err := os.ReadFile(filepath.Join(summary.RunRoot, "context.json"))
	if err != nil {
		t.Fatalf("ReadFile(context.json) error = %v", err)
	}
	for _, want := range []string{
		`"sync_results"`,
		`"output": "Updating abc123..def456"`,
		`"implementation_inventory"`,
		`"landing_site"`,
		`"hugo_docs"`,
	} {
		if !strings.Contains(string(contextData), want) {
			t.Fatalf("context.json missing %q:\n%s", want, contextData)
		}
	}
}

func TestRunOnceReturnsBackendErrorWithOutput(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	wantErr := os.ErrPermission
	runner := &autoloop.FakeRunner{
		Results: []autoloop.Result{{}, {}, {}, {Err: wantErr, Stderr: "backend denied\n"}},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         mustConfig(t, repoRoot),
		Runner:         runner,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "backend denied") {
		t.Fatalf("RunOnce() error = %q, want backend stderr", err)
	}
}

func TestRunOnceRunsValidationAfterBackend(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	runner := &autoloop.FakeRunner{
		Results: []autoloop.Result{{}, {}, {}, {}, {}, {}, {}, {}, {}},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: mustConfig(t, repoRoot),
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if got, want := len(runner.Commands), 9; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	wantArgs := [][]string{
		{"run", "./cmd/progress-gen", "-write"},
		{"run", "./cmd/progress-gen", "-validate"},
		{"test", "./internal/progress", "-count=1"},
		{"test", "./docs", "-count=1"},
		{"test", "./...", "-count=1"},
	}
	for i, want := range wantArgs {
		command := runner.Commands[i+4]
		if command.Name != "go" {
			t.Fatalf("validation command %d name = %q, want go", i, command.Name)
		}
		if strings.Join(command.Args, " ") != strings.Join(want, " ") {
			t.Fatalf("validation command %d args = %#v, want %#v", i, command.Args, want)
		}
	}
	if got, want := runner.Commands[8].Dir, filepath.Join(repoRoot, "www.gormes.ai"); got != want {
		t.Fatalf("landing validation dir = %q, want %q", got, want)
	}
}

func mustConfig(t *testing.T, repoRoot string) Config {
	t.Helper()

	cfg, err := ConfigFromEnv(repoRoot, map[string]string{
		"RUN_ROOT": filepath.Join(repoRoot, ".codex", "architecture-planner-test"),
	})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	return cfg
}

func writePlannerFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json"), `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned"},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
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
		filepath.Join(root, "docs", "hugo.toml"),
		filepath.Join(root, "docs", "layouts", "index.html"),
		filepath.Join(root, "docs", "static", "site.css"),
		filepath.Join(root, "www.gormes.ai", "README.md"),
		filepath.Join(root, "www.gormes.ai", "internal", "site", "server.go"),
		filepath.Join(root, "www.gormes.ai", "tests", "home.spec.mjs"),
	} {
		writeFile(t, path, "# fixture\n")
	}
	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
