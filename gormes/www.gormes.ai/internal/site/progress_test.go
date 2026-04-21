package site

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/progress"
)

func TestToneFor(t *testing.T) {
	tests := []struct {
		st   progress.Status
		key  string
		want string
	}{
		{progress.StatusComplete, "1", "shipped"},
		{progress.StatusInProgress, "2", "progress"},
		{progress.StatusPlanned, "4", "planned"},
		{progress.StatusPlanned, "5", "later"}, // Phase 5 gets "later" tone even when planned
		{progress.StatusPlanned, "6", "planned"},
	}
	for _, tc := range tests {
		got := toneFor(tc.st, tc.key)
		if got != tc.want {
			t.Errorf("toneFor(%q, phase=%s) = %q, want %q", tc.st, tc.key, got, tc.want)
		}
	}
}

func TestBuildRoadmapPhases_Counts(t *testing.T) {
	p := &progress.Progress{
		Meta: progress.Meta{Version: "2.0"},
		Phases: map[string]progress.Phase{
			"1": {Name: "Phase 1 — Dashboard", Subphases: map[string]progress.Subphase{
				"1.A": {Items: []progress.Item{{Status: progress.StatusComplete}}},
				"1.B": {Items: []progress.Item{{Status: progress.StatusComplete}}},
			}},
		},
	}
	got := buildRoadmapPhases(p)
	if len(got) != 1 {
		t.Fatalf("len(phases) = %d, want 1", len(got))
	}
	if got[0].StatusTone != "shipped" {
		t.Errorf("StatusTone = %q, want shipped", got[0].StatusTone)
	}
	if got[0].Title != "Phase 1 — Dashboard" {
		t.Errorf("Title = %q", got[0].Title)
	}
}
