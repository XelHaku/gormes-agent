package autoloop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ServiceUnitOptions struct {
	AutoloopPath string
	WorkDir      string
	ExecArgs     []string
}

func RenderServiceUnit(opts ServiceUnitOptions) string {
	execArgs := opts.ExecArgs
	if execArgs == nil {
		execArgs = []string{"run"}
	}
	execStart := systemdPathValue(opts.AutoloopPath)
	for _, arg := range execArgs {
		execStart += " " + systemdPathValue(arg)
	}

	return fmt.Sprintf(`[Unit]
Description=Gormes autoloop

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure

[Install]
WantedBy=default.target
`, systemdPathValue(opts.WorkDir), execStart)
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
	timerPath := filepath.Join(opts.UnitDir, opts.TimerName)
	for _, path := range []string{servicePath, timerPath} {
		if opts.Force {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("systemd unit %s already exists", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	service := RenderAuditServiceUnit(AuditServiceUnitOptions{
		AuditPath: opts.AuditPath,
		WorkDir:   opts.WorkDir,
	})
	if err := os.WriteFile(servicePath, []byte(service), 0o644); err != nil {
		return err
	}
	timer := RenderAuditTimerUnit(AuditTimerUnitOptions{
		ServiceUnitName: opts.UnitName,
	})
	if err := os.WriteFile(timerPath, []byte(timer), 0o644); err != nil {
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

type AuditServiceUnitOptions struct {
	AuditPath string
	WorkDir   string
}

func RenderAuditServiceUnit(opts AuditServiceUnitOptions) string {
	return fmt.Sprintf(`[Unit]
Description=Gormes orchestrator audit

[Service]
Type=oneshot
WorkingDirectory=%s
ExecStart=%s
`, systemdPathValue(opts.WorkDir), systemdPathValue(opts.AuditPath))
}

type AuditTimerUnitOptions struct {
	ServiceUnitName string
}

func RenderAuditTimerUnit(opts AuditTimerUnitOptions) string {
	return fmt.Sprintf(`[Unit]
Description=Run Gormes orchestrator audit periodically

[Timer]
OnBootSec=5min
OnUnitActiveSec=20min
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
