package autoloop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ServiceUnitOptions struct {
	AutoloopPath string
	WorkDir      string
}

func RenderServiceUnit(opts ServiceUnitOptions) string {
	return fmt.Sprintf(`[Unit]
Description=Gormes autoloop

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s run
Restart=on-failure

[Install]
WantedBy=default.target
`, opts.WorkDir, opts.AutoloopPath)
}

type ServiceInstallOptions struct {
	Runner       Runner
	UnitDir      string
	UnitName     string
	AutoloopPath string
	WorkDir      string
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
		if result.Err != nil {
			return result.Err
		}
	}

	return nil
}
