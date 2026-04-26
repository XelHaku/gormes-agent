package builderloop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const systemdPathEnvironment = "Environment=PATH=%h/.local/bin:/usr/local/bin:/usr/bin:/bin"

type ServiceUnitOptions struct {
	AutoloopPath string
	WorkDir      string
	ExecArgs     []string
}

func RenderServiceUnit(opts ServiceUnitOptions) string {
	execArgs := opts.ExecArgs
	if execArgs == nil {
		execArgs = []string{"run", "--loop"}
	}
	execStart := systemdPathValue(opts.AutoloopPath)
	for _, arg := range execArgs {
		execStart += " " + systemdPathValue(arg)
	}

	return fmt.Sprintf(`[Unit]
Description=Gormes autoloop
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
%s
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartPreventExitStatus=2 30
RestartSec=30s
TimeoutStopSec=60s
KillMode=mixed
KillSignal=SIGTERM
Environment=DISABLE_COMPANIONS=0
Environment=COMPANION_ON_IDLE=1
Environment=MAX_AGENTS=4
Environment=MODE=safe

[Install]
WantedBy=default.target
`, systemdPathEnvironment, systemdPathValue(opts.WorkDir), execStart)
}

type ServiceInstallOptions struct {
	Runner       Runner
	UnitDir      string
	UnitName     string
	AutoloopPath string
	WorkDir      string
	ExecArgs     []string
	AutoStart    bool
	Force        bool
}

func InstallService(ctx context.Context, opts ServiceInstallOptions) error {
	if opts.UnitDir == "" {
		return errors.New("unit dir is required")
	}
	if opts.UnitName == "" {
		return errors.New("unit name is required")
	}

	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	if err := os.MkdirAll(opts.UnitDir, 0o755); err != nil {
		return err
	}

	unitPath := filepath.Join(opts.UnitDir, opts.UnitName)
	if !opts.Force {
		if _, err := os.Stat(unitPath); err == nil {
			return fmt.Errorf("service unit %s already exists", unitPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	unit := RenderServiceUnit(ServiceUnitOptions{
		AutoloopPath: opts.AutoloopPath,
		WorkDir:      opts.WorkDir,
		ExecArgs:     opts.ExecArgs,
	})
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return err
	}

	if result := runner.Run(ctx, Command{Name: "systemctl", Args: []string{"--user", "daemon-reload"}}); result.Err != nil {
		return result.Err
	}

	if opts.AutoStart {
		result := runner.Run(ctx, Command{
			Name: "systemctl",
			Args: []string{"--user", "enable", "--now", opts.UnitName},
		})
		if result.Err != nil {
			return result.Err
		}
	}

	return nil
}

type AuditServiceInstallOptions struct {
	Runner    Runner
	UnitDir   string
	UnitName  string
	TimerName string
	AuditPath string
	WorkDir   string
	AutoStart bool
	Force     bool
}

func InstallAuditService(ctx context.Context, opts AuditServiceInstallOptions) error {
	if opts.UnitDir == "" {
		return errors.New("unit dir is required")
	}
	if opts.UnitName == "" {
		return errors.New("unit name is required")
	}
	if opts.TimerName == "" {
		return errors.New("timer name is required")
	}

	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	if err := os.MkdirAll(opts.UnitDir, 0o755); err != nil {
		return err
	}

	servicePath := filepath.Join(opts.UnitDir, opts.UnitName)
	if err := writeAuditUnitFile(servicePath, opts.Force, RenderAuditServiceUnit(AuditServiceUnitOptions{
		AuditPath: opts.AuditPath,
		WorkDir:   opts.WorkDir,
	})); err != nil {
		return err
	}

	timerPath := filepath.Join(opts.UnitDir, opts.TimerName)
	if err := writeAuditUnitFile(timerPath, opts.Force, RenderAuditTimerUnit(AuditTimerUnitOptions{
		ServiceUnitName: opts.UnitName,
	})); err != nil {
		return err
	}

	if result := runner.Run(ctx, Command{Name: "systemctl", Args: []string{"--user", "daemon-reload"}}); result.Err != nil {
		return result.Err
	}
	if opts.AutoStart {
		result := runner.Run(ctx, Command{
			Name: "systemctl",
			Args: []string{"--user", "enable", "--now", opts.TimerName},
		})
		if result.Err != nil {
			return result.Err
		}
	}

	return nil
}

func writeAuditUnitFile(path string, force bool, contents string) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.WriteFile(path, []byte(contents), 0o644)
}

type AuditServiceUnitOptions struct {
	AuditPath string
	WorkDir   string
}

func RenderAuditServiceUnit(opts AuditServiceUnitOptions) string {
	return fmt.Sprintf(`[Unit]
Description=Gormes orchestrator audit

[Service]
Type=oneshot
%s
Environment=REPO_ROOT=%s
WorkingDirectory=%s
ExecStart=%s
TimeoutStartSec=60s
Nice=10
`, systemdPathEnvironment, systemdPathValue(opts.WorkDir), systemdPathValue(opts.WorkDir), systemdPathValue(opts.AuditPath))
}

type AuditTimerUnitOptions struct {
	ServiceUnitName string
}

func RenderAuditTimerUnit(opts AuditTimerUnitOptions) string {
	return fmt.Sprintf(`[Unit]
Description=Run Gormes orchestrator audit periodically

[Timer]
OnBootSec=2min
OnUnitActiveSec=20min
AccuracySec=30s
Persistent=true
Unit=%s

[Install]
WantedBy=timers.target
`, opts.ServiceUnitName)
}

func systemdPathValue(path string) string {
	escaped := strings.ReplaceAll(path, "%", "%%")
	if !strings.ContainsAny(escaped, " \t\r\n\"\\") {
		return escaped
	}

	escaped = strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	).Replace(escaped)
	return `"` + escaped + `"`
}

func isMissingSystemdUnit(result Result) bool {
	output := strings.ToLower(result.Stdout + "\n" + result.Stderr)
	for _, marker := range []string{
		"missing",
		"not loaded",
		"not-loaded",
		"not found",
		"not-found",
		"does not exist",
		"no such file",
	} {
		if strings.Contains(output, marker) {
			return true
		}
	}

	return false
}

func DisableLegacyTimers(ctx context.Context, runner Runner) error {
	if runner == nil {
		runner = ExecRunner{}
	}

	for _, timer := range []string{
		"gormes-architecture-planner-tasks-manager.timer",
		"gormes-architectureplanneragent.timer",
		"gormes-architecture-planner.timer",
		"gormes-architecture-planner.path",
		"gormes-architecture-planner-impl.path",
	} {
		result := runner.Run(ctx, Command{
			Name: "systemctl",
			Args: []string{"--user", "disable", "--now", timer},
		})
		if result.Err != nil && !isMissingSystemdUnit(result) {
			return result.Err
		}
	}

	result := runner.Run(ctx, Command{
		Name: "sh",
		Args: []string{"-c", legacyCronCleanupScript},
	})
	return result.Err
}

const legacyCronCleanupScript = `set -eu
if command -v crontab >/dev/null 2>&1 && crontab -l >/dev/null 2>&1; then
  current=$(crontab -l 2>/dev/null || true)
  filtered=$(printf '%s\n' "$current" | grep -Ev 'gormes-architecture-planner-tasks-manager\.sh|documentation-improver\.sh|landingpage-improver\.sh' || true)
  if [ "$current" != "$filtered" ]; then
    printf '%s\n' "$filtered" | crontab -
  fi
fi`
