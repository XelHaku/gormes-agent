package architectureplanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

func TestRenderPlannerServiceUnitQuotesPaths(t *testing.T) {
	unit := RenderPlannerServiceUnit(PlannerServiceUnitOptions{
		PlannerPath: `/opt/gormes/bin/planner "loop"`,
		WorkDir:     `/srv/gormes agent`,
	})

	for _, want := range []string{
		"Type=oneshot",
		"Environment=PATH=",
		`WorkingDirectory="/srv/gormes agent"`,
		`ExecStart="/opt/gormes/bin/planner \"loop\""`,
		"TimeoutStartSec=30min",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("service unit missing %q:\n%s", want, unit)
		}
	}
}

func TestRenderPlannerTimerUnitDefaultsInterval(t *testing.T) {
	timer := RenderPlannerTimerUnit(PlannerTimerUnitOptions{ServiceUnitName: "gormes-planner.service"})

	for _, want := range []string{
		"OnBootSec=10min",
		"OnUnitActiveSec=6h",
		"Persistent=true",
		"Unit=gormes-planner.service",
	} {
		if !strings.Contains(timer, want) {
			t.Fatalf("timer unit missing %q:\n%s", want, timer)
		}
	}
}

func TestInstallPlannerServiceWritesUnitsAndEnablesTimer(t *testing.T) {
	unitDir := t.TempDir()
	runner := &autoloop.FakeRunner{Results: []autoloop.Result{{}, {}}}

	err := InstallPlannerService(context.Background(), PlannerServiceInstallOptions{
		Runner:      runner,
		UnitDir:     unitDir,
		UnitName:    "gormes-planner.service",
		TimerName:   "gormes-planner.timer",
		PlannerPath: "/opt/gormes/bin/architecture-planner-loop",
		WorkDir:     "/srv/gormes",
		AutoStart:   true,
	})
	if err != nil {
		t.Fatalf("InstallPlannerService() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(unitDir, "gormes-planner.service")); err != nil {
		t.Fatalf("service unit missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "gormes-planner.timer")); err != nil {
		t.Fatalf("timer unit missing: %v", err)
	}
	if got, want := len(runner.Commands), 2; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	if strings.Join(runner.Commands[0].Args, " ") != "--user daemon-reload" {
		t.Fatalf("daemon-reload args = %#v", runner.Commands[0].Args)
	}
	if strings.Join(runner.Commands[1].Args, " ") != "--user enable --now gormes-planner.timer" {
		t.Fatalf("enable args = %#v", runner.Commands[1].Args)
	}
}
