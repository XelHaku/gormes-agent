package architectureplanner

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// RenderStatusOptions carries the file paths RenderStatus needs to assemble
// the operator-facing status surface. All fields are required; pass empty
// strings only for paths that legitimately do not exist on disk yet (the
// renderer soft-fails missing files and produces zero-valued sections).
type RenderStatusOptions struct {
	// StatePath is the planner_state.json file emitted by the most recent
	// planner run. Provides the metadata header (last run UTC, backend, mode,
	// progress json, report path, context path).
	StatePath string
	// PlannerLedgerPath is the JSONL planner ledger (typically
	// <RunRoot>/state/runs.jsonl). Used to bucket recent reshape outcomes.
	PlannerLedgerPath string
	// AutoloopLedgerPath is the JSONL autoloop ledger
	// (<AutoloopRunRoot>/state/runs.jsonl). Used to correlate reshapes with
	// what autoloop did to the row afterwards.
	AutoloopLedgerPath string
	// ProgressJSONPath is the canonical progress.json. Walked to inventory
	// rows whose PlannerVerdict.NeedsHuman is true.
	ProgressJSONPath string
	// EvaluationWindow controls the lookback for the "Reshape outcomes (last
	// Nd):" section. Defaults to DefaultEvaluationWindow when zero.
	EvaluationWindow time.Duration
	// Now is the wall clock. Defaults to time.Now().UTC() when zero. Exposed
	// for deterministic tests.
	Now time.Time
}

// RenderStatus assembles the operator-facing status surface for the
// `architecture-planner-loop status` command. Combines the planner_state.json
// metadata header, recent ReshapeOutcome bucket counts, and an inventory of
// rows whose PlannerVerdict.NeedsHuman is true (with suggested operator
// actions derived from the row's last failure category).
//
// Missing input files are tolerated: each section that depends on the missing
// file becomes empty (zero counts, empty inventory) rather than aborting the
// render. This mirrors the planner's "soft-fail observability" stance — a
// status command should always produce *something* operators can read.
func RenderStatus(opts RenderStatusOptions) (string, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	window := opts.EvaluationWindow
	if window <= 0 {
		window = DefaultEvaluationWindow
	}

	state, _ := loadStateFile(opts.StatePath)
	outcomes, _ := Evaluate(opts.PlannerLedgerPath, opts.AutoloopLedgerPath, window, now)
	needsHuman, _ := loadNeedsHumanRows(opts.ProgressJSONPath)
	prog, _ := loadProgressForStatus(opts.ProgressJSONPath)
	plannerEvents, _ := LoadLedgerWindow(opts.PlannerLedgerPath, window, now)

	var b strings.Builder
	fmt.Fprintf(&b, "Last run UTC: %s\n", stateFieldOrUnknown(state, "last_run_utc"))
	fmt.Fprintf(&b, "Backend: %s\n", stateFieldOrUnknown(state, "backend"))
	fmt.Fprintf(&b, "Mode: %s\n", stateFieldOrUnknown(state, "mode"))
	fmt.Fprintf(&b, "Progress JSON: %s\n", stateFieldOrUnknown(state, "progress_json"))
	fmt.Fprintf(&b, "Report: %s\n", stateFieldOrUnknown(state, "report_path"))
	fmt.Fprintf(&b, "Context: %s\n", stateFieldOrUnknown(state, "context_path"))

	windowDays := int(window / (24 * time.Hour))
	if windowDays < 1 {
		windowDays = 1
	}
	fmt.Fprintf(&b, "\nReshape outcomes (last %dd):\n", windowDays)
	buckets := bucketOutcomes(outcomes)
	fmt.Fprintf(&b, "  unstuck: %d\n", buckets["unstuck"])
	fmt.Fprintf(&b, "  still failing: %d\n", buckets["still_failing"])
	fmt.Fprintf(&b, "  no attempts yet: %d\n", buckets["no_attempts_yet"])

	fmt.Fprintf(&b, "\nRows needing human attention: %d\n", len(needsHuman))
	for _, row := range needsHuman {
		category := ""
		if row.Health != nil && row.Health.LastFailure != nil {
			category = string(row.Health.LastFailure.Category)
		}
		reason := strings.TrimSpace(row.Verdict.Reason)
		if reason == "" {
			reason = "(no reason recorded)"
		}
		key := row.PhaseID + "/" + row.SubphaseID + "/" + row.ItemName
		fmt.Fprintf(&b, "  - %s — %s\n", key, reason)
		fmt.Fprintf(&b, "                 reshape count: %d   since: %s\n",
			row.Verdict.ReshapeCount, valueOrDash(row.Verdict.Since))
		fmt.Fprintf(&b, "                 → suggested action: %s\n", SuggestedActionForCategory(category))
	}

	// Phase D Task 5: drift state buckets + recent promotions. Both sections
	// soft-fail to empty when their inputs are missing (no progress.json,
	// empty ledger) so the status surface stays useful on a fresh checkout.
	b.WriteString(renderDriftStateBuckets(prog))
	b.WriteString(renderRecentDriftPromotions(plannerEvents, windowDays))

	return b.String(), nil
}

// renderDriftStateBuckets walks the loaded progress doc and partitions
// every subphase into one of three buckets by DriftState.Status. Subphases
// without a DriftState block default to the "porting" bucket (matches the
// L3 PROVENANCE / DRIFT STATE prompt clause's default semantics). Returns
// the empty string when prog is nil so the section disappears entirely on
// a fresh checkout where progress.json has not been seeded yet.
//
// Subphase IDs are emitted as "phaseID.subphaseID" — the same convention
// diffSubphaseStates uses for DriftPromotion.SubphaseID, so a row in the
// ledger forensics section names the same identifier the operator just
// read in the bucket list.
func renderDriftStateBuckets(prog *progress.Progress) string {
	if prog == nil {
		return ""
	}
	var porting, converged, owned []string
	for phaseID, phase := range prog.Phases {
		for subID, sub := range phase.Subphases {
			id := phaseID + "." + subID
			switch driftStatusOrPorting(sub) {
			case "converged":
				converged = append(converged, id)
			case "owned":
				owned = append(owned, id)
			default:
				porting = append(porting, id)
			}
		}
	}
	sort.Strings(porting)
	sort.Strings(converged)
	sort.Strings(owned)

	var b strings.Builder
	b.WriteString("\nDrift state by subphase:\n")
	fmt.Fprintf(&b, "  PORTING (%d): %s\n", len(porting), strings.Join(porting, ", "))
	fmt.Fprintf(&b, "  CONVERGED (%d): %s\n", len(converged), strings.Join(converged, ", "))
	fmt.Fprintf(&b, "  OWNED (%d): %s\n", len(owned), strings.Join(owned, ", "))
	return b.String()
}

// renderRecentDriftPromotions extracts every DriftPromotion across the
// supplied ledger window and renders one line per promotion. Returns the
// empty string (no header) when there are no promotions, so the section
// disappears entirely on quiet weeks. Window days is plumbed in for the
// header text only — the caller is responsible for windowing the events.
func renderRecentDriftPromotions(events []LedgerEvent, windowDays int) string {
	var lines []string
	for _, ev := range events {
		for _, p := range ev.DriftPromotions {
			lines = append(lines, fmt.Sprintf("  - %s: %s → %s (%s, run %s)",
				p.SubphaseID, p.From, p.To, ev.TS, ev.RunID))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("\nRecent drift promotions (last %dd):\n%s\n", windowDays, strings.Join(lines, "\n"))
}

// loadProgressForStatus is a thin wrapper around progress.Load that returns
// (nil, nil) for missing files so the drift bucket section soft-fails to
// empty on a fresh checkout, matching every other section in this file.
func loadProgressForStatus(path string) (*progress.Progress, error) {
	if path == "" {
		return nil, nil
	}
	prog, err := progress.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return prog, nil
}

// SuggestedActionForCategory maps an autoloop FailureCategory string to the
// operator action recommended in the L5 status surface. Centralised so the
// planner status command, future dashboards, and tests share one definition.
//
// Categories not in the closed FailureCategory set (and the empty string)
// fall through to "manual review" — a deliberate soft default rather than a
// panic so unknown / new categories surface without crashing the renderer.
func SuggestedActionForCategory(category string) string {
	switch strings.TrimSpace(category) {
	case string(progress.FailureReportValidation):
		return "split into smaller rows or set contract_status=\"draft\""
	case string(progress.FailureWorkerError), string(progress.FailureBackendDegraded):
		return "investigate infrastructure (backend or worktree state)"
	case string(progress.FailureProgressSummary):
		return "manual contract review — autoloop preflight is failing"
	case string(progress.FailureTimeout):
		return "split into smaller rows; the work is too large for the worker budget"
	default:
		return "manual review"
	}
}

// needsHumanRow is the per-row tuple loadNeedsHumanRows yields. It carries
// the verdict (always non-nil; we only collect rows whose NeedsHuman is true)
// plus the row's Health block (nil-safe; rows can be NeedsHuman without ever
// failing in autoloop, e.g. a manually-stamped escalation).
type needsHumanRow struct {
	PhaseID    string
	SubphaseID string
	ItemName   string
	Verdict    *progress.PlannerVerdict
	Health     *progress.RowHealth
}

func loadNeedsHumanRows(path string) ([]needsHumanRow, error) {
	if path == "" {
		return nil, nil
	}
	prog, err := progress.Load(path)
	if err != nil {
		return nil, err
	}
	var out []needsHumanRow
	phaseIDs := make([]string, 0, len(prog.Phases))
	for id := range prog.Phases {
		phaseIDs = append(phaseIDs, id)
	}
	sort.Strings(phaseIDs)
	for _, phaseID := range phaseIDs {
		phase := prog.Phases[phaseID]
		subIDs := make([]string, 0, len(phase.Subphases))
		for id := range phase.Subphases {
			subIDs = append(subIDs, id)
		}
		sort.Strings(subIDs)
		for _, subID := range subIDs {
			sub := phase.Subphases[subID]
			for i := range sub.Items {
				item := &sub.Items[i]
				if item.PlannerVerdict == nil || !item.PlannerVerdict.NeedsHuman {
					continue
				}
				out = append(out, needsHumanRow{
					PhaseID:    phaseID,
					SubphaseID: subID,
					ItemName:   item.Name,
					Verdict:    item.PlannerVerdict,
					Health:     item.Health,
				})
			}
		}
	}
	return out, nil
}

// bucketOutcomes counts ReshapeOutcomes by their Outcome field. Returns a
// pre-keyed map so callers can index without nil-checks; unknown outcome
// values fall through silently (intentional — a corrupt outcome shouldn't
// inflate any of the three known buckets).
func bucketOutcomes(outcomes []ReshapeOutcome) map[string]int {
	buckets := map[string]int{
		"unstuck":         0,
		"still_failing":   0,
		"no_attempts_yet": 0,
	}
	for _, o := range outcomes {
		if _, ok := buckets[o.Outcome]; ok {
			buckets[o.Outcome]++
		}
	}
	return buckets
}

// loadStateFile reads planner_state.json into a generic map. The status
// surface only needs string fields, so untyped decoding keeps the renderer
// resilient to schema drift (new fields don't require code changes here).
func loadStateFile(path string) (map[string]any, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func stateFieldOrUnknown(state map[string]any, key string) string {
	if state == nil {
		return "unknown"
	}
	if value, ok := state[key].(string); ok && value != "" {
		return value
	}
	return "unknown"
}

func valueOrDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
