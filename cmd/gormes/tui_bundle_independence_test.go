package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func TestTUIBundleIndependence_StartupPreflightIgnoresHermesInkBundle(t *testing.T) {
	setupNativeTUITestEnv(t)
	workDir := t.TempDir()
	mustWriteFile(t, filepath.Join(workDir, "ui-tui", "dist", "entry.js"), []byte("export {};"))
	mustWriteFile(t, filepath.Join(workDir, "node_modules", "ink", "package.json"), []byte(`{"name":"ink"}`))
	t.Chdir(workDir)

	commandLog := filepath.Join(workDir, "unexpected-package-command.log")
	installFailingPackageCommands(t, commandLog)

	var programRuns int
	cmd := newRootCommandWithRuntime(rootRuntime{
		tuiProgramFactory: func(tea.Model, ...tea.ProgramOption) tuiProgram {
			return fakeTUIProgram{run: func() { programRuns++ }}
		},
	})
	stdout, stderr, err := executeNativeTUICommand(cmd, "--offline")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if programRuns != 1 {
		t.Fatalf("program runs = %d, want 1", programRuns)
	}
	if data, err := os.ReadFile(commandLog); err == nil {
		t.Fatalf("startup invoked package command unexpectedly:\n%s", data)
	}
}

func TestDoctorTUIStatusReportsNativeGoBubbleTeaAvailability(t *testing.T) {
	got := doctorTUIStatus().Format()
	for _, want := range []string{
		"Native TUI",
		"Go-native Bubble Tea",
		"compiled into gormes",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor TUI status missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"HERMES_TUI_DIR",
		"package-lock",
		"node_modules",
		"ink-bundle",
		"npm install",
		"npm run",
		"packages/hermes-ink",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("doctor TUI status mentions Hermes/Node bundle preflight %q:\n%s", forbidden, got)
		}
	}
}

func TestNativeTUIInstallCopyPromisesNoRuntimeNodeOrNPM(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "www.gormes.ai", "internal", "site", "content.go"))
	if err != nil {
		t.Fatalf("read landing content: %v", err)
	}
	content := string(body)
	if !strings.Contains(content, "No runtime Node or npm") {
		t.Fatalf("landing install copy does not promise no runtime Node/npm dependency")
	}
	for _, forbidden := range []string{
		"npm install",
		"npm run build",
		"HERMES_TUI_DIR",
		"packages/hermes-ink",
		"ink-bundle",
		"node_modules/ink",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("landing content inherited Hermes TUI rebuild instruction %q", forbidden)
		}
	}
}

type fakeTUIProgram struct {
	run func()
}

func (p fakeTUIProgram) Run() (tea.Model, error) {
	if p.run != nil {
		p.run()
	}
	return nil, nil
}

func (p fakeTUIProgram) Quit() {}

func setupNativeTUITestEnv(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("GORMES_ENDPOINT", "")
	t.Setenv("GORMES_MODEL", "")
	t.Setenv("GORMES_API_KEY", "")
}

func executeNativeTUICommand(cmd *cobra.Command, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func installFailingPackageCommands(t *testing.T, logPath string) {
	t.Helper()
	binDir := t.TempDir()
	for _, name := range []string{"node", "npm", "pnpm", "yarn", "corepack"} {
		path := filepath.Join(binDir, name)
		body := "#!/bin/sh\nprintf '%s %s\\n' \"$0\" \"$*\" >> " + shellQuote(logPath) + "\nexit 88\n"
		if runtime.GOOS == "windows" {
			body = "@echo off\r\necho %0 %*>> " + logPath + "\r\nexit /b 88\r\n"
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
