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

// TestSaveProgress_IdempotentOnRealCheckedInFile verifies the schema fix
// (Item field order + natural-sort map keys via custom MarshalJSON) by
// loading the real checked-in progress.json, applying a non-trivial
// mutation that bypasses insertEmptyHealthBlock, saving via SaveProgress,
// then loading and saving again. The second round-trip must be byte-equal
// to the first — that's the idempotency contract.
//
// We can't compare against the on-disk file directly because the on-disk
// file isn't yet in canonical form (e.g. subphase 2.B.10 currently sits
// after 2.G in insertion order rather than between 2.B.5 and 2.B.11 in
// natural-numeric order). The first SaveProgress will canonicalize it;
// every subsequent SaveProgress must produce identical bytes.
func TestSaveProgress_IdempotentOnRealCheckedInFile(t *testing.T) {
	src := filepath.Join("..", "..", "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	original, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("checked-in progress.json not found, skipping idempotency test: %v", err)
	}

	tmp1 := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(tmp1, original, 0o644); err != nil {
		t.Fatalf("write tmp1: %v", err)
	}

	// First round-trip: mutation that SETS a non-zero field, so
	// insertEmptyHealthBlock cannot short-circuit. We use the same target
	// row as TestApplyHealthUpdates_RoundTripPreservesCheckedInProgressJSON
	// so failure modes line up if both tests fail together.
	if err := ApplyHealthUpdates(tmp1, []HealthUpdate{{
		PhaseID:    "1",
		SubphaseID: "1.A",
		ItemName:   "Bubble Tea shell",
		Mutate: func(h *RowHealth) {
			h.AttemptCount = 1 // non-zero, defeats the empty-block optimization
		},
	}}); err != nil {
		t.Fatalf("first ApplyHealthUpdates: %v", err)
	}

	pass1, err := os.ReadFile(tmp1)
	if err != nil {
		t.Fatalf("read after first round-trip: %v", err)
	}

	// Second round-trip on a fresh temp copy of pass1: no further mutation,
	// just Load → SaveProgress. The output must be byte-equal to pass1.
	tmp2 := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(tmp2, pass1, 0o644); err != nil {
		t.Fatalf("write tmp2: %v", err)
	}
	prog, err := Load(tmp2)
	if err != nil {
		t.Fatalf("Load tmp2: %v", err)
	}
	if err := SaveProgress(tmp2, prog); err != nil {
		t.Fatalf("second SaveProgress: %v", err)
	}
	pass2, err := os.ReadFile(tmp2)
	if err != nil {
		t.Fatalf("read after second round-trip: %v", err)
	}

	if bytes.Equal(pass1, pass2) {
		return // idempotent — schema fix works
	}

	// Surface the first divergence so a future failure is debuggable.
	for i := 0; i < min(len(pass1), len(pass2)); i++ {
		if pass1[i] != pass2[i] {
			start := max(0, i-50)
			endA := min(len(pass1), i+50)
			endB := min(len(pass2), i+50)
			t.Fatalf("SaveProgress not idempotent at offset %d:\nPASS1: %q\nPASS2: %q",
				i,
				bytes.ReplaceAll(pass1[start:endA], []byte("\n"), []byte("\\n")),
				bytes.ReplaceAll(pass2[start:endB], []byte("\n"), []byte("\\n")))
		}
	}
	t.Fatalf("SaveProgress not idempotent: lengths differ pass1=%d pass2=%d", len(pass1), len(pass2))
}
