package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsSyncAndListCommands(t *testing.T) {
	skillsRoot := seedSkillsHubCatalog(t)

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"skills", "sync"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstderr=%s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "official/research/arxiv") {
		t.Fatalf("stdout = %q, want synced ref", out)
	}

	cmd = newRootCommand()
	stdout.Reset()
	stderr.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"skills", "list", "--source=hub"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "official/research/arxiv") {
		t.Fatalf("stdout = %q, want hub ref", stdout.String())
	}

	if _, err := os.Stat(filepath.Join(skillsRoot, ".hub", "lock.json")); err != nil {
		t.Fatalf("hub lock missing after sync: %v", err)
	}
}

func TestSkillsInstallCommand_InstallsHubSkill(t *testing.T) {
	skillsRoot := seedSkillsHubCatalog(t)

	syncCmd := newRootCommand()
	syncCmd.SetArgs([]string{"skills", "sync"})
	if err := syncCmd.Execute(); err != nil {
		t.Fatalf("sync Execute: %v", err)
	}

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"skills", "install", "official/research/arxiv"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "installed official/research/arxiv") {
		t.Fatalf("stdout = %q, want install message", stdout.String())
	}

	activePath := filepath.Join(skillsRoot, "active", "research", "arxiv", "SKILL.md")
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active skill missing: %v", err)
	}

	listCmd := newRootCommand()
	stdout.Reset()
	stderr.Reset()
	listCmd.SetOut(&stdout)
	listCmd.SetErr(&stderr)
	listCmd.SetArgs([]string{"skills", "list"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list Execute: %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "arxiv") {
		t.Fatalf("stdout = %q, want installed skill name", stdout.String())
	}
}

func seedSkillsHubCatalog(t *testing.T) string {
	t.Helper()

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dataHome, "config"))

	skillsRoot := filepath.Join(dataHome, "gormes", "skills")
	if err := os.MkdirAll(filepath.Join(skillsRoot, ".hub", "catalogs", "official", "research", "arxiv"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	raw := "---\nname: arxiv\ndescription: Search and retrieve arXiv papers\n---\n\nUse arXiv APIs and summarize relevant papers."
	if err := os.WriteFile(filepath.Join(skillsRoot, ".hub", "catalogs", "official", "research", "arxiv", "SKILL.md"), []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return skillsRoot
}
