package architectureplanner

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plannertriggers"
)

// healthPreservationClause is appended to every planner prompt as a HARD
// rule. The autoloop runtime owns RowHealth metadata; the planner must
// reproduce it verbatim for any row it keeps. Dropping or reformatting any
// field inside `health` causes RunOnce to reject the regeneration via
// validateHealthPreservation.
const healthPreservationClause = `
HEALTH BLOCK PRESERVATION (HARD RULE)
Every progress.json item may carry a ` + "`health`" + ` block (RowHealth). This block
is OWNED by the autoloop runtime — you must reproduce it verbatim in your
output for any row you keep. Do not modify, omit, or reformat any field
inside ` + "`health`" + `. If you delete a row, the health block dies with it (that
is expected). If you split a row into multiple new rows, the original
health block is dropped (the split is a new contract; quarantine resets
naturally via spec-hash detection).
`

// quarantinePriorityClause is appended to every planner prompt as a SOFT
// rule. It instructs the planner to materially change quarantined rows
// (sharpen, split, or mark for human review) so autoloop's auto-clear path
// (spec-hash mismatch) actually triggers on the next run.
const quarantinePriorityClause = `
QUARANTINE PRIORITY (SOFT RULE)
Rows in quarantined_rows[] are top priority for repair. For each one:
  - Read its last_category and last_failure_excerpt
  - Examine its contract and acceptance
  - Decide ONE of:
    (a) Sharpen the contract — make done_signal more concrete, add an
        explicit fixture path, narrow write_scope
    (b) Split the row — if it's an umbrella that workers can't complete
        atomically, split into 2-3 smaller rows with explicit dependencies
    (c) Mark it for human review — if the failure is infrastructural
        (category=worker_error or backend_degraded with no diff), set
        contract_status: "draft" and add a note in degraded_mode
        explaining what's needed
  Whatever you choose, the row's contract/contract_status/blocked_by/
  write_scope/fixture must change in some material way. Otherwise
  quarantine will not auto-clear and autoloop will keep skipping the row.
`

// selfEvaluationClause is appended to every planner prompt as a SOFT rule.
// It tells the planner how to read the "Previous Reshape Outcomes" data
// section that follows: UNSTUCK rows confirm a working approach, STILL
// FAILING rows have resisted reshape and likely need a different
// decomposition or escalation, NO ATTEMPTS YET rows are inconclusive.
// The clause is unconditional (matches the HARD/SOFT clause pattern); the
// data section appears only when bundle.PreviousReshapes is non-empty.
const selfEvaluationClause = `
SELF-EVALUATION (SOFT RULE)

The "Previous Reshape Outcomes" section reports what autoloop did with rows
you reshaped in past runs. Use this signal:
  - UNSTUCK rows confirm your previous approach worked
  - STILL FAILING rows have resisted reshape — try a different decomposition,
    escalate to "needs_human" via PlannerVerdict (L5), or tighten ready_when
  - NO ATTEMPTS YET rows may be legitimately blocked
`

// topicalClauseTemplate is appended to the planner prompt when the run was
// invoked with positional keyword arguments (L6 topical focus mode). The
// upstream context (Quarantined Rows, Previous Reshapes, Implementation
// Inventory) has already been narrowed by FilterContextByKeywords; this
// clause merely tells the LLM what just happened and what scope to honor.
const topicalClauseTemplate = `
TOPICAL FOCUS

This run was invoked with keyword arguments: %s. The context above
(Quarantined Rows, Previous Reshapes, Implementation Inventory) has been
narrowed to only rows that mechanically match these keywords.

Focus your refinement work on these areas. You may still adjust adjacent
rows if a topical row's blocked_by/unblocks dependencies require it, but
do NOT widen the scope to unrelated phases. If you believe a topical
keyword needs structural rework that crosses phase boundaries, set
contract_status="draft" on the affected rows and add a degraded_mode note
explaining the cross-phase dependency rather than reshaping the whole
graph.
`

// formatTopicalClause renders the topical clause with each keyword
// surrounded by Go-quoted double quotes (so "skills" appears as `"skills"`
// in the prompt — easier to scan than raw words).
func formatTopicalClause(keywords []string) string {
	quoted := make([]string, len(keywords))
	for i, kw := range keywords {
		quoted[i] = strconv.Quote(kw)
	}
	return fmt.Sprintf(topicalClauseTemplate, "["+strings.Join(quoted, ", ")+"]")
}

func BuildPrompt(bundle ContextBundle, keywords []string) string {
	var roots []string
	for _, root := range bundle.SourceRoots {
		status := "missing"
		if root.Exists {
			status = fmt.Sprintf("present, files=%d", root.FileCount)
		}
		roots = append(roots, fmt.Sprintf("- %s: %s (%s)", root.Name, root.Path, status))
	}
	var syncLines []string
	if len(bundle.SyncResults) == 0 {
		syncLines = append(syncLines, "- no sync actions recorded for this run")
	}
	for _, result := range bundle.SyncResults {
		line := fmt.Sprintf("- %s: %s %s", result.Name, result.Action, result.Path)
		if result.Output != "" {
			line += " | " + result.Output
		}
		syncLines = append(syncLines, line)
	}
	landingSite := formatInventorySurface(bundle.ImplementationInventory.LandingSite)
	hugoDocs := formatInventorySurface(bundle.ImplementationInventory.HugoDocs)
	auditBlock := formatAutoloopAudit(bundle.AutoloopAudit)
	quarantineBlock := formatQuarantinedRows(bundle.QuarantinedRows)
	reshapeBlock := formatPreviousReshapes(bundle.PreviousReshapes)
	triggerBlock := formatTriggerEvents(bundle.TriggerEvents)
	topicalBlock := ""
	if len(keywords) > 0 {
		topicalBlock = formatTopicalClause(keywords)
	}

	return fmt.Sprintf(`You are the Gormes Architecture Planner Loop.

Mission:
Improve the architecture plan for building full Gormes, the Go port of Hermes, while preserving the internal goncho package direction for Honcho-compatible memory. You also own the www.gormes.ai landing page and the Hugo docs webpage.

Long-term operating contract:
- You are the only long-term prompt agent responsible for architecture planning from now on.
- Every run must detect upstream Hermes, Honcho, and GBrain changes from the synchronized sibling repos.
- Synchronize progress.json with the current Gormes implementation and the latest upstream behavior.
- Synchronize landing page, Hugo docs, generated pages, and progress.json whenever roadmap or implementation reality changes.
- If upstream moved but Gormes has not, add or refine small TDD-ready progress rows instead of silently accepting drift.
- If Gormes implementation moved but progress.json is stale, update progress status, notes, acceptance, and source_refs.
- If www.gormes.ai or docs/hugo.toml drift from the roadmap, add planner tasks or docs edits that bring the public surfaces back in line.
- Autoloop workers should not have to search or guess; every executable row must carry enough concrete context to start TDD immediately.

Planning scope:
%s

Sync results:
%s

Current Gormes implementation inventory:
- commands: %s
- internal packages: %s
- building-gormes docs: %s
- landing page: %s
- Hugo docs: %s

Autoloop audit (last 7 days):
%s

Control plane:
- Canonical progress file: %s
- Target repo: %s
- Current progress items: %d

Required behavior:
1. Study hermes-agent, gbrain, docs/content/upstream-hermes, docs/content/upstream-gbrain, docs/content/building-gormes, www.gormes.ai, docs/hugo.toml, and Honcho/Goncho memory references.
2. Improve docs/content/building-gormes/architecture_plan/progress.json conservatively so autoloop workers receive smaller, dependency-aware, TDD-ready slices.
3. Keep GONCHO as the internal implementation name while preserving honcho_* external compatibility where the public tool contract requires it.
4. Include Goncho/Honcho tasks when they affect the full Gormes architecture.
5. Compare synchronized upstream repos against current Gormes implementation inventory before changing any roadmap row.
6. Synchronize progress.json with the current Gormes implementation; do not let docs, generated pages, web surfaces, and source drift apart.
7. Synchronize the www.gormes.ai landing page and Hugo docs when public messaging, installation flows, architecture milestones, or progress totals change.
8. For every new or refined executable row, include concrete source_refs, write_scope, test_commands, acceptance, ready_when, not_ready_when, and done_signal fields wherever the schema allows them.
9. Prefer exact file paths, function/type names, upstream commits, fixture names, dependency ordering, and validation commands over generic notes.
10. Split broad goals into small rows with explicit blocked_by/unblocks relationships so autoloop workers can pick the next safe slice without rediscovering architecture.
11. Do not implement runtime feature code; planning/docs/progress/web content work only.
12. Do not mark implementation complete without concrete repository evidence.

After edits, run:
- go run ./cmd/autoloop progress write
- go run ./cmd/autoloop progress validate
- go test ./internal/progress -count=1
- go test ./docs -count=1
- (cd www.gormes.ai && go test ./... -count=1)

Required final report sections:
1. Scope scanned
2. Architecture deltas found
3. Progress plan changes
4. Goncho/Honcho implications
5. Validation evidence
6. Recommended next autoloop tasks
7. Autoloop handoff completeness
8. Risks and ambiguities
%s%s%s%s%s%s%s
`, strings.Join(roots, "\n"), strings.Join(syncLines, "\n"), strings.Join(bundle.ImplementationInventory.Commands, ", "), strings.Join(bundle.ImplementationInventory.InternalPackages, ", "), strings.Join(bundle.ImplementationInventory.BuildingDocs, ", "), landingSite, hugoDocs, auditBlock, bundle.ProgressJSON, bundle.RepoRoot, bundle.ProgressStats.Items, healthPreservationClause, quarantinePriorityClause, quarantineBlock, selfEvaluationClause, reshapeBlock, triggerBlock, topicalBlock)
}

// formatTriggerEvents renders the autoloop signals consumed by this run as
// a bullet section. Returns "" when there are no events so the section is
// omitted entirely on scheduled (no-event) runs. Each bullet names the
// row's coordinates plus the kind/reason so the planner can scan and
// route attention without re-reading the trigger ledger.
func formatTriggerEvents(events []plannertriggers.TriggerEvent) string {
	if len(events) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Recent Autoloop Signals (Since Last Planner Run)\n\nThese rows changed state in autoloop and may need attention this run:\n\n")
	for _, ev := range events {
		fmt.Fprintf(&b, "- %s/%s/%s — %s — %s\n", ev.PhaseID, ev.SubphaseID, ev.ItemName, ev.Kind, ev.Reason)
	}
	return b.String()
}

// formatPreviousReshapes renders the L4 self-evaluation surface as a
// bucketed list (UNSTUCK / STILL FAILING / NO ATTEMPTS YET). Returns the
// empty string when there are no outcomes so the section is omitted
// entirely on first runs and on runs where the planner reshaped nothing.
// The SELF-EVALUATION (SOFT RULE) clause still ships unconditionally so
// the LLM knows what the section means even when it is absent.
func formatPreviousReshapes(outcomes []ReshapeOutcome) string {
	if len(outcomes) == 0 {
		return ""
	}
	var unstuck, still, none []ReshapeOutcome
	for _, o := range outcomes {
		switch o.Outcome {
		case "unstuck":
			unstuck = append(unstuck, o)
		case "still_failing":
			still = append(still, o)
		default:
			none = append(none, o)
		}
	}
	var b strings.Builder
	b.WriteString("\n## Previous Reshape Outcomes (Last 7 Days)\n\n")
	if len(unstuck) > 0 {
		fmt.Fprintf(&b, "UNSTUCK (%d):\n", len(unstuck))
		for _, o := range unstuck {
			fmt.Fprintf(&b, "- %s/%s/%s — reshaped %s by %s; autoloop promoted %s\n",
				o.PhaseID, o.SubphaseID, o.ItemName, o.ReshapedAt, o.ReshapedBy, o.LastSuccess)
		}
	}
	if len(still) > 0 {
		fmt.Fprintf(&b, "\nSTILL FAILING (%d):\n", len(still))
		for _, o := range still {
			fmt.Fprintf(&b, "- %s/%s/%s — reshaped %s by %s; autoloop attempted %d times, last category: %s\n",
				o.PhaseID, o.SubphaseID, o.ItemName, o.ReshapedAt, o.ReshapedBy, o.AutoloopRuns, o.LastFailure)
		}
	}
	if len(none) > 0 {
		fmt.Fprintf(&b, "\nNO ATTEMPTS YET (%d):\n", len(none))
		for _, o := range none {
			fmt.Fprintf(&b, "- %s/%s/%s — reshaped %s by %s; autoloop has not selected this row since\n",
				o.PhaseID, o.SubphaseID, o.ItemName, o.ReshapedAt, o.ReshapedBy)
		}
	}
	return b.String()
}

// formatQuarantinedRows renders the planner's call-to-action list for
// quarantined rows. Returns the empty string when there are no rows so the
// section is omitted entirely (the HARD/SOFT rule clauses still ship).
func formatQuarantinedRows(rows []QuarantinedRowContext) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Quarantined Rows (Top Priority for Repair)\n\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "- %s/%s/%s — %d attempts, last category=%s, since=%s\n",
			r.PhaseID, r.SubphaseID, r.ItemName, r.AttemptCount, r.LastCategory, r.QuarantinedSince)
		if r.Contract != "" {
			fmt.Fprintf(&b, "  contract: %s\n", r.Contract)
		}
		if r.LastFailureExcerpt != "" {
			fmt.Fprintf(&b, "  last failure tail: %s\n", r.LastFailureExcerpt)
		}
	}
	return b.String()
}

func formatAutoloopAudit(audit AutoloopAudit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Window: %s -> %s\n", audit.WindowStartUTC, audit.WindowEndUTC)
	fmt.Fprintf(&b, "- Runs: %d  Claimed: %d  Succeeded: %d  Promoted: %d  PromotionFailed: %d  Failed: %d\n",
		audit.Runs, audit.Claimed, audit.Succeeded, audit.Promoted, audit.PromotionFailed, audit.Failed)
	fmt.Fprintf(&b, "- Productivity (promoted/claimed): %d%%\n", audit.ProductivityPercent())

	if len(audit.FailStatusCounts) > 0 {
		statuses := make([]string, 0, len(audit.FailStatusCounts))
		for status := range audit.FailStatusCounts {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)
		var parts []string
		for _, status := range statuses {
			parts = append(parts, fmt.Sprintf("%s=%d", status, audit.FailStatusCounts[status]))
		}
		fmt.Fprintf(&b, "- Fail-status counts: %s\n", strings.Join(parts, ", "))
	}

	if len(audit.ToxicSubphases) > 0 {
		b.WriteString("- Toxic subphases (split or re-spec these rows):\n")
		for _, row := range audit.ToxicSubphases {
			fmt.Fprintf(&b, "  - %s: claimed=%d succeeded=%d promoted=%d failed=%d promotion_failed=%d\n",
				row.SubphaseID, row.Claimed, row.Succeeded, row.Promoted, row.Failed, row.PromotionFailed)
		}
	} else {
		b.WriteString("- Toxic subphases: none in window\n")
	}

	if len(audit.HotSubphases) > 0 {
		b.WriteString("- Hot subphases (most claimed):\n")
		for _, row := range audit.HotSubphases {
			fmt.Fprintf(&b, "  - %s: claimed=%d promoted=%d failed=%d\n",
				row.SubphaseID, row.Claimed, row.Promoted, row.Failed)
		}
	}

	if len(audit.RecentFailedTasks) > 0 {
		b.WriteString("- Recent failed tasks:\n")
		for _, row := range audit.RecentFailedTasks {
			fmt.Fprintf(&b, "  - %s [%s]: %s\n", row.TS, row.Status, row.Task)
		}
	}

	plannerGuidance := strings.Join([]string{
		"",
		"Planner guidance from audit:",
		"- Subphases listed under 'Toxic' are repeatedly failing or stuck in promotion. Split them into smaller, dependency-ordered rows with clearer ready_when, write_scope, and test_commands so a single worker slice can succeed in one shot.",
		"- If 'Hot subphases' show many claims but few promotions, the rows are not landable as written. Either tighten the scope or fix the source_refs/fixture so workers stop colliding.",
		"- If productivity is below 50%, prefer reducing the count of executable rows in those subphases over adding new work elsewhere.",
	}, "\n")
	b.WriteString(plannerGuidance)
	return b.String()
}

func formatInventorySurface(root SourceRoot) string {
	status := "missing"
	if root.Exists {
		status = fmt.Sprintf("present, files=%d", root.FileCount)
	}
	line := fmt.Sprintf("%s (%s)", root.Path, status)
	if len(root.Samples) > 0 {
		line += "; samples: " + strings.Join(root.Samples, ", ")
	}
	return line
}
