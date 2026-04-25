package architectureplanner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

const (
	// PlannerInterval is the default cadence between planner timer firings.
	PlannerInterval = "6h"
	// plannerEnvironmentPath mirrors autoloop's PATH so the timer-launched
	// service can find git, go, codexu, etc.
	plannerEnvironmentPath = "Environment=PATH=%h/.local/bin:/usr/local/bin:/usr/bin:/bin"
)

// PlannerServiceUnitOptions render the systemd .service unit that fires the
// architecture-planner-loop one-shot from a timer.
type PlannerServiceUnitOptions struct {
	PlannerPath string
	WorkDir     string
}

func RenderPlannerServiceUnit(opts PlannerServiceUnitOptions) string {
	return fmt.Sprintf(`[Unit]
Description=Gormes architecture planner (one-shot, fired by timer)

[Service]
Type=oneshot
%s
WorkingDirectory=%s
ExecStart=%s
TimeoutStartSec=30min
Nice=10
`, plannerEnvironmentPath, systemdPathValue(opts.WorkDir), systemdPathValue(opts.PlannerPath))
}

// PlannerTimerUnitOptions render the systemd .timer that periodically fires
// the planner service. OnBootSec is intentionally short so a freshly booted
// host catches up on planning quickly; OnUnitActiveSec defaults to 6h so
// planner runs do not crowd autoloop or burn quota.
type PlannerTimerUnitOptions struct {
	ServiceUnitName string
	Interval        string
}

func RenderPlannerTimerUnit(opts PlannerTimerUnitOptions) string {
	interval := strings.TrimSpace(opts.Interval)
	if interval == "" {
		interval = PlannerInterval
	}
	return fmt.Sprintf(`[Unit]
Description=Run Gormes architecture-planner-loop periodically

[Timer]
OnBootSec=10min
OnUnitActiveSec=%s
AccuracySec=1min
Persistent=true
Unit=%s

[Install]
WantedBy=timers.target
`, interval, opts.ServiceUnitName)
}

// PlannerServiceInstallOptions describe how to write planner systemd units to
// the user unit directory.
type PlannerServiceInstallOptions struct {
	Runner      autoloop.Runner
	UnitDir     string
	UnitName    string
	TimerName   string
	PlannerPath string
	WorkDir     string
	Interval    string
	AutoStart   bool
	Force       bool
}

func InstallPlannerService(ctx context.Context, opts PlannerServiceInstallOptions) error {
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
		runner = autoloop.ExecRunner{}
	}

	if err := os.MkdirAll(opts.UnitDir, 0o755); err != nil {
		return err
	}

	servicePath := filepath.Join(opts.UnitDir, opts.UnitName)
	if err := writePlannerUnit(servicePath, opts.Force, RenderPlannerServiceUnit(PlannerServiceUnitOptions{
		PlannerPath: opts.PlannerPath,
		WorkDir:     opts.WorkDir,
	})); err != nil {
		return err
	}

	timerPath := filepath.Join(opts.UnitDir, opts.TimerName)
	if err := writePlannerUnit(timerPath, opts.Force, RenderPlannerTimerUnit(PlannerTimerUnitOptions{
		ServiceUnitName: opts.UnitName,
		Interval:        opts.Interval,
	})); err != nil {
		return err
	}

	if result := runner.Run(ctx, autoloop.Command{Name: "systemctl", Args: []string{"--user", "daemon-reload"}}); result.Err != nil {
		return result.Err
	}
	if opts.AutoStart {
		result := runner.Run(ctx, autoloop.Command{
			Name: "systemctl",
			Args: []string{"--user", "enable", "--now", opts.TimerName},
		})
		if result.Err != nil {
			return result.Err
		}
	}

	return nil
}

func writePlannerUnit(path string, force bool, contents string) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.WriteFile(path, []byte(contents), 0o644)
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
