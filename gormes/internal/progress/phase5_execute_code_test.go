package progress

import (
	"strings"
	"testing"
)

func TestLoad_RealFile_Phase5ExecuteCode(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	exec := p.Phases["5"].Subphases["5.K"]
	if got := exec.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 5.K = %q, want complete", got)
	}

	items := itemsByName(exec.Items)
	sandboxed := items["Sandboxed exec"]
	if sandboxed.Status != StatusComplete {
		t.Fatalf("Phase 5.K Sandboxed exec status = %q, want complete", sandboxed.Status)
	}
	for _, want := range []string{"execute_code", "filesystem/network", "Phase 5.B"} {
		if !strings.Contains(sandboxed.Note, want) {
			t.Fatalf("Phase 5.K note = %q, want substring %q", sandboxed.Note, want)
		}
	}
}
