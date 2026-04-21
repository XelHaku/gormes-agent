package progress

import (
	"path/filepath"
	"testing"
)

func TestLoad_MinimalFixture(t *testing.T) {
	p, err := Load(filepath.Join("testdata", "minimal.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if p.Meta.Version != "2.0" {
		t.Errorf("Meta.Version = %q, want %q", p.Meta.Version, "2.0")
	}
	if p.Meta.LastUpdated != "2026-04-20" {
		t.Errorf("Meta.LastUpdated = %q, want %q", p.Meta.LastUpdated, "2026-04-20")
	}
	ph, ok := p.Phases["1"]
	if !ok {
		t.Fatalf("Phases[\"1\"] missing")
	}
	sp, ok := ph.Subphases["1.A"]
	if !ok {
		t.Fatalf("Subphases[\"1.A\"] missing")
	}
	if len(sp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(sp.Items))
	}
	if sp.Items[0].Name != "item one" || sp.Items[0].Status != StatusComplete {
		t.Errorf("items[0] = %+v, want name=item one status=complete", sp.Items[0])
	}
}

func TestLoad_RealFile(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	// Phase 1 must derive as complete (all subphases complete).
	if got := p.Phases["1"].DerivedStatus(); got != StatusComplete {
		t.Errorf("Phase 1 = %q, want complete", got)
	}
	// Phase 2 has 2.A, 2.B.1, 2.C complete and more planned -> in_progress.
	if got := p.Phases["2"].DerivedStatus(); got != StatusInProgress {
		t.Errorf("Phase 2 = %q, want in_progress", got)
	}
	// Phase 4 is entirely planned.
	if got := p.Phases["4"].DerivedStatus(); got != StatusPlanned {
		t.Errorf("Phase 4 = %q, want planned", got)
	}
}
