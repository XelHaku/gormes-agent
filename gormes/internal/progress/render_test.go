package progress

import (
	"strings"
	"testing"
)

func TestRenderReadmeRollup_Shape(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Name: "Phase 1 — Dashboard", Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Status: StatusComplete}}},
			}},
			"2": {Name: "Phase 2 — Gateway", Subphases: map[string]Subphase{
				"2.A": {Items: []Item{{Status: StatusComplete}}},
				"2.B": {Items: []Item{{Status: StatusPlanned}}},
			}},
		},
	}
	got := RenderReadmeRollup(p)
	if !strings.Contains(got, "| Phase | Status | Shipped |") {
		t.Errorf("rollup missing table header; got:\n%s", got)
	}
	if !strings.Contains(got, "Phase 1 — Dashboard") {
		t.Errorf("rollup missing Phase 1 row; got:\n%s", got)
	}
	if !strings.Contains(got, "1/1") {
		t.Errorf("rollup missing 1/1 count for Phase 1; got:\n%s", got)
	}
	if !strings.Contains(got, "1/2") {
		t.Errorf("rollup missing 1/2 count for Phase 2; got:\n%s", got)
	}
	// Guard against statusIcon() silently returning "".
	if !strings.Contains(got, "✅") {
		t.Errorf("rollup missing shipped icon ✅; got:\n%s", got)
	}
	if !strings.Contains(got, "🔨") {
		t.Errorf("rollup missing in-progress icon 🔨; got:\n%s", got)
	}
}

func TestRenderReadmeRollup_Sorted(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"2": {Name: "Phase 2", Subphases: map[string]Subphase{"2.A": {Status: StatusPlanned}}},
			"1": {Name: "Phase 1", Subphases: map[string]Subphase{"1.A": {Status: StatusComplete}}},
		},
	}
	got := RenderReadmeRollup(p)
	// Match on the table-cell leader "| Phase 1 " (with trailing space) so
	// "Phase 1" does not accidentally match "Phase 10".
	i1 := strings.Index(got, "| Phase 1 ")
	i2 := strings.Index(got, "| Phase 2 ")
	if i1 < 0 || i2 < 0 || i1 > i2 {
		t.Errorf("phases not sorted (i1=%d, i2=%d):\n%s", i1, i2, got)
	}
}

func TestRenderDocsChecklist_StatsLine(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Name: "P1", Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Name: "done", Status: StatusComplete}}},
				"1.B": {Items: []Item{{Name: "todo", Status: StatusPlanned}}},
			}},
		},
	}
	got := RenderDocsChecklist(p)
	if !strings.Contains(got, "**Overall:** 1/2 subphases shipped") {
		t.Errorf("checklist missing overall stats line; got:\n%s", got)
	}
}

func TestRenderDocsChecklist_ItemCheckboxes(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Name: "Phase 1 — Test", Subphases: map[string]Subphase{
				"1.A": {Name: "Alpha", Items: []Item{
					{Name: "done", Status: StatusComplete},
					{Name: "todo", Status: StatusPlanned},
				}},
			}},
		},
	}
	got := RenderDocsChecklist(p)
	if !strings.Contains(got, "- [x] done") {
		t.Errorf("checklist missing checked item; got:\n%s", got)
	}
	if !strings.Contains(got, "- [ ] todo") {
		t.Errorf("checklist missing unchecked item; got:\n%s", got)
	}
	if !strings.Contains(got, "### 1.A — Alpha") {
		t.Errorf("checklist missing subphase header; got:\n%s", got)
	}
}
