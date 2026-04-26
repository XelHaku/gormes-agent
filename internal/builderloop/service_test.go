package builderloop

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
		"After=network-online.target",
		"Wants=network-online.target",
		"Environment=PATH=%h/.local/bin:/usr/local/bin:/usr/bin:/bin",
		"Environment=DISABLE_COMPANIONS=0",
		"Environment=COMPANION_ON_IDLE=1",
		"Environment=MAX_AGENTS=4",
		"Environment=MODE=safe",
		"WorkingDirectory=/srv/gormes",
		"ExecStart=/opt/gormes/bin/autoloop run --loop",
		"Restart=always",
		"RestartPreventExitStatus=2 30",
		"RestartSec=30s",
		"TimeoutStopSec=60s",
		"KillMode=mixed",
		"KillSignal=SIGTERM",
		"WantedBy=default.target",
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
		`ExecStart="/tmp/gormes repo/bin/auto%%loop\"\\bin" run --loop`,
	} {
		if !strings.Contains(quoted, want) {
			t.Fatalf("RenderServiceUnit() = %q, want %q", quoted, want)
		}
	}

	controlEscaped := RenderServiceUnit(ServiceUnitOptions{
		AutoloopPath: "/tmp/gormes\nrepo/bin/autoloop",
		WorkDir:      "/tmp/gormes\trepo",
	})
	for _, want := range []string{
		`WorkingDirectory="/tmp/gormes\trepo"`,
		`ExecStart="/tmp/gormes\nrepo/bin/autoloop" run --loop`,
	} {
		if !strings.Contains(controlEscaped, want) {
			t.Fatalf("RenderServiceUnit() = %q, want %q", controlEscaped, want)
		}
	}
}

func TestRenderServiceUnitAllowsCustomExecArgs(t *testing.T) {
	unit := RenderServiceUnit(ServiceUnitOptions{
		AutoloopPath: "/srv/gormes/scripts/gormes-auto-codexu-orchestrator.sh",
		WorkDir:      "/srv/gormes",
		ExecArgs:     []string{},
	})

	if strings.Contains(unit, " run") {
		t.Fatalf("RenderServiceUnit() = %q, want no implicit run arg with explicit empty ExecArgs", unit)
	}
	if !strings.Contains(unit, "ExecStart=/srv/gormes/scripts/gormes-auto-codexu-orchestrator.sh") {
		t.Fatalf("RenderServiceUnit() = %q, want wrapper ExecStart", unit)
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
		"ExecStart=/opt/gormes/bin/autoloop run --loop",
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

func TestInstallAuditServiceWritesServiceAndTimerAndEnablesTimer(t *testing.T) {
	unitDir := t.TempDir()
	runner := &FakeRunner{
		Results: []Result{{}, {}},
	}

	if err := InstallAuditService(context.Background(), AuditServiceInstallOptions{
		Runner:    runner,
		UnitDir:   unitDir,
		UnitName:  "gormes-orchestrator-audit.service",
		TimerName: "gormes-orchestrator-audit.timer",
		AuditPath: "/srv/gormes/scripts/orchestrator/audit.sh",
		WorkDir:   "/srv/gormes",
		AutoStart: true,
	}); err != nil {
		t.Fatalf("InstallAuditService() error = %v", err)
	}

	service, err := os.ReadFile(filepath.Join(unitDir, "gormes-orchestrator-audit.service"))
	if err != nil {
		t.Fatalf("ReadFile(service) error = %v", err)
	}
	for _, want := range []string{
		"Type=oneshot",
		"Environment=PATH=%h/.local/bin:/usr/local/bin:/usr/bin:/bin",
		"Environment=REPO_ROOT=/srv/gormes",
		"WorkingDirectory=/srv/gormes",
		"ExecStart=/srv/gormes/scripts/orchestrator/audit.sh",
		"TimeoutStartSec=60s",
		"Nice=10",
	} {
		if !strings.Contains(string(service), want) {
			t.Fatalf("service unit = %q, want %q", service, want)
		}
	}
	if strings.Contains(string(service), " run") {
		t.Fatalf("service unit = %q, want audit wrapper without run arg", service)
	}

	timer, err := os.ReadFile(filepath.Join(unitDir, "gormes-orchestrator-audit.timer"))
	if err != nil {
		t.Fatalf("ReadFile(timer) error = %v", err)
	}
	for _, want := range []string{
		"[Timer]",
		"OnBootSec=2min",
		"OnUnitActiveSec=20min",
		"AccuracySec=30s",
		"Persistent=true",
		"Unit=gormes-orchestrator-audit.service",
		"WantedBy=timers.target",
	} {
		if !strings.Contains(string(timer), want) {
			t.Fatalf("timer unit = %q, want %q", timer, want)
		}
	}

	wantCommands := []Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
		{Name: "systemctl", Args: []string{"--user", "enable", "--now", "gormes-orchestrator-audit.timer"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestInstallAuditServicePreservesExistingServiceAndWritesMissingTimer(t *testing.T) {
	unitDir := t.TempDir()
	servicePath := filepath.Join(unitDir, "gormes-orchestrator-audit.service")
	if err := os.WriteFile(servicePath, []byte("existing service\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(service) error = %v", err)
	}
	runner := &FakeRunner{
		Results: []Result{{}, {}},
	}

	if err := InstallAuditService(context.Background(), AuditServiceInstallOptions{
		Runner:    runner,
		UnitDir:   unitDir,
		UnitName:  "gormes-orchestrator-audit.service",
		TimerName: "gormes-orchestrator-audit.timer",
		AuditPath: "/srv/gormes/scripts/orchestrator/audit.sh",
		WorkDir:   "/srv/gormes",
		AutoStart: true,
	}); err != nil {
		t.Fatalf("InstallAuditService() error = %v", err)
	}

	service, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("ReadFile(service) error = %v", err)
	}
	if got, want := string(service), "existing service\n"; got != want {
		t.Fatalf("service = %q, want preserved %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "gormes-orchestrator-audit.timer")); err != nil {
		t.Fatalf("Stat(timer) error = %v", err)
	}

	wantCommands := []Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
		{Name: "systemctl", Args: []string{"--user", "enable", "--now", "gormes-orchestrator-audit.timer"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestInstallAuditServiceWithoutAutoStartDoesNotEnableTimer(t *testing.T) {
	runner := &FakeRunner{Results: []Result{{}}}

	if err := InstallAuditService(context.Background(), AuditServiceInstallOptions{
		Runner:    runner,
		UnitDir:   t.TempDir(),
		UnitName:  "gormes-orchestrator-audit.service",
		TimerName: "gormes-orchestrator-audit.timer",
		AuditPath: "/srv/gormes/scripts/orchestrator/audit.sh",
		WorkDir:   "/srv/gormes",
	}); err != nil {
		t.Fatalf("InstallAuditService() error = %v", err)
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
		Results: []Result{{}, {}, {}, {}, {}, {}},
	}

	if err := DisableLegacyTimers(context.Background(), runner); err != nil {
		t.Fatalf("DisableLegacyTimers() error = %v", err)
	}

	wantCommands := []Command{
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner-tasks-manager.timer"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architectureplanneragent.timer"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner.timer"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner.path"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner-impl.path"}},
		{Name: "sh", Args: []string{"-c", legacyCronCleanupScript}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestDisableLegacyTimersInvokesCronCleanup(t *testing.T) {
	runner := &FakeRunner{
		Results: []Result{{}, {}, {}, {}, {}, {}},
	}

	if err := DisableLegacyTimers(context.Background(), runner); err != nil {
		t.Fatalf("DisableLegacyTimers() error = %v", err)
	}

	if got := runner.Commands[len(runner.Commands)-1]; got.Name != "sh" || !strings.Contains(got.Args[1], "gormes-architecture-planner-tasks-manager\\.sh") || !strings.Contains(got.Args[1], "documentation-improver\\.sh") || !strings.Contains(got.Args[1], "landingpage-improver\\.sh") {
		t.Fatalf("cron cleanup command = %#v, want cleanup for all legacy scripts", got)
	}
}

func TestDisableLegacyTimersIgnoresMissingTimerAndContinues(t *testing.T) {
	runner := &FakeRunner{
		Results: []Result{
			{Stderr: "Failed to disable unit: Unit file gormes-architecture-planner-tasks-manager.timer does not exist.", Err: errors.New("missing")},
			{}, {}, {}, {}, {},
		},
	}

	if err := DisableLegacyTimers(context.Background(), runner); err != nil {
		t.Fatalf("DisableLegacyTimers() error = %v", err)
	}

	if got, want := len(runner.Commands), 6; got != want {
		t.Fatalf("commands length = %d, want %d", got, want)
	}
}

func TestDisableLegacyTimersIgnoresHyphenatedMissingStates(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
	}{
		{
			name:   "not-loaded",
			stderr: "Warning: The unit file, source configuration file or drop-ins of gormes-architecture-planner-tasks-manager.timer changed on disk. Run 'systemctl --user daemon-reload' to reload units.\nFailed to disable unit: Unit gormes-architecture-planner-tasks-manager.timer is not-loaded.",
		},
		{
			name:   "not-found",
			stderr: "Failed to disable unit: Unit gormes-architecture-planner-tasks-manager.timer not-found.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &FakeRunner{
				Results: []Result{
					{Stderr: test.stderr, Err: errors.New(test.name)},
					{}, {}, {}, {}, {},
				},
			}

			if err := DisableLegacyTimers(context.Background(), runner); err != nil {
				t.Fatalf("DisableLegacyTimers() error = %v", err)
			}
			if got, want := len(runner.Commands), 6; got != want {
				t.Fatalf("commands length = %d, want %d", got, want)
			}
		})
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
