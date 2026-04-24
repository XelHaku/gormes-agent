package autoloop

import (
	"context"
	"errors"
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

	quoted := RenderServiceUnit(ServiceUnitOptions{
		AutoloopPath: `/tmp/gormes repo/bin/auto%loop"\bin`,
		WorkDir:      `/tmp/gormes repo/work%dir"\subdir`,
	})
	for _, want := range []string{
		`WorkingDirectory="/tmp/gormes repo/work%%dir\"\\subdir"`,
		`ExecStart="/tmp/gormes repo/bin/auto%%loop\"\\bin" run`,
	} {
		if !strings.Contains(quoted, want) {
			t.Fatalf("RenderServiceUnit() = %q, want %q", quoted, want)
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

func TestInstallServiceWithoutAutoStartDoesNotEnable(t *testing.T) {
	unitDir := t.TempDir()
	runner := &FakeRunner{
		Results: []Result{{}},
	}

	if err := InstallService(context.Background(), ServiceInstallOptions{
		Runner:       runner,
		UnitDir:      unitDir,
		UnitName:     "gormes-autoloop.service",
		AutoloopPath: "/opt/gormes/bin/autoloop",
		WorkDir:      "/srv/gormes",
		AutoStart:    false,
	}); err != nil {
		t.Fatalf("InstallService() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(unitDir, "gormes-autoloop.service")); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	wantCommands := []Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestInstallServiceForceOverwritesExistingUnit(t *testing.T) {
	unitDir := t.TempDir()
	unitPath := filepath.Join(unitDir, "gormes-autoloop.service")
	if err := os.WriteFile(unitPath, []byte("old unit"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runner := &FakeRunner{
		Results: []Result{{}},
	}

	if err := InstallService(context.Background(), ServiceInstallOptions{
		Runner:       runner,
		UnitDir:      unitDir,
		UnitName:     "gormes-autoloop.service",
		AutoloopPath: "/opt/gormes/bin/autoloop",
		WorkDir:      "/srv/gormes",
		Force:        true,
	}); err != nil {
		t.Fatalf("InstallService() error = %v", err)
	}

	raw, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(raw), "old unit") {
		t.Fatalf("installed unit = %q, want old contents overwritten", raw)
	}
}

func TestInstallServiceReturnsDaemonReloadFailure(t *testing.T) {
	wantErr := errors.New("daemon reload failed")
	runner := &FakeRunner{
		Results: []Result{{Err: wantErr}},
	}

	err := InstallService(context.Background(), ServiceInstallOptions{
		Runner:       runner,
		UnitDir:      t.TempDir(),
		UnitName:     "gormes-autoloop.service",
		AutoloopPath: "/opt/gormes/bin/autoloop",
		WorkDir:      "/srv/gormes",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("InstallService() error = %v, want %v", err, wantErr)
	}
}

func TestInstallServiceReturnsEnableFailure(t *testing.T) {
	wantErr := errors.New("enable failed")
	runner := &FakeRunner{
		Results: []Result{{}, {Err: wantErr}},
	}

	err := InstallService(context.Background(), ServiceInstallOptions{
		Runner:       runner,
		UnitDir:      t.TempDir(),
		UnitName:     "gormes-autoloop.service",
		AutoloopPath: "/opt/gormes/bin/autoloop",
		WorkDir:      "/srv/gormes",
		AutoStart:    true,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("InstallService() error = %v, want %v", err, wantErr)
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

func TestDisableLegacyTimersIgnoresMissingTimerAndContinues(t *testing.T) {
	runner := &FakeRunner{
		Results: []Result{
			{Stderr: "Failed to disable unit: Unit file gormes-architecture-planner-tasks-manager.timer does not exist.", Err: errors.New("missing")},
			{},
		},
	}

	if err := DisableLegacyTimers(context.Background(), runner); err != nil {
		t.Fatalf("DisableLegacyTimers() error = %v", err)
	}

	if got, want := len(runner.Commands), 2; got != want {
		t.Fatalf("commands length = %d, want %d", got, want)
	}
}

func TestDisableLegacyTimersReturnsNonMissingFailure(t *testing.T) {
	wantErr := errors.New("permission denied")
	runner := &FakeRunner{
		Results: []Result{{Stderr: "Failed to disable unit: Access denied", Err: wantErr}},
	}

	err := DisableLegacyTimers(context.Background(), runner)
	if !errors.Is(err, wantErr) {
		t.Fatalf("DisableLegacyTimers() error = %v, want %v", err, wantErr)
	}
	if got, want := len(runner.Commands), 1; got != want {
		t.Fatalf("commands length = %d, want %d", got, want)
	}
}
