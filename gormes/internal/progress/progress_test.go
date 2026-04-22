package progress

import (
	"path/filepath"
	"testing"
)

func itemStatusByName(items []Item) map[string]Status {
	out := make(map[string]Status, len(items))
	for _, it := range items {
		out[it.Name] = it.Status
	}
	return out
}

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
	// Phase 3 has most memory subphases shipped, 3.E.* planned -> in_progress.
	if got := p.Phases["3"].DerivedStatus(); got != StatusInProgress {
		t.Errorf("Phase 3 = %q, want in_progress", got)
	}
	// Phase 4 is entirely planned.
	if got := p.Phases["4"].DerivedStatus(); got != StatusPlanned {
		t.Errorf("Phase 4 = %q, want planned", got)
	}
	// Floor counts — catches mass-deletion regressions without pinning exact values.
	if n := len(p.Phases); n < 6 {
		t.Errorf("phase count = %d, want >= 6", n)
	}
	s := p.Stats()
	if s.Subphases.Total < 50 {
		t.Errorf("subphase total = %d, want >= 50", s.Subphases.Total)
	}
	if s.Items.Total < 100 {
		t.Errorf("item total = %d, want >= 100", s.Items.Total)
	}
}

func TestLoad_RealFile_Phase2Ledger(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cron := p.Phases["2"].Subphases["2.D"]
	if got := cron.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.D = %q, want complete", got)
	}
	cronItems := itemStatusByName(cron.Items)
	for name, want := range map[string]Status{
		"robfig/cron scheduler + bbolt job store":          StatusComplete,
		"SQLite cron_runs audit + CRON.md mirror":          StatusComplete,
		"Heartbeat [SYSTEM:] + [SILENT] delivery contract": StatusComplete,
	} {
		if got := cronItems[name]; got != want {
			t.Errorf("Phase 2.D item %q = %q, want %q", name, got, want)
		}
	}

	gateway := p.Phases["2"].Subphases["2.B.2"]
	if got := gateway.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 2.B.2 = %q, want in_progress", got)
	}
	gatewayItems := itemStatusByName(gateway.Items)
	for name, want := range map[string]Status{
		"Discord": StatusComplete,
		"Slack":   StatusComplete,
	} {
		if got := gatewayItems[name]; got != want {
			t.Errorf("Phase 2.B.2 item %q = %q, want %q", name, got, want)
		}
	}

	skills := p.Phases["2"].Subphases["2.G"]
	if got := skills.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 2.G = %q, want in_progress", got)
	}
	skillItems := itemStatusByName(skills.Items)
	for name, want := range map[string]Status{
		"SKILL.md parsing + active store":        StatusComplete,
		"Deterministic selection + prompt block": StatusComplete,
		"Kernel injection + usage log":           StatusComplete,
		"Candidate drafting + promotion flow":    StatusPlanned,
	} {
		if got := skillItems[name]; got != want {
			t.Errorf("Phase 2.G item %q = %q, want %q", name, got, want)
		}
	}
}
