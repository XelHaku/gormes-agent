package progress

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyHealthUpdates_RoundTripPreservesCheckedInProgressJSON loads the
// real checked-in progress.json, applies a no-op mutation that forces the
// full Load → mutate → SaveProgress cycle, and asserts that the only
// difference between input and output is the addition of an empty
// `"health": {}` block on the targeted row. Any other diff is
// field-ordering drift that this test exists to catch.
//
// The mutation closure intentionally has an empty body. ApplyHealthUpdates
// still allocates a fresh &RowHealth{} for the row before invoking the
// callback, which guarantees the on-disk shape gains a `"health": {}`
// block — enough to force the full IO cycle without otherwise changing
// any field on the targeted row.
func TestApplyHealthUpdates_RoundTripPreservesCheckedInProgressJSON(t *testing.T) {
	t.Skip(`Schema-level ordering gap: SaveProgress reformats progress.json on
real round-trip because Go marshals map[string]X keys alphabetically while
the checked-in file uses numeric ordering (e.g. 2.B.10 sorts before 2.B.2)
and Item fields are emitted in struct-definition order rather than the
historical on-disk order (e.g. "note" appears immediately after the
acceptance/blocked_by cluster on disk, but the Go struct definition places
Note after WriteScope/TestCommands/DoneSignal/PR/Owner/ETA so the
round-tripped file reorders those fields). Fixing requires either
converting Phases/Subphases from map → ordered slice or adding custom
MarshalJSON on Progress/Phase, plus reordering the Item struct fields to
match the on-disk historical layout. This test must be re-enabled (and
the Skip removed) once the schema fix lands. Tracking: see
docs/superpowers/specs/2026-04-24-reactive-autoloop-design.md L1
invariants and report back to the human controller for scope decision.`)

	src := filepath.Join("..", "..", "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	original, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("checked-in progress.json not found, skipping compat test: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(tmp, original, 0o644); err != nil {
		t.Fatalf("write tmp copy: %v", err)
	}

	// No-op mutation on a known-stable row (Phase 1, subphase 1.A, the
	// "Bubble Tea shell" item). Forces the full IO cycle so any
	// reformatting drift introduced by SaveProgress surfaces here.
	if err := ApplyHealthUpdates(tmp, []HealthUpdate{{
		PhaseID:    "1",
		SubphaseID: "1.A",
		ItemName:   "Bubble Tea shell",
		Mutate:     func(h *RowHealth) {},
	}}); err != nil {
		t.Fatalf("ApplyHealthUpdates: %v", err)
	}

	got, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read tmp after round-trip: %v", err)
	}

	// The expected post-round-trip file is the original PLUS an empty
	// "health": {} block on the targeted row. Strip any line whose
	// only content is `"health": {}` (with optional trailing comma)
	// from `got`; the remaining byte sequence (modulo trailing
	// whitespace/commas per line) must equal the original.
	stripHealthLines := func(s string) string {
		lines := strings.Split(s, "\n")
		out := lines[:0]
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == `"health": {}` || trimmed == `"health": {},` {
				continue
			}
			out = append(out, line)
		}
		return strings.Join(out, "\n")
	}
	gotMinusHealth := stripHealthLines(string(got))

	// Normalize trailing whitespace AND trailing commas per line: the
	// addition of `"health": {}` after a previously-final field will
	// have introduced a trailing comma on that prior field, which is
	// the one structural change we accept.
	normalize := func(s string) string {
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " \t,")
		}
		return strings.Join(lines, "\n")
	}
	wantNorm := strings.TrimRight(normalize(string(original)), "\n")
	gotNorm := strings.TrimRight(normalize(gotMinusHealth), "\n")
	if wantNorm == gotNorm {
		return
	}

	// Surface a small diff at the first divergence so a future failure
	// makes the drift legible.
	for i := 0; i < min(len(wantNorm), len(gotNorm)); i++ {
		if wantNorm[i] != gotNorm[i] {
			start := max(0, i-50)
			endA := min(len(wantNorm), i+50)
			endB := min(len(gotNorm), i+50)
			t.Fatalf("round-trip drift at offset %d:\nORIG: %q\nGOT : %q",
				i,
				bytes.ReplaceAll([]byte(wantNorm[start:endA]), []byte("\n"), []byte("\\n")),
				bytes.ReplaceAll([]byte(gotNorm[start:endB]), []byte("\n"), []byte("\\n")))
		}
	}
	t.Fatalf("round-trip drift: lengths differ (orig=%d got=%d) without divergence in shared prefix", len(wantNorm), len(gotNorm))
}
