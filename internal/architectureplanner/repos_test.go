package architectureplanner

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

func TestConfigFromEnvDefaultsExternalRepoURLs(t *testing.T) {
	cfg, err := ConfigFromEnv(filepath.Join("tmp", "repo"), map[string]string{})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	got := cfg.ExternalRepos()
	want := []ExternalRepo{
		{Name: "hermes-agent", Path: filepath.Join("tmp", "hermes-agent"), CloneURL: "https://github.com/NousResearch/hermes-agent.git"},
		{Name: "gbrain", Path: filepath.Join("tmp", "gbrain"), CloneURL: "https://github.com/garrytan/gbrain.git"},
		{Name: "honcho", Path: filepath.Join("tmp", "honcho"), CloneURL: "https://github.com/plastic-labs/honcho"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExternalRepos() = %#v, want %#v", got, want)
	}
}

func TestConfigFromEnvReadsExternalRepoURLOverrides(t *testing.T) {
	cfg, err := ConfigFromEnv(filepath.Join("tmp", "repo"), map[string]string{
		"HERMES_REPO_URL": "https://example.test/hermes.git",
		"GBRAIN_REPO_URL": "https://example.test/gbrain.git",
		"HONCHO_REPO_URL": "https://example.test/honcho.git",
	})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	got := cfg.ExternalRepos()
	if got[0].CloneURL != "https://example.test/hermes.git" {
		t.Fatalf("Hermes CloneURL = %q", got[0].CloneURL)
	}
	if got[1].CloneURL != "https://example.test/gbrain.git" {
		t.Fatalf("GBrain CloneURL = %q", got[1].CloneURL)
	}
	if got[2].CloneURL != "https://example.test/honcho.git" {
		t.Fatalf("Honcho CloneURL = %q", got[2].CloneURL)
	}
}

func TestSyncExternalReposPullsExistingGitReposAndClonesMissingRepos(t *testing.T) {
	root := t.TempDir()
	cfg, err := ConfigFromEnv(filepath.Join(root, "gormes-agent"), map[string]string{
		"HERMES_DIR":      filepath.Join(root, "hermes-agent"),
		"GBRAIN_DIR":      filepath.Join(root, "gbrain"),
		"HONCHO_DIR":      filepath.Join(root, "honcho"),
		"HERMES_REPO_URL": "https://example.test/hermes.git",
		"GBRAIN_REPO_URL": "https://example.test/gbrain.git",
		"HONCHO_REPO_URL": "https://example.test/honcho.git",
	})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	for _, dir := range []string{cfg.HermesDir, cfg.HonchoDir} {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s/.git) error = %v", dir, err)
		}
	}
	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}, {}, {}}}

	if err := SyncExternalRepos(context.Background(), cfg, runner); err != nil {
		t.Fatalf("SyncExternalRepos() error = %v", err)
	}

	want := []autoloop.Command{
		{Name: "git", Args: []string{"-C", cfg.HermesDir, "pull", "--ff-only"}, Dir: cfg.RepoRoot},
		{Name: "git", Args: []string{"clone", "https://example.test/gbrain.git", cfg.GBrainDir}, Dir: cfg.RepoRoot},
		{Name: "git", Args: []string{"-C", cfg.HonchoDir, "pull", "--ff-only"}, Dir: cfg.RepoRoot},
	}
	if !reflect.DeepEqual(runner.Commands, want) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, want)
	}
}

func TestSyncExternalReposRejectsNonGitExistingDirectory(t *testing.T) {
	root := t.TempDir()
	cfg, err := ConfigFromEnv(filepath.Join(root, "gormes-agent"), map[string]string{
		"HERMES_DIR": filepath.Join(root, "hermes-agent"),
	})
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if err := os.MkdirAll(cfg.HermesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	err = SyncExternalRepos(context.Background(), cfg, &autoloop.FakeRunner{})
	if err == nil {
		t.Fatal("SyncExternalRepos() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "exists but is not a git repository") {
		t.Fatalf("SyncExternalRepos() error = %q", err)
	}
}
