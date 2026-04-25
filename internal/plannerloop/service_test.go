package plannerloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
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
	runner := &builderloop.FakeRunner{Results: []builderloop.Result{{}, {}, {}}}

	err := InstallPlannerService(context.Background(), PlannerServiceInstallOptions{
		Runner:      runner,
		UnitDir:     unitDir,
		UnitName:    "gormes-planner.service",
		TimerName:   "gormes-planner.timer",
		PathName:    "gormes-planner.path",
		PathToWatch: "/srv/gormes/.codex/architecture-planner/triggers.jsonl",
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
	if _, err := os.Stat(filepath.Join(unitDir, "gormes-planner.path")); err != nil {
		t.Fatalf("path unit missing: %v", err)
	}
	if got, want := len(runner.Commands), 3; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	if strings.Join(runner.Commands[0].Args, " ") != "--user daemon-reload" {
		t.Fatalf("daemon-reload args = %#v", runner.Commands[0].Args)
	}
	if strings.Join(runner.Commands[1].Args, " ") != "--user enable --now gormes-planner.timer" {
		t.Fatalf("enable timer args = %#v", runner.Commands[1].Args)
	}
	if strings.Join(runner.Commands[2].Args, " ") != "--user enable --now gormes-planner.path" {
		t.Fatalf("enable path args = %#v", runner.Commands[2].Args)
	}
}

func TestRenderPlannerPathUnit_ContainsExpectedDirectives(t *testing.T) {
	rendered := RenderPlannerPathUnit(PlannerPathUnitOptions{
		Description: "Trigger Gormes architecture planner on autoloop signal",
		PathToWatch: "/home/test/.codex/architecture-planner/triggers.jsonl",
		ServiceUnit: "gormes-architecture-planner.service",
	})
	wants := []string{
		"PathChanged=/home/test/.codex/architecture-planner/triggers.jsonl",
		"TriggerLimitIntervalSec=60",
		"TriggerLimitBurst=1",
		"Unit=gormes-architecture-planner.service",
		"WantedBy=default.target",
	}
	for _, w := range wants {
		if !strings.Contains(rendered, w) {
			t.Errorf("rendered unit missing %q\n%s", w, rendered)
		}
	}
}

func TestInstallPlannerService_WritesAllThreeUnits(t *testing.T) {
	dir := t.TempDir()
	runner := &builderloop.FakeRunner{Results: []builderloop.Result{{}}}
	opts := PlannerServiceInstallOptions{
		Runner:      runner,
		UnitDir:     dir,
		UnitName:    "gormes-architecture-planner.service",
		TimerName:   "gormes-architecture-planner.timer",
		PathName:    "gormes-architecture-planner.path",
		PathToWatch: "/srv/gormes/.codex/architecture-planner/triggers.jsonl",
		PlannerPath: "/usr/local/bin/planner.sh",
		WorkDir:     "/repo",
	}
	if err := InstallPlannerService(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"gormes-architecture-planner.service",
		"gormes-architecture-planner.timer",
		"gormes-architecture-planner.path",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("unit %s not written: %v", name, err)
		}
	}
}

func TestRenderPlannerImplPathUnit_HasLongerRateLimit(t *testing.T) {
	rendered := RenderPlannerImplPathUnit(PlannerImplPathUnitOptions{
		Description:   "Trigger Gormes architecture planner on impl tree change",
		PathsToWatch:  []string{"/repo/cmd", "/repo/internal"},
		ServiceUnit:   "gormes-architecture-planner.service",
		TriggerReason: "impl_change",
	})
	for _, w := range []string{
		"PathChanged=/repo/cmd",
		"PathChanged=/repo/internal",
		"TriggerLimitIntervalSec=1800",
		"TriggerLimitBurst=1",
		"Unit=gormes-architecture-planner.service",
	} {
		if !strings.Contains(rendered, w) {
			t.Errorf("rendered impl-path unit missing %q\n%s", w, rendered)
		}
	}
}

func TestInstallPlannerService_WritesAllFourUnits(t *testing.T) {
	dir := t.TempDir()
	opts := PlannerServiceInstallOptions{
		Runner:           &builderloop.FakeRunner{Results: []builderloop.Result{{}, {}, {}, {}}},
		UnitDir:          dir,
		UnitName:         "gormes-architecture-planner.service",
		TimerName:        "gormes-architecture-planner.timer",
		PathName:         "gormes-architecture-planner.path",
		ImplPathName:     "gormes-architecture-planner-impl.path",
		PlannerPath:      "/usr/local/bin/planner.sh",
		WorkDir:          "/repo",
		PathToWatch:      "/repo/.codex/architecture-planner/triggers.jsonl",
		ImplPathsToWatch: []string{"/repo/cmd", "/repo/internal"},
	}
	if err := InstallPlannerService(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"gormes-architecture-planner.service",
		"gormes-architecture-planner.timer",
		"gormes-architecture-planner.path",
		"gormes-architecture-planner-impl.path",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("unit %s not written: %v", name, err)
		}
	}
}
