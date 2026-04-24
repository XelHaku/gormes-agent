# Orchestrator Final Polish — Design

**Status:** Draft
**Date:** 2026-04-24
**Supersedes:** none — this is the commit-freeze capstone.

## Why this exists

Today's session shipped Oil Release 1 (8 fixes) + hardening bundle (5 fixes) + poison-pill + claudeu shim + regex fix. Autoloop is working: 6 commits landed. But audit shows 11% productivity, unverified code quality, and a constant drip of self-inflicted orchestrator bugs that keep stealing attention from gormes itself.

This spec is the **final** orchestrator change. After it lands:
- **Review gate** — promoted commits open PRs, not straight-to-integration. You skim daily, approve, merge.
- **Tighter task scope** — worker prompt forces claude to define and self-verify per-task acceptance criteria.
- **Commit freeze** — `FROZEN.md` declares no more architectural changes except (a) production incident fixes or (b) explicitly-requested features.
- **Cost tracking** — audit CSV gains per-cycle token/cost estimates.
- **Daily digest** — one command prints yesterday's landed PRs, failures, and cost.

## Change 1 — PR-based promotion gate

Replace `git cherry-pick -Xtheirs` in `promote_successful_workers` with:
```
git push origin <worker_branch>
gh pr create --head <worker_branch> --base main \
  --title "autoloop: <slug>" \
  --body "$pr_body" \
  --label autoloop-bot
```

- Worker branches already have unique names (`codexu/<run_id>/worker<N>`); pushable as-is.
- PR body: includes slug, ledger link, last 40 lines of stderr, final report markdown.
- One PR per successful worker.
- Integration branch continues to exist but is NOT auto-advanced. Manual promotion is still available via `cmd_promote_commit` for emergency.
- Requires `gh` CLI + auth. `validate()` adds `require_cmd gh` and `gh auth status` preflight.
- New env: `PROMOTION_MODE={pr,cherry-pick}` (default `pr` after this release). Setting `cherry-pick` keeps old behavior for rollback.

**Ledger events:** add `worker_pr_opened` with `detail=<slug>@<pr_url>`. Replaces `worker_promoted` in PR mode.

## Change 2 — Self-scoped acceptance criteria

Prepend two new sections to the prompt in `build_prompt`:

1. Right after "Selected task" block:
   ```
   ==================================================
   ACCEPTANCE CRITERIA (YOU MUST DEFINE AND VERIFY)
   ==================================================
   Before you write any test, write a list of 3-5 concrete, observable
   acceptance criteria for this task. Example format:

   - The new struct X has methods Y and Z, documented.
   - Test TestX validates edge case W.
   - progress.json's X entry has status complete with a note pointing at the
     new symbol/file.

   At the END of your final report, include a new section "9) Acceptance check"
   that lists each criterion with PASS/FAIL and a one-line rationale for any FAIL.

   If any criterion fails, include why and what you tried; do NOT claim the
   task done.
   ```

2. Extend `collect_final_report_issues` to validate that section 9 exists AND that every criterion line is labeled `PASS` (case-insensitive). Any `FAIL` rejects promotion. Backward-compat: reports without section 9 continue to validate as they do today (section 9 is technically optional from the orchestrator's perspective; if missing, no accept-criteria gate). Compatible with the existing optional section 9 Runtime flags by using distinct label prefix `Criterion:` on each line.

Actually cleaner: section 9 = "Acceptance check" (always present), section 10 = "Runtime flags" (optional; Task 4 of Oil Release 1 established this pattern). Update section_titles list to include index 9 as "Acceptance check" and make it required.

## Change 3 — Commit-freeze doc

Create `gormes/scripts/orchestrator/FROZEN.md` with:
- Current commit hash (frozen baseline)
- What's frozen (all files under `orchestrator/lib/` and `gormes-auto-codexu-orchestrator.sh`)
- Exceptions allowed: production-incident hotfixes, user-requested features with an issue number
- Review process for exceptions

Not code-enforced, just a norm — but makes subsequent "while I'm in here, let me also..." reflex visible.

## Change 4 — Cost tracking in audit

`audit.sh` already aggregates per-cycle counts. Add two columns to `report.csv`:
- `tokens_estimated` — sum of `usage.input_tokens + usage.output_tokens` from each worker's `.jsonl` file in the window
- `dollars_estimated` — tokens × $3/M input + $15/M output (claude Sonnet 4.5 rough)

`worker-jsonl-path` can be resolved by matching the slug pattern `<slug>__worker<N>__*.jsonl` in `$LOGS_DIR`.

Tolerant of missing jsonl (older codex runs, shim invocations): report 0.

## Change 5 — Daily digest command

New script `gormes/scripts/orchestrator/daily-digest.sh`:
- Reads `runs.jsonl` for the last 24h.
- Emits plain-text summary:
  - PRs opened (pulled via `gh pr list --label autoloop-bot --state open --created "since yesterday"`)
  - PRs merged
  - Failure counts by status
  - Cost estimate from CSV
  - Top 3 poisoned tasks (most failures)
- Writes to stdout. Optional `--output /tmp/digest.md` writes to file for email pipelines.
- User can alias + cron this or just run ad-hoc.

## Out of scope

- Migrating the integration branch away. Keep it as an emergency surface.
- Auto-merge of PRs. Manual only this release.
- Dashboard/web UI.
- Multi-repo support.

## Acceptance

After release:
- `promote_successful_workers` with `PROMOTION_MODE=pr` (default) opens a PR per worker_success.
- Integration branch advances only via manual `promote-commit` subcommand.
- Every worker's final report contains a passing "Acceptance check" section or fails validation.
- `FROZEN.md` exists and is referenced in `gormes/scripts/orchestrator/README.md`.
- `report.csv` gains two cost columns.
- `daily-digest.sh` runs end-to-end and produces sensible output.

Then: **stop**. No more orchestrator work unless an incident or new feature request.
