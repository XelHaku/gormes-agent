package autoloop

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRenderServiceUnitInjectsPaths(t *testing.T) {
	unit := RenderServiceUnit(ServiceUnitOptions{
		AutoloopPath: "/opt/gormes/bin/autoloop",
		WorkDir:      "/srv/gormes",
	})

	for _, want := range []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"WorkingDirectory=/srv/gormes",
		"ExecStart=/opt/gormes/bin/autoloop run",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("RenderServiceUnit() = %q, want %q", unit, want)
		}
	}
}

func TestInstallServiceWritesUnitAndReloadsSystemd(t *testing.T) {
	unitDir := t.TempDir()
	runner := &FakeRunner{
		Results: []Result{{}, {}},
	}

	err := InstallService(context.Background(), ServiceInstallOptions{
		Runner:       runner,
		UnitDir:      unitDir,
		UnitName:     "gormes-autoloop.service",
		AutoloopPath: "/opt/gormes/bin/autoloop",
		WorkDir:      "/srv/gormes",
		AutoStart:    true,
	})
	if err != nil {
		t.Fatalf("InstallService() error = %v", err)
	}

	unitPath := filepath.Join(unitDir, "gormes-autoloop.service")
	raw, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	info, err := os.Stat(unitPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("unit mode = %v, want %v", got, os.FileMode(0o644))
	}
	unit := string(raw)
	for _, want := range []string{
		"WorkingDirectory=/srv/gormes",
		"ExecStart=/opt/gormes/bin/autoloop run",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("installed unit = %q, want %q", unit, want)
		}
	}

	wantCommands := []Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
		{Name: "systemctl", Args: []string{"--user", "enable", "--now", "gormes-autoloop.service"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}

	if err := InstallService(context.Background(), ServiceInstallOptions{
		Runner:       &FakeRunner{},
		UnitDir:      unitDir,
		UnitName:     "gormes-autoloop.service",
		AutoloopPath: "/new/autoloop",
		WorkDir:      "/new/workdir",
	}); err == nil {
		t.Fatal("InstallService() error = nil, want existing unit error")
	}
}

func TestDisableLegacyTimersRunsExpectedSystemctlCalls(t *testing.T) {
	runner := &FakeRunner{
		Results: []Result{{}, {}},
	}

	if err := DisableLegacyTimers(context.Background(), runner); err != nil {
		t.Fatalf("DisableLegacyTimers() error = %v", err)
	}

	wantCommands := []Command{
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner-tasks-manager.timer"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architectureplanneragent.timer"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}
