package plannerloop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
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

// PlannerPathUnitOptions render the systemd .path unit that watches the
// triggers ledger. systemd fires the linked service whenever the file
// changes, with rate-limiting so a burst of triggers does not stampede
// the planner.
type PlannerPathUnitOptions struct {
	Description string
	PathToWatch string
	ServiceUnit string
}

// RenderPlannerPathUnit returns a systemd .path unit body. The
// TriggerLimit* directives cap planner firings to once per minute even
// if autoloop appends many trigger events in quick succession.
func RenderPlannerPathUnit(opts PlannerPathUnitOptions) string {
	return fmt.Sprintf(`[Unit]
Description=%s

[Path]
PathChanged=%s
TriggerLimitIntervalSec=60
TriggerLimitBurst=1
Unit=%s

[Install]
WantedBy=default.target
`, opts.Description, opts.PathToWatch, opts.ServiceUnit)
}

// PlannerImplPathUnitOptions render the systemd .path unit that watches
// the impl tree (cmd/, internal/, etc.) so the planner re-runs whenever
// developer activity changes the implementation surface. Distinct from
// the trigger-ledger .path unit (60s rate limit) because dev activity
// is far burstier; we cap to once per 30 minutes so a refactor session
// does not flood the planner with regen requests.
type PlannerImplPathUnitOptions struct {
	Description   string
	PathsToWatch  []string
	ServiceUnit   string
	TriggerReason string
}

const plannerImplPathUnitTemplate = `[Unit]
Description=%s

[Path]
%s
TriggerLimitIntervalSec=1800
TriggerLimitBurst=1
Unit=%s

[Install]
WantedBy=default.target
`

// RenderPlannerImplPathUnit returns a systemd .path unit body that fans
// one PathChanged= line per watched directory. TriggerReason is currently
// informational — it is consumed by the planner via the
// PLANNER_TRIGGER_REASON env var threaded through the service unit (or a
// drop-in), not by the .path unit itself; the field exists so future
// drop-ins can reference the same string the renderer was configured
// with.
func RenderPlannerImplPathUnit(opts PlannerImplPathUnitOptions) string {
	lines := make([]string, 0, len(opts.PathsToWatch))
	for _, p := range opts.PathsToWatch {
		lines = append(lines, "PathChanged="+p)
	}
	return fmt.Sprintf(plannerImplPathUnitTemplate, opts.Description, strings.Join(lines, "\n"), opts.ServiceUnit)
}

// PlannerServiceInstallOptions describe how to write planner systemd units to
// the user unit directory.
type PlannerServiceInstallOptions struct {
	Runner    builderloop.Runner
	UnitDir   string
	UnitName  string
	TimerName string
	// PathName is the filename of the .path unit that reactively fires
	// the planner service on triggers ledger changes. Defaults to
	// "gormes-architecture-planner.path" if empty.
	PathName string
	// PathToWatch is the absolute filesystem path the .path unit
	// watches (autoloop's triggers JSONL ledger). Threaded in from the
	// caller so the planner Config and systemd unit agree on which
	// file represents the trigger source-of-truth.
	PathToWatch string
	// ImplPathName is the filename of the impl-tree .path unit that
	// reactively fires the planner service on impl-tree changes
	// (e.g. cmd/, internal/). Empty disables impl-tree watching;
	// existing 3-unit installs are unaffected.
	ImplPathName string
	// ImplPathsToWatch is the list of absolute directories the impl
	// .path unit watches (e.g. cmd/, internal/). Empty disables
	// impl-tree watching even if ImplPathName is set.
	ImplPathsToWatch []string
	PlannerPath      string
	WorkDir          string
	Interval         string
	AutoStart        bool
	Force            bool
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
		runner = builderloop.ExecRunner{}
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

	pathName := opts.PathName
	if pathName == "" {
		pathName = "gormes-architecture-planner.path"
	}
	pathUnitPath := filepath.Join(opts.UnitDir, pathName)
	if err := writePlannerUnit(pathUnitPath, opts.Force, RenderPlannerPathUnit(PlannerPathUnitOptions{
		Description: "Trigger Gormes architecture planner on autoloop signal",
		PathToWatch: opts.PathToWatch,
		ServiceUnit: opts.UnitName,
	})); err != nil {
		return err
	}

	// Phase D Task 4: optional impl-tree .path unit. Skipped when either
	// the unit name or watched-paths list is empty so existing 3-unit
	// installs keep their original behavior.
	implPathConfigured := opts.ImplPathName != "" && len(opts.ImplPathsToWatch) > 0
	if implPathConfigured {
		implPathUnitPath := filepath.Join(opts.UnitDir, opts.ImplPathName)
		if err := writePlannerUnit(implPathUnitPath, opts.Force, RenderPlannerImplPathUnit(PlannerImplPathUnitOptions{
			Description:   "Trigger Gormes architecture planner on impl tree change",
			PathsToWatch:  opts.ImplPathsToWatch,
			ServiceUnit:   opts.UnitName,
			TriggerReason: "impl_change",
		})); err != nil {
			return err
		}
	}

	if result := runner.Run(ctx, builderloop.Command{Name: "systemctl", Args: []string{"--user", "daemon-reload"}}); result.Err != nil {
		return result.Err
	}
	if opts.AutoStart {
		result := runner.Run(ctx, builderloop.Command{
			Name: "systemctl",
			Args: []string{"--user", "enable", "--now", opts.TimerName},
		})
		if result.Err != nil {
			return result.Err
		}
		result = runner.Run(ctx, builderloop.Command{
			Name: "systemctl",
			Args: []string{"--user", "enable", "--now", pathName},
		})
		if result.Err != nil {
			return result.Err
		}
		if implPathConfigured {
			result = runner.Run(ctx, builderloop.Command{
				Name: "systemctl",
				Args: []string{"--user", "enable", "--now", opts.ImplPathName},
			})
			if result.Err != nil {
				return result.Err
			}
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
