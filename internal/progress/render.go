package progress

import (
	"fmt"
	"strings"
)

// statusIcon maps derived status to the glyph shown in markdown tables.
func statusIcon(s Status) string {
	switch s {
	case StatusComplete:
		return "✅"
	case StatusInProgress:
		return "🔨"
	default:
		return "⏳"
	}
}

// RenderReadmeRollup returns the 6-row phase table inserted into the
// README's `## Architecture` section between the PROGRESS markers.
func RenderReadmeRollup(p *Progress) string {
	var b strings.Builder
	b.WriteString("| Phase | Status | Shipped |\n")
	b.WriteString("|-------|--------|---------|\n")
	for _, key := range sortedMapKeys(p.Phases) {
		ph := p.Phases[key]
		total := len(ph.Subphases)
		complete := 0
		for _, sp := range ph.Subphases {
			if sp.DerivedStatus() == StatusComplete {
				complete++
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %d/%d subphases |\n",
			ph.Name, statusIcon(ph.DerivedStatus()), complete, total)
	}
	return b.String()
}

// RenderDocsChecklist returns the full item-level checklist embedded
// in _index.md between the PROGRESS markers. Emits:
//   - an **Overall** stats line
//   - a phase-level table matching the README rollup
//   - a per-subphase section with - [x] / - [ ] checkboxes
func RenderDocsChecklist(p *Progress) string {
	s := p.Stats()
	var b strings.Builder

	fmt.Fprintf(&b, "**Overall:** %d/%d subphases shipped · %d in progress · %d planned\n\n",
		s.Subphases.Complete, s.Subphases.Total, s.Subphases.InProgress, s.Subphases.Planned)

	b.WriteString(RenderReadmeRollup(p))
	b.WriteString("\n---\n\n")

	for _, key := range sortedMapKeys(p.Phases) {
		ph := p.Phases[key]
		fmt.Fprintf(&b, "## %s %s\n\n", ph.Name, statusIcon(ph.DerivedStatus()))
		if ph.Deliverable != "" {
			fmt.Fprintf(&b, "*%s*\n\n", ph.Deliverable)
		}
		for _, spKey := range sortedMapKeys(ph.Subphases) {
			sp := ph.Subphases[spKey]
			fmt.Fprintf(&b, "### %s — %s %s\n\n", spKey, sp.Name, statusIcon(sp.DerivedStatus()))
			if len(sp.Items) == 0 {
				st := string(sp.Status)
				if st == "" {
					st = "unspecified"
				}
				fmt.Fprintf(&b, "*(no item breakdown — tracked at subphase level: %s)*\n\n", st)
				continue
			}
			for _, it := range sp.Items {
				box := "[ ]"
				if it.Status == StatusComplete {
					box = "[x]"
				}
				fmt.Fprintf(&b, "- %s %s\n", box, it.Name)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// RenderContractReadiness returns a markdown table for progress rows that have
// contract metadata. The canonical progress JSON remains the source of truth.
func RenderContractReadiness(p *Progress) string {
	rows := contractRows(p)
	if len(rows) == 0 {
		return "_No progress rows currently carry contract metadata._\n"
	}

	var b strings.Builder
	b.WriteString("| Phase | Progress item | Contract status | Owner | Size | Trust class | Fixture | Degraded mode |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, row := range rows {
		it := row.Item
		fmt.Fprintf(&b, "| %s | %s | `%s` | `%s` | `%s` | %s | `%s` | %s |\n",
			mdCell(row.PhaseKey+" / "+row.SubphaseKey),
			mdCell(it.Name+" — "+it.Contract),
			mdCell(string(it.ContractStatus)),
			mdCell(string(it.ExecutionOwner)),
			mdCell(string(it.SliceSize)),
			mdCell(joinOrDash(it.TrustClass)),
			mdCell(it.Fixture),
			mdCell(it.DegradedMode),
		)
	}
	return b.String()
}

// RenderNextSlices returns the highest-leverage unblocked, non-umbrella
// contract-bearing rows.
func RenderNextSlices(p *Progress, limit int) string {
	if limit <= 0 {
		limit = 10
	}
	rows := nextSliceRows(contractRows(p), limit)
	if len(rows) == 0 {
		return "_No contract-ready progress rows are available._\n"
	}

	var b strings.Builder
	b.WriteString("| Phase | Slice | Contract | Trust class | Fixture | Why now |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, row := range rows {
		it := row.Item
		fmt.Fprintf(&b, "| %s | %s | %s | %s | `%s` | %s |\n",
			mdCell(row.PhaseKey+" / "+row.SubphaseKey),
			mdCell(it.Name),
			mdCell(it.Contract),
			mdCell(joinOrDash(it.TrustClass)),
			mdCell(it.Fixture),
			mdCell(whyNow(it)),
		)
	}
	return b.String()
}

// RenderAgentQueue returns execution cards for unblocked, non-umbrella rows
// that an autonomous worker can turn into a focused implementation attempt.
func RenderAgentQueue(p *Progress, limit int) string {
	if limit <= 0 {
		limit = 10
	}
	rows := nextSliceRows(contractRows(p), limit)
	if len(rows) == 0 {
		return "_No unblocked contract rows are ready for autonomous execution._\n"
	}

	var b strings.Builder
	for i, row := range rows {
		it := row.Item
		fmt.Fprintf(&b, "## %d. %s\n\n", i+1, it.Name)
		fmt.Fprintf(&b, "- Phase: %s / %s\n", row.PhaseKey, row.SubphaseKey)
		fmt.Fprintf(&b, "- Owner: `%s`\n", it.ExecutionOwner)
		fmt.Fprintf(&b, "- Size: `%s`\n", it.SliceSize)
		fmt.Fprintf(&b, "- Status: `%s`\n", it.Status)
		if it.Priority != "" {
			fmt.Fprintf(&b, "- Priority: `%s`\n", it.Priority)
		}
		fmt.Fprintf(&b, "- Contract: %s\n", mdCell(it.Contract))
		fmt.Fprintf(&b, "- Trust class: %s\n", mdCell(joinOrDash(it.TrustClass)))
		fmt.Fprintf(&b, "- Ready when: %s\n", mdCell(joinOrDash(it.ReadyWhen)))
		fmt.Fprintf(&b, "- Not ready when: %s\n", mdCell(joinOrDash(it.NotReadyWhen)))
		fmt.Fprintf(&b, "- Degraded mode: %s\n", mdCell(it.DegradedMode))
		fmt.Fprintf(&b, "- Fixture: `%s`\n", mdCell(it.Fixture))
		fmt.Fprintf(&b, "- Write scope: %s\n", mdCell(joinCodeOrDash(it.WriteScope)))
		fmt.Fprintf(&b, "- Test commands: %s\n", mdCell(joinCodeOrDash(it.TestCommands)))
		fmt.Fprintf(&b, "- Done signal: %s\n", mdCell(joinOrDash(it.DoneSignal)))
		fmt.Fprintf(&b, "- Acceptance: %s\n", mdCell(joinOrDash(it.Acceptance)))
		fmt.Fprintf(&b, "- Source refs: %s\n", mdCell(joinOrDash(it.SourceRefs)))
		if len(it.Unblocks) > 0 {
			fmt.Fprintf(&b, "- Unblocks: %s\n", mdCell(joinOrDash(it.Unblocks)))
		}
		fmt.Fprintf(&b, "- Why now: %s\n\n", mdCell(whyNow(it)))
	}
	return b.String()
}

// RenderBlockedSlices returns rows that cannot start until another roadmap row
// is complete or another readiness condition becomes true.
func RenderBlockedSlices(p *Progress) string {
	rows := blockedRows(contractRows(p))
	if len(rows) == 0 {
		return "_No contract-bearing rows are currently blocked._\n"
	}

	var b strings.Builder
	b.WriteString("| Phase | Slice | Blocked by | Ready when | Unblocks |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, row := range rows {
		it := row.Item
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			mdCell(row.PhaseKey+" / "+row.SubphaseKey),
			mdCell(it.Name),
			mdCell(joinOrDash(it.BlockedBy)),
			mdCell(joinOrDash(it.ReadyWhen)),
			mdCell(joinOrDash(it.Unblocks)),
		)
	}
	return b.String()
}

// RenderUmbrellaCleanup returns planned rows that are inventory buckets rather
// than executable implementation slices.
func RenderUmbrellaCleanup(p *Progress) string {
	rows := umbrellaRows(p)
	if len(rows) == 0 {
		return "_No umbrella rows are currently marked for cleanup._\n"
	}

	var b strings.Builder
	b.WriteString("| Phase | Umbrella row | Owner | Not ready when | Split into |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, row := range rows {
		it := row.Item
		fmt.Fprintf(&b, "| %s | %s | `%s` | %s | %s |\n",
			mdCell(row.PhaseKey+" / "+row.SubphaseKey),
			mdCell(it.Name),
			mdCell(string(it.ExecutionOwner)),
			mdCell(joinOrDash(it.NotReadyWhen)),
			mdCell(joinOrDash(it.Unblocks)),
		)
	}
	return b.String()
}

// RenderAutoloopHandoff returns the control-plane facts used by the unattended
// worker loop. These live in progress.json meta so docs and prompts do not
// drift from each other.
func RenderAutoloopHandoff(p *Progress) string {
	m := p.Meta.Autoloop
	if !autoloopMetaDeclared(m) {
		return "_No autoloop metadata declared in canonical progress._\n"
	}

	var b strings.Builder
	b.WriteString("## Control Plane\n\n")
	fmt.Fprintf(&b, "- Entrypoint: `%s`\n", mdCell(m.Entrypoint))
	fmt.Fprintf(&b, "- Plan: `%s`\n", mdCell(m.Plan))
	fmt.Fprintf(&b, "- Candidate source: `%s`\n", mdCell(m.CandidateSource))
	fmt.Fprintf(&b, "- Agent queue: `%s`\n", mdCell(m.AgentQueue))
	fmt.Fprintf(&b, "- Progress schema: `%s`\n", mdCell(m.ProgressSchema))
	fmt.Fprintf(&b, "- Unit tests: `%s`\n", mdCell(m.UnitTest))

	b.WriteString("\n## Candidate Policy\n\n")
	if len(m.CandidatePolicy) == 0 {
		b.WriteString("- (not declared)\n")
	} else {
		for _, policy := range m.CandidatePolicy {
			fmt.Fprintf(&b, "- %s\n", mdCell(policy))
		}
	}
	return b.String()
}

// RenderProgressSchema returns the operator-facing schema reference for
// contract-aware progress rows.
func RenderProgressSchema() string {
	return strings.TrimSpace(`
## Item Fields

| Field | Required when | Meaning |
|---|---|---|
| `+"`name`"+` | every item | Human-readable roadmap row name. |
| `+"`status`"+` | every item | `+"`planned`"+`, `+"`in_progress`"+`, or `+"`complete`"+`. |
| `+"`priority`"+` | optional | `+"`P0`"+` through `+"`P4`"+`. Item-level `+"`P0`"+` rows require contract metadata. |
| `+"`contract`"+` | active/P0 handoffs | The upstream behavior or Gormes-native behavior being preserved. |
| `+"`contract_status`"+` | contract rows | `+"`missing`"+`, `+"`draft`"+`, `+"`fixture_ready`"+`, or `+"`validated`"+`. |
| `+"`slice_size`"+` | contract rows and umbrella rows | `+"`small`"+`, `+"`medium`"+`, `+"`large`"+`, or `+"`umbrella`"+`. |
| `+"`execution_owner`"+` | contract rows and umbrella rows | `+"`docs`"+`, `+"`gateway`"+`, `+"`memory`"+`, `+"`provider`"+`, `+"`tools`"+`, `+"`skills`"+`, or `+"`orchestrator`"+`. |
| `+"`trust_class`"+` | active/P0 handoffs | Allowed caller classes: `+"`operator`"+`, `+"`gateway`"+`, `+"`child-agent`"+`, `+"`system`"+`. |
| `+"`degraded_mode`"+` | active/P0 handoffs | How partial capability is visible in doctor, status, audit, logs, or generated docs. |
| `+"`fixture`"+` | active/P0 handoffs | Local package/path/fixture set proving compatibility without live credentials. |
| `+"`source_refs`"+` | active/P0 handoffs | Docs or code references used to derive the contract. |
| `+"`blocked_by`"+` | optional | Roadmap rows or conditions blocking this slice. Requires `+"`ready_when`"+`. |
| `+"`unblocks`"+` | optional | Downstream rows enabled by this slice. |
| `+"`ready_when`"+` | contract rows and blocked rows | Concrete condition that makes the row assignable. |
| `+"`not_ready_when`"+` | umbrella rows, optional elsewhere | Conditions that make the row unsafe or too broad to assign. |
| `+"`acceptance`"+` | active/P0 handoffs | Testable done criteria. |
| `+"`write_scope`"+` | contract rows | Files, directories, or packages an autonomous agent may edit for this slice. |
| `+"`test_commands`"+` | contract rows | Commands that prove the slice without live provider or platform credentials. |
| `+"`done_signal`"+` | contract rows | Observable evidence that the row can move forward or close. |

## Meta Fields

| Field | Required when | Meaning |
|---|---|---|
| `+"`meta.autoloop.entrypoint`"+` | autoloop metadata is declared | Main unattended-loop script. |
| `+"`meta.autoloop.plan`"+` | autoloop metadata is declared | Canonical implementation plan for improving the orchestrator. |
| `+"`meta.autoloop.agent_queue`"+` | autoloop metadata is declared | Generated queue page for assignable rows. |
| `+"`meta.autoloop.progress_schema`"+` | autoloop metadata is declared | This schema reference. |
| `+"`meta.autoloop.candidate_source`"+` | autoloop metadata is declared | Canonical progress file consumed by the loop. |
| `+"`meta.autoloop.unit_test`"+` | autoloop metadata is declared | Fast verification command for orchestrator prompt/candidate behavior. |
| `+"`meta.autoloop.candidate_policy`"+` | autoloop metadata is declared | Shared selection rules injected into worker prompts. |

## Validation Rules

- `+"`docs/data/progress.json`"+` must not exist.
- if `+"`meta.autoloop`"+` is declared, entrypoint, plan, candidate source, generated docs, unit test, and candidate policy must all be present.
- `+"`in_progress`"+` rows cannot use `+"`slice_size: umbrella`"+`.
- item-level `+"`P0`"+` and `+"`in_progress`"+` rows must include full contract metadata.
- contract rows must declare `+"`slice_size`"+`, `+"`execution_owner`"+`, `+"`ready_when`"+`, `+"`write_scope`"+`, `+"`test_commands`"+`, and `+"`done_signal`"+`.
- blocked rows must declare `+"`ready_when`"+`.
- `+"`fixture_ready`"+` rows must name a concrete fixture package or path.
- complete rows with contract metadata must use `+"`contract_status: validated`"+`.

## Generated Agent Surfaces

- `+"`docs/content/building-gormes/autoloop/autoloop-handoff.md`"+` lists shared unattended-loop entrypoint, plan, candidate source, generated docs, test command, and candidate policy.
- `+"`docs/content/building-gormes/autoloop/agent-queue.md`"+` lists only unblocked, non-umbrella contract rows with owner, size, readiness, degraded mode, fixture, write scope, test commands, done signal, acceptance, and source references.
- `+"`docs/content/building-gormes/autoloop/blocked-slices.md`"+` keeps blocked rows out of the execution queue while preserving their unblock condition.
- `+"`docs/content/building-gormes/autoloop/umbrella-cleanup.md`"+` lists broad inventory rows that must be split before assignment.

## Good Row

`+"```json"+`
{
  "name": "Provider transcript harness",
  "status": "planned",
  "priority": "P1",
  "contract": "Provider-neutral request and stream event transcript harness",
  "contract_status": "fixture_ready",
  "slice_size": "medium",
  "execution_owner": "provider",
  "trust_class": ["system"],
  "degraded_mode": "Provider status reports missing fixture coverage before routing can select the adapter.",
  "fixture": "internal/hermes/testdata/provider_transcripts",
  "source_refs": ["docs/content/upstream-hermes/source-study.md"],
  "ready_when": ["Anthropic transcript fixtures replay without live credentials."],
  "write_scope": ["internal/hermes/"],
  "test_commands": ["go test ./internal/hermes -count=1"],
  "done_signal": ["Provider transcript replay passes from captured fixtures."],
  "acceptance": ["All provider transcript fixtures pass under go test ./internal/hermes."]
}
`+"```"+`

## Bad Row

`+"```json"+`
{
  "name": "Port CLI",
  "status": "in_progress",
  "slice_size": "umbrella"
}
`+"```"+`

This is invalid because an active execution row cannot be an umbrella, and it
does not explain the contract, fixture, caller trust class, degraded mode, or
acceptance criteria.
`) + "\n"
}

type contractRow struct {
	PhaseKey    string
	PhaseName   string
	SubphaseKey string
	Subphase    string
	Item        Item
}

func contractRows(p *Progress) []contractRow {
	var rows []contractRow
	for _, phKey := range sortedMapKeys(p.Phases) {
		ph := p.Phases[phKey]
		for _, spKey := range sortedMapKeys(ph.Subphases) {
			sp := ph.Subphases[spKey]
			for _, it := range sp.Items {
				if it.Contract == "" {
					continue
				}
				rows = append(rows, contractRow{
					PhaseKey:    phKey,
					PhaseName:   ph.Name,
					SubphaseKey: spKey,
					Subphase:    sp.Name,
					Item:        it,
				})
			}
		}
	}
	return rows
}

func nextSliceRows(rows []contractRow, limit int) []contractRow {
	var out []contractRow
	seen := map[string]struct{}{}
	for bucket := 0; bucket <= 4 && len(out) < limit; bucket++ {
		for _, row := range rows {
			if row.Item.Status == StatusComplete || len(row.Item.BlockedBy) > 0 || row.Item.SliceSize == SliceSizeUmbrella {
				continue
			}
			if nextSliceBucket(row.Item) != bucket {
				continue
			}
			key := row.PhaseKey + "\x00" + row.SubphaseKey + "\x00" + row.Item.Name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, row)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

func nextSliceBucket(it Item) int {
	switch {
	case it.Priority == "P0":
		return 0
	case it.Status == StatusInProgress:
		return 1
	case it.ContractStatus == ContractStatusFixtureReady:
		return 2
	case len(it.Unblocks) > 0:
		return 3
	case it.ContractStatus == ContractStatusDraft:
		return 4
	default:
		return 5
	}
}

func whyNow(it Item) string {
	switch {
	case it.Status == StatusInProgress:
		return "Already active; contract metadata keeps execution bounded."
	case it.Priority == "P0":
		return "P0 handoff; needs contract proof before closeout."
	case len(it.BlockedBy) > 0:
		return "Blocked by " + joinOrDash(it.BlockedBy) + "; keep dependencies visible."
	case len(it.Unblocks) > 0:
		return "Unblocks " + joinOrDash(it.Unblocks) + "."
	default:
		return "Contract metadata is present; ready for a focused spec or fixture slice."
	}
}

func blockedRows(rows []contractRow) []contractRow {
	var out []contractRow
	for _, row := range rows {
		if row.Item.Status != StatusComplete && len(row.Item.BlockedBy) > 0 {
			out = append(out, row)
		}
	}
	return out
}

func umbrellaRows(p *Progress) []contractRow {
	var rows []contractRow
	for _, phKey := range sortedMapKeys(p.Phases) {
		ph := p.Phases[phKey]
		for _, spKey := range sortedMapKeys(ph.Subphases) {
			sp := ph.Subphases[spKey]
			for _, it := range sp.Items {
				if it.SliceSize != SliceSizeUmbrella {
					continue
				}
				rows = append(rows, contractRow{
					PhaseKey:    phKey,
					PhaseName:   ph.Name,
					SubphaseKey: spKey,
					Subphase:    sp.Name,
					Item:        it,
				})
			}
		}
	}
	return rows
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}

func joinCodeOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "`"+value+"`")
	}
	return strings.Join(quoted, ", ")
}

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	if s == "" {
		return "-"
	}
	return s
}
