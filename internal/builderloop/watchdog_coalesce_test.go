package builderloop

import (
	"testing"
	"time"
)

func TestDecideCheckpoint_FirstTickInFreshWindowReturnsFirst(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	decision, next := DecideCheckpoint(now, CheckpointState{}, CoalesceConfig{
		Dirty:        true,
		NextWindowID: func() string { return "w-1" },
	})

	if decision != DecisionFirst {
		t.Fatalf("decision = %q, want %q", decision, DecisionFirst)
	}
	want := CheckpointState{LastCheckpointAt: now, WindowID: "w-1"}
	if next != want {
		t.Fatalf("state = %#v, want %#v", next, want)
	}
}

func TestDecideCheckpoint_LaterTickInsideWindowReturnsAmend(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 30, 0, time.UTC)
	prior := CheckpointState{
		LastCheckpointAt: now.Add(-30 * time.Second),
		LastSubject:      "builder-loop: watchdog checkpoint w-1",
		WindowID:         "w-1",
	}
	nextWindowIDCalled := false

	decision, next := DecideCheckpoint(now, prior, CoalesceConfig{
		WindowSeconds: 600,
		Dirty:         true,
		NextWindowID: func() string {
			nextWindowIDCalled = true
			return "w-unexpected"
		},
	})

	if nextWindowIDCalled {
		t.Fatalf("NextWindowID was called inside an active window")
	}
	if decision != DecisionAmend {
		t.Fatalf("decision = %q, want %q", decision, DecisionAmend)
	}
	want := CheckpointState{
		LastCheckpointAt: now,
		LastSubject:      prior.LastSubject,
		WindowID:         "w-1",
	}
	if next != want {
		t.Fatalf("state = %#v, want %#v", next, want)
	}
}

func TestDecideCheckpoint_LaterTickPastWindowReturnsFirst(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 10, 1, 0, time.UTC)
	prior := CheckpointState{
		LastCheckpointAt: now.Add(-601 * time.Second),
		LastSubject:      "builder-loop: watchdog checkpoint w-1",
		WindowID:         "w-1",
	}

	decision, next := DecideCheckpoint(now, prior, CoalesceConfig{
		WindowSeconds: 600,
		Dirty:         true,
		NextWindowID:  func() string { return "w-2" },
	})

	if decision != DecisionFirst {
		t.Fatalf("decision = %q, want %q", decision, DecisionFirst)
	}
	want := CheckpointState{
		LastCheckpointAt: now,
		LastSubject:      prior.LastSubject,
		WindowID:         "w-2",
	}
	if next != want {
		t.Fatalf("state = %#v, want %#v", next, want)
	}
}

func TestDecideCheckpoint_NoopWhenNotDirty(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 11, 0, 0, time.UTC)
	prior := CheckpointState{
		LastCheckpointAt: now.Add(-30 * time.Second),
		LastSubject:      "builder-loop: watchdog checkpoint w-1",
		WindowID:         "w-1",
	}
	nextWindowIDCalled := false

	decision, next := DecideCheckpoint(now, prior, CoalesceConfig{
		WindowSeconds: 600,
		Dirty:         false,
		NextWindowID: func() string {
			nextWindowIDCalled = true
			return "w-unexpected"
		},
	})

	if nextWindowIDCalled {
		t.Fatalf("NextWindowID was called when worktree is not dirty")
	}
	if decision != DecisionNoop {
		t.Fatalf("decision = %q, want %q", decision, DecisionNoop)
	}
	if next != prior {
		t.Fatalf("state = %#v, want unchanged %#v", next, prior)
	}
}

func TestDecideCheckpoint_DefaultWindowWhenZeroOrNegative(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 30, 0, time.UTC)
	prior := CheckpointState{
		LastCheckpointAt: now.Add(-30 * time.Second),
		LastSubject:      "builder-loop: watchdog checkpoint w-1",
		WindowID:         "w-1",
	}

	for _, windowSeconds := range []int{0, -5} {
		t.Run(time.Duration(windowSeconds).String(), func(t *testing.T) {
			nextWindowIDCalled := false

			decision, next := DecideCheckpoint(now, prior, CoalesceConfig{
				WindowSeconds: windowSeconds,
				Dirty:         true,
				NextWindowID: func() string {
					nextWindowIDCalled = true
					return "w-unexpected"
				},
			})

			if nextWindowIDCalled {
				t.Fatalf("NextWindowID was called inside the default window")
			}
			if decision != DecisionAmend {
				t.Fatalf("decision = %q, want %q", decision, DecisionAmend)
			}
			want := CheckpointState{
				LastCheckpointAt: now,
				LastSubject:      prior.LastSubject,
				WindowID:         "w-1",
			}
			if next != want {
				t.Fatalf("state = %#v, want %#v", next, want)
			}
		})
	}
}
