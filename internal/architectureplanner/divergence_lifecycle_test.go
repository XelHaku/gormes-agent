package architectureplanner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// TestLifecycle_DivergenceFullCycle walks one subphase through the full
// Phase D divergence lifecycle using direct progress.SaveProgress calls and
// the planner-side helpers (no LLM). Five runs:
//
//	Run 1: subphase has no drift_state; planner stamps status="porting".
//	Run 2: planner adds 3 rows with provenance.origin_type="upstream".
//	Run 3: rows ship; planner promotes status to "converged"
//	       (assert DriftPromotion porting->converged lands in the ledger).
//	Run 4: Gormes-original extension lands; planner adds a row with
//	       provenance.origin_type="gormes" and promotes status to "owned"
//	       (assert DriftPromotion converged->owned lands in the ledger).
//	Run 5: a Gormes-original impl tree is synthesized and ScanImplementation
//	       lists the subphase in ImplInventory.OwnedSubphases — the signal
//	       the L3 prompt clause uses to stop reading upstream for owned
//	       subphases.
//
// The LLM is mocked: each "planner run" simulates a successful regen by
// directly editing the after-doc (rows, drift_state, provenance) and
// invoking diffSubphaseStates + AppendLedgerEvent on the resulting
// before/after pair. This is the catch-all integration check that the
// divergence-awareness loop composes end to end. If any of the five run
// boundaries diverges from expectation the failure message names which
// boundary failed and what diverged.
func TestLifecycle_DivergenceFullCycle(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	plannerLedgerPath := filepath.Join(dir, "planner-runs.jsonl")

	const phaseID = "5"
	const subphaseID = "5.O"
	// SubphaseID emitted by diffSubphaseStates is "phaseID.subphaseID" (the
	// existing drift.go convention). Pre-compute once so the assertions don't
	// drift if the convention changes.
	const driftID = phaseID + "." + subphaseID

	// Seed a single subphase with NO drift_state — the planner will stamp
	// the first one in Run 1.
	writeDivergenceProgress(t, progressPath, phaseID, subphaseID)

	t0 := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	// ---- Run 1: planner stamps DriftState{Status:"porting"} ----
	r1Time := t0
	beforeDoc1, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("Run 1: load before-doc: %v", err)
	}
	mutateSubphase(t, progressPath, phaseID, subphaseID, func(sub *progress.Subphase) {
		sub.DriftState = &progress.DriftState{
			Status:            "porting",
			LastUpstreamCheck: r1Time.UTC().Format(time.RFC3339),
			OriginDecision:    "subphase observed; first DriftState stamp",
		}
	})
	afterDoc1, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("Run 1: load after-doc: %v", err)
	}
	// nil -> "porting" is a no-op (default semantics): no DriftPromotion
	// should be emitted because the from-rank and to-rank are both zero.
	promotions1 := diffSubphaseStates(beforeDoc1, afterDoc1)
	if len(promotions1) != 0 {
		t.Fatalf("Run 1: stamping nil->porting should NOT emit a promotion, got %+v", promotions1)
	}
	if err := AppendLedgerEvent(plannerLedgerPath, LedgerEvent{
		TS:              r1Time.UTC().Format(time.RFC3339),
		RunID:           "P1",
		Status:          "ok",
		DriftPromotions: promotions1,
	}); err != nil {
		t.Fatalf("Run 1: append ledger: %v", err)
	}
	// Sanity: drift_state survives the round trip and reads back as "porting".
	if got := loadSubphase(t, progressPath, phaseID, subphaseID); got.DriftState == nil || got.DriftState.Status != "porting" {
		t.Fatalf("Run 1: expected DriftState.Status=porting, got %+v", got.DriftState)
	}

	// ---- Run 2: planner adds 3 rows with provenance.origin_type="upstream" ----
	r2Time := t0.Add(1 * time.Hour)
	beforeDoc2, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("Run 2: load before-doc: %v", err)
	}
	mutateSubphase(t, progressPath, phaseID, subphaseID, func(sub *progress.Subphase) {
		for i := 1; i <= 3; i++ {
			sub.Items = append(sub.Items, progress.Item{
				Name:           "upstream-row-" + itoa(i),
				Status:         progress.StatusPlanned,
				Contract:       "port upstream behaviour " + itoa(i),
				ContractStatus: progress.ContractStatusDraft,
				Provenance: &progress.Provenance{
					OriginType:  "upstream",
					UpstreamRef: "hermes:gateway/api_server.py@abc12" + itoa(i),
				},
			})
		}
	})
	afterDoc2, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("Run 2: load after-doc: %v", err)
	}
	rowsChanged2 := diffRows(beforeDoc2, afterDoc2)
	if len(rowsChanged2) != 3 {
		t.Fatalf("Run 2: expected 3 added RowChange entries, got %d: %+v", len(rowsChanged2), rowsChanged2)
	}
	for _, rc := range rowsChanged2 {
		if rc.Kind != "added" {
			t.Fatalf("Run 2: expected Kind=added, got %+v", rc)
		}
	}
	promotions2 := diffSubphaseStates(beforeDoc2, afterDoc2)
	if len(promotions2) != 0 {
		t.Fatalf("Run 2: status unchanged should NOT emit a promotion, got %+v", promotions2)
	}
	if err := AppendLedgerEvent(plannerLedgerPath, LedgerEvent{
		TS:              r2Time.UTC().Format(time.RFC3339),
		RunID:           "P2",
		Status:          "ok",
		RowsChanged:     rowsChanged2,
		DriftPromotions: promotions2,
	}); err != nil {
		t.Fatalf("Run 2: append ledger: %v", err)
	}
	// Sanity: every row carries Provenance.OriginType="upstream".
	sub2 := loadSubphase(t, progressPath, phaseID, subphaseID)
	if len(sub2.Items) != 3 {
		t.Fatalf("Run 2: expected 3 items in subphase, got %d", len(sub2.Items))
	}
	for _, it := range sub2.Items {
		if it.Provenance == nil || it.Provenance.OriginType != "upstream" {
			t.Fatalf("Run 2: row %q missing Provenance.OriginType=upstream, got %+v", it.Name, it.Provenance)
		}
	}

	// ---- Run 3: rows ship; planner promotes to "converged" ----
	r3Time := t0.Add(2 * time.Hour)
	beforeDoc3, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("Run 3: load before-doc: %v", err)
	}
	mutateSubphase(t, progressPath, phaseID, subphaseID, func(sub *progress.Subphase) {
		// Mark rows shipped to simulate autoloop completing them.
		for i := range sub.Items {
			sub.Items[i].Status = progress.StatusComplete
		}
		// Forward promotion: porting -> converged.
		sub.DriftState = &progress.DriftState{
			Status:            "converged",
			LastUpstreamCheck: r3Time.UTC().Format(time.RFC3339),
			OriginDecision:    "all rows shipped; behaviour matches upstream",
		}
	})
	afterDoc3, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("Run 3: load after-doc: %v", err)
	}
	promotions3 := diffSubphaseStates(beforeDoc3, afterDoc3)
	if len(promotions3) != 1 {
		t.Fatalf("Run 3: expected 1 DriftPromotion, got %d: %+v", len(promotions3), promotions3)
	}
	if got := promotions3[0]; got.SubphaseID != driftID || got.From != "porting" || got.To != "converged" {
		t.Fatalf("Run 3: expected promotion %s porting->converged, got %+v", driftID, got)
	}
	if promotions3[0].Reason != "all rows shipped; behaviour matches upstream" {
		t.Fatalf("Run 3: expected Reason='all rows shipped; behaviour matches upstream', got %q", promotions3[0].Reason)
	}
	if err := AppendLedgerEvent(plannerLedgerPath, LedgerEvent{
		TS:              r3Time.UTC().Format(time.RFC3339),
		RunID:           "P3",
		Status:          "ok",
		DriftPromotions: promotions3,
	}); err != nil {
		t.Fatalf("Run 3: append ledger: %v", err)
	}

	// ---- Run 4: Gormes-original row + promote to "owned" ----
	r4Time := t0.Add(3 * time.Hour)
	beforeDoc4, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("Run 4: load before-doc: %v", err)
	}
	mutateSubphase(t, progressPath, phaseID, subphaseID, func(sub *progress.Subphase) {
		sub.Items = append(sub.Items, progress.Item{
			Name:           "gormes-extension",
			Status:         progress.StatusComplete,
			Contract:       "Gormes-original extension with no upstream analog",
			ContractStatus: progress.ContractStatusDraft,
			WriteScope:     []string{"internal/architectureplanner/"},
			Provenance: &progress.Provenance{
				OriginType: "gormes",
				OwnedSince: r4Time.UTC().Format(time.RFC3339),
				Note:       "no upstream analog; planner-original",
			},
		})
		// Forward promotion: converged -> owned.
		sub.DriftState = &progress.DriftState{
			Status:            "owned",
			LastUpstreamCheck: r4Time.UTC().Format(time.RFC3339),
			OriginDecision:    "Gormes-original extension shipped; subphase fully owned",
		}
	})
	afterDoc4, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("Run 4: load after-doc: %v", err)
	}
	rowsChanged4 := diffRows(beforeDoc4, afterDoc4)
	if len(rowsChanged4) != 1 || rowsChanged4[0].Kind != "added" || rowsChanged4[0].ItemName != "gormes-extension" {
		t.Fatalf("Run 4: expected single added gormes-extension RowChange, got %+v", rowsChanged4)
	}
	promotions4 := diffSubphaseStates(beforeDoc4, afterDoc4)
	if len(promotions4) != 1 {
		t.Fatalf("Run 4: expected 1 DriftPromotion, got %d: %+v", len(promotions4), promotions4)
	}
	if got := promotions4[0]; got.SubphaseID != driftID || got.From != "converged" || got.To != "owned" {
		t.Fatalf("Run 4: expected promotion %s converged->owned, got %+v", driftID, got)
	}
	if err := AppendLedgerEvent(plannerLedgerPath, LedgerEvent{
		TS:              r4Time.UTC().Format(time.RFC3339),
		RunID:           "P4",
		Status:          "ok",
		RowsChanged:     rowsChanged4,
		DriftPromotions: promotions4,
	}); err != nil {
		t.Fatalf("Run 4: append ledger: %v", err)
	}
	// Sanity: the gormes-extension row reads back with origin_type="gormes".
	sub4 := loadSubphase(t, progressPath, phaseID, subphaseID)
	var gormesRow *progress.Item
	for i := range sub4.Items {
		if sub4.Items[i].Name == "gormes-extension" {
			gormesRow = &sub4.Items[i]
			break
		}
	}
	if gormesRow == nil {
		t.Fatal("Run 4: gormes-extension row missing after round trip")
	}
	if gormesRow.Provenance == nil || gormesRow.Provenance.OriginType != "gormes" {
		t.Fatalf("Run 4: gormes-extension Provenance.OriginType=%v, want gormes", gormesRow.Provenance)
	}

	// ---- Run 5: ScanImplementation surfaces the subphase as owned ----
	// Synthesize a Gormes-original impl tree under the same WriteScope prefix
	// the row in Run 4 declared. The ScanImplementation walk hits cmd/ and
	// internal/, so we mkdir + create one file matching the prefix.
	repoRoot := filepath.Join(dir, "repo")
	implPath := filepath.Join(repoRoot, "internal", "architectureplanner", "lifecycle_marker.go")
	if err := os.MkdirAll(filepath.Dir(implPath), 0o755); err != nil {
		t.Fatalf("Run 5: mkdir impl tree: %v", err)
	}
	if err := os.WriteFile(implPath, []byte("package architectureplanner\n"), 0o644); err != nil {
		t.Fatalf("Run 5: write impl marker: %v", err)
	}
	r5Time := t0.Add(4 * time.Hour)
	if err := os.Chtimes(implPath, r5Time, r5Time); err != nil {
		t.Fatalf("Run 5: chtimes: %v", err)
	}

	prefixes := []string{"internal/architectureplanner/"}
	inv, err := ScanImplementation(repoRoot, prefixes, 7*24*time.Hour, r5Time)
	if err != nil {
		t.Fatalf("Run 5: ScanImplementation: %v", err)
	}
	wantPath := "internal/architectureplanner/lifecycle_marker.go"
	if !contains(inv.GormesOriginalPaths, wantPath) {
		t.Fatalf("Run 5: GormesOriginalPaths=%v missing %q", inv.GormesOriginalPaths, wantPath)
	}
	if !contains(inv.RecentlyChanged, wantPath) {
		t.Fatalf("Run 5: RecentlyChanged=%v missing %q", inv.RecentlyChanged, wantPath)
	}
	// computeOwnedSubphases inspects the loaded progress doc — it must list
	// 5.O because every row's WriteScope (only the gormes-extension row sets
	// one in this test) falls under the Gormes-original prefix.
	progFinal, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("Run 5: load progress: %v", err)
	}
	owned := computeOwnedSubphases(progFinal, prefixes)
	if !contains(owned, subphaseID) {
		t.Fatalf("Run 5: computeOwnedSubphases=%v missing %q (planner should stop reading upstream)",
			owned, subphaseID)
	}

	// Final ledger forensics: confirm the two promotions persisted across
	// the whole run sequence and stay in chronological order.
	events, err := LoadLedger(plannerLedgerPath)
	if err != nil {
		t.Fatalf("Run 5: load ledger: %v", err)
	}
	var allPromotions []DriftPromotion
	for _, ev := range events {
		allPromotions = append(allPromotions, ev.DriftPromotions...)
	}
	if len(allPromotions) != 2 {
		t.Fatalf("Run 5: expected 2 total DriftPromotions across the run history, got %d: %+v",
			len(allPromotions), allPromotions)
	}
	if allPromotions[0].From != "porting" || allPromotions[0].To != "converged" {
		t.Fatalf("Run 5: first ledger promotion = %+v, want porting->converged", allPromotions[0])
	}
	if allPromotions[1].From != "converged" || allPromotions[1].To != "owned" {
		t.Fatalf("Run 5: second ledger promotion = %+v, want converged->owned", allPromotions[1])
	}
}

// writeDivergenceProgress seeds a progress.json with a single empty
// subphase. The lifecycle test layers rows + drift_state on top across
// successive runs.
func writeDivergenceProgress(t *testing.T, path, phaseID, subphaseID string) {
	t.Helper()
	prog := &progress.Progress{
		Meta: progress.Meta{Version: "1"},
		Phases: map[string]progress.Phase{
			phaseID: {
				Name:        "Phase " + phaseID,
				Deliverable: "divergence lifecycle harness",
				Subphases: map[string]progress.Subphase{
					subphaseID: {
						Name: "Sub " + subphaseID,
					},
				},
			},
		},
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir progress dir: %v", err)
	}
	if err := progress.SaveProgress(path, prog); err != nil {
		t.Fatalf("write progress: %v", err)
	}
}

// mutateSubphase loads progress.json, applies fn to the named subphase,
// and saves it back. Centralised so each run boundary in the lifecycle
// reads as one logical edit.
func mutateSubphase(t *testing.T, path, phaseID, subphaseID string, fn func(*progress.Subphase)) {
	t.Helper()
	prog, err := progress.Load(path)
	if err != nil {
		t.Fatalf("mutateSubphase load: %v", err)
	}
	phase, ok := prog.Phases[phaseID]
	if !ok {
		t.Fatalf("mutateSubphase: phase %q not found", phaseID)
	}
	sub, ok := phase.Subphases[subphaseID]
	if !ok {
		t.Fatalf("mutateSubphase: subphase %q not found in phase %q", subphaseID, phaseID)
	}
	fn(&sub)
	phase.Subphases[subphaseID] = sub
	prog.Phases[phaseID] = phase
	if err := progress.SaveProgress(path, prog); err != nil {
		t.Fatalf("mutateSubphase save: %v", err)
	}
}

// loadSubphase fetches one subphase from progress.json. Helper exists to
// keep the per-run sanity assertions tight.
func loadSubphase(t *testing.T, path, phaseID, subphaseID string) progress.Subphase {
	t.Helper()
	prog, err := progress.Load(path)
	if err != nil {
		t.Fatalf("loadSubphase: %v", err)
	}
	phase, ok := prog.Phases[phaseID]
	if !ok {
		t.Fatalf("loadSubphase: phase %q not found", phaseID)
	}
	sub, ok := phase.Subphases[subphaseID]
	if !ok {
		t.Fatalf("loadSubphase: subphase %q not found in phase %q", subphaseID, phaseID)
	}
	return sub
}

// contains returns true when the slice contains the string. Tiny helper to
// keep the assertion calls readable.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// itoa is a stack-allocated single-digit Itoa for the row-name suffixes
// used by the lifecycle test. Avoids strconv just to keep the divergence
// test self-contained.
func itoa(n int) string {
	if n < 0 || n > 9 {
		return "?"
	}
	return string(rune('0' + n))
}
