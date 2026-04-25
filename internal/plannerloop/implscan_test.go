package plannerloop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestScanImplementation_DenyListPathsAreOriginal(t *testing.T) {
	dir := t.TempDir()
	// Synth an impl tree.
	for _, p := range []string{
		"cmd/builder-loop/main.go",
		"internal/plannerloop/run.go",
		"internal/plannertriggers/triggers.go",
		"cmd/gormes/main.go",         // NOT original (not in deny list)
		"internal/gateway/server.go", // NOT original
	} {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	denyList := []string{
		"cmd/builder-loop/",
		"internal/plannerloop/",
		"internal/plannertriggers/",
	}
	inv, err := ScanImplementation(dir, denyList, 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("ScanImplementation: %v", err)
	}

	// All deny-listed prefixes should appear in GormesOriginalPaths inventory.
	wantGormes := map[string]bool{
		"cmd/builder-loop/main.go":                 true,
		"internal/plannerloop/run.go":  true,
		"internal/plannertriggers/triggers.go": true,
	}
	gotGormes := map[string]bool{}
	for _, p := range inv.GormesOriginalPaths {
		gotGormes[p] = true
	}
	for w := range wantGormes {
		if !gotGormes[w] {
			t.Errorf("expected %q in GormesOriginalPaths, got %v", w, inv.GormesOriginalPaths)
		}
	}

	// Non-deny paths must NOT appear in GormesOriginalPaths.
	for _, mustNot := range []string{"cmd/gormes/main.go", "internal/gateway/server.go"} {
		if gotGormes[mustNot] {
			t.Errorf("path %q should NOT be in GormesOriginalPaths", mustNot)
		}
	}
}

func TestScanImplementation_RecentlyChangedHonorsLookback(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	recent := filepath.Join(dir, "cmd/builder-loop/recent.go")
	old := filepath.Join(dir, "cmd/builder-loop/old.go")
	for _, p := range []string{recent, old} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(recent, now.Add(-1*time.Hour), now.Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(old, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	inv, err := ScanImplementation(dir, []string{"cmd/builder-loop/"}, 24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"cmd/builder-loop/recent.go": true}
	for _, p := range inv.RecentlyChanged {
		if !want[p] {
			t.Errorf("RecentlyChanged includes %q (older than lookback)", p)
		}
	}
	if len(inv.RecentlyChanged) != 1 {
		t.Errorf("expected 1 recently changed, got %d: %v", len(inv.RecentlyChanged), inv.RecentlyChanged)
	}
}

func TestScanImplementation_MissingDirReturnsEmpty(t *testing.T) {
	inv, err := ScanImplementation("/nonexistent-dir-12345", nil, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(inv.GormesOriginalPaths) != 0 || len(inv.RecentlyChanged) != 0 {
		t.Errorf("expected empty inventory, got %+v", inv)
	}
}

func TestComputeOwnedSubphases_AllWriteScopesUnderOriginalPrefixes(t *testing.T) {
	prog := &progress.Progress{
		Phases: map[string]progress.Phase{
			"5": {
				Subphases: map[string]progress.Subphase{
					"5.O": {
						Items: []progress.Item{
							{Name: "planner", WriteScope: []string{"internal/plannerloop/run.go"}},
							{Name: "autoloop", WriteScope: []string{"cmd/builder-loop/main.go"}},
						},
					},
					"5.P": {
						Items: []progress.Item{
							{Name: "mixed", WriteScope: []string{"internal/plannerloop/run.go", "internal/gateway/server.go"}},
						},
					},
					"5.Q": {
						Items: []progress.Item{{Name: "no-scope"}},
					},
				},
			},
		},
	}

	got := computeOwnedSubphases(prog, []string{"internal/plannerloop/", "cmd/builder-loop/"})
	if len(got) != 1 || got[0] != "5.O" {
		t.Fatalf("computeOwnedSubphases() = %v, want [5.O]", got)
	}
}
