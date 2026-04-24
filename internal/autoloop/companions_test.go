package autoloop

import (
	"testing"
	"time"
)

func TestCompanionDuePlannerOnCadence(t *testing.T) {
	decision := CompanionDue(CompanionOptions{
		Name:         "planner",
		CurrentCycle: 8,
		EveryNCycles: 4,
		Now:          time.Unix(200, 0),
		LoopSleep:    time.Second,
	}, CompanionState{
		LastCycle: 4,
		LastEpoch: 190,
	})

	if !decision.Run {
		t.Fatal("Run = false, want true")
	}
	if decision.Reason != "cycle cadence reached" {
		t.Fatalf("Reason = %q, want cycle cadence reached", decision.Reason)
	}
}

func TestCompanionSkipsWhenDisabled(t *testing.T) {
	decision := CompanionDue(CompanionOptions{
		Name:         "planner",
		CurrentCycle: 10,
		EveryNCycles: 1,
		Now:          time.Unix(300, 0),
		Disabled:     true,
	}, CompanionState{
		LastCycle: 0,
		LastEpoch: 0,
	})

	if decision.Run {
		t.Fatal("Run = true, want false")
	}
	if decision.Reason != "disabled" {
		t.Fatalf("Reason = %q, want disabled", decision.Reason)
	}
}

func TestCompanionDisabledBeatsDueCadence(t *testing.T) {
	decision := CompanionDue(CompanionOptions{
		Name:          "planner",
		CurrentCycle:  10,
		EveryNCycles:  1,
		EveryDuration: time.Second,
		Now:           time.Unix(300, 0),
		Disabled:      true,
	}, CompanionState{
		LastCycle: 9,
		LastEpoch: 0,
	})

	if decision.Run {
		t.Fatal("Run = true, want false")
	}
	if decision.Reason != "disabled" {
		t.Fatalf("Reason = %q, want disabled", decision.Reason)
	}
}

func TestCompanionExternalRecentBeatsDueCadence(t *testing.T) {
	decision := CompanionDue(CompanionOptions{
		Name:           "planner",
		CurrentCycle:   10,
		EveryNCycles:   1,
		EveryDuration:  time.Second,
		Now:            time.Unix(300, 0),
		ExternalRecent: true,
	}, CompanionState{
		LastCycle: 9,
		LastEpoch: 0,
	})

	if decision.Run {
		t.Fatal("Run = true, want false")
	}
	if decision.Reason != "external scheduler ran recently" {
		t.Fatalf("Reason = %q, want external scheduler ran recently", decision.Reason)
	}
}

func TestCompanionZeroCadencesDoNotRun(t *testing.T) {
	decision := CompanionDue(CompanionOptions{
		Name:         "planner",
		CurrentCycle: 10,
		Now:          time.Unix(300, 0),
	}, CompanionState{
		LastCycle: 0,
		LastEpoch: 0,
	})

	if decision.Run {
		t.Fatal("Run = true, want false")
	}
	if decision.Reason != "not due" {
		t.Fatalf("Reason = %q, want not due", decision.Reason)
	}
}

func TestCompanionTimeCadenceDue(t *testing.T) {
	decision := CompanionDue(CompanionOptions{
		Name:          "planner",
		CurrentCycle:  10,
		EveryDuration: 5 * time.Second,
		Now:           time.Unix(110, 0),
	}, CompanionState{
		LastCycle: 0,
		LastEpoch: 100,
	})

	if !decision.Run {
		t.Fatal("Run = false, want true")
	}
	if decision.Reason != "time cadence reached" {
		t.Fatalf("Reason = %q, want time cadence reached", decision.Reason)
	}
}

func TestCompanionTimeCadenceNotDue(t *testing.T) {
	decision := CompanionDue(CompanionOptions{
		Name:          "planner",
		CurrentCycle:  10,
		EveryDuration: 500 * time.Millisecond,
		Now:           time.Unix(100, int64(100*time.Millisecond)),
	}, CompanionState{
		LastCycle: 0,
		LastEpoch: 100,
	})

	if decision.Run {
		t.Fatal("Run = true, want false")
	}
	if decision.Reason != "not due" {
		t.Fatalf("Reason = %q, want not due", decision.Reason)
	}
}

func TestCompanionBothCadencesNotReached(t *testing.T) {
	decision := CompanionDue(CompanionOptions{
		Name:          "planner",
		CurrentCycle:  12,
		EveryNCycles:  5,
		EveryDuration: 10 * time.Second,
		Now:           time.Unix(105, 0),
	}, CompanionState{
		LastCycle: 10,
		LastEpoch: 100,
	})

	if decision.Run {
		t.Fatal("Run = true, want false")
	}
	if decision.Reason != "not due" {
		t.Fatalf("Reason = %q, want not due", decision.Reason)
	}
}
