# Orchestrator Test Harness + Modular Refactor + Companion Seam — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a bats test harness + minimal source-only module split of `gormes-auto-codexu-orchestrator.sh`, plus a new `companions.sh` seam that periodically invokes the planner / doc-improver / landing-page scripts between orchestrator cycles. Zero runtime behavior change except the new (default-conservative) companion seam.

**Architecture:** The 2,034-line entry script stays at `gormes/scripts/gormes-auto-codexu-orchestrator.sh`. Seven libs get sourced from `gormes/scripts/orchestrator/lib/`. Tests live under `gormes/scripts/orchestrator/tests/`, using vendored bats-core + bats-assert + bats-support. Fake `codexu` / `planner` / `doc-improver` / `landingpage` binaries on `PATH` drive integration tests against throwaway git fixtures.

**Tech Stack:** Bash 5.2, `jq`, `git worktree`, GNU `timeout`/`find`/`sed`, `bats-core v1.11.0`, `bats-assert v2.1.0`, `bats-support v0.3.0`.

**Reference spec:** `gormes/docs/superpowers/specs/2026-04-23-orchestrator-test-harness-design.md`

**Baseline commit (do not diverge without coordination):** `90f86103` (spec). Entry script baseline: commit `8c486e59`.

---

## File Structure

**New files:**

```
gormes/scripts/orchestrator/
├── README.md                                         # created in Task 13
└── lib/
    ├── common.sh            # log_*, safe_path_token, require_cmd, classify_worker_failure, show_progress, available_mem_mb
    ├── candidates.sh        # normalize_candidates, write_candidates_file, candidate_count, candidate_at, task_slug
    ├── report.sh            # build_prompt, collect_final_report_issues, verify_final_report, extract_report_*, wait_for_valid_final_report, print_final_report_diagnostics, extract_session_id
    ├── claim.sh             # claim_task, release_task, cleanup_stale_locks
    ├── worktree.sh          # create_worker_worktree, maybe_remove_worker_worktree, enforce_worktree_dir_cap, verify_worker_commit, worker_branch_name, worker_worktree_root, worker_repo_root, branch_worktree_path
    ├── promote.sh           # setup_integration_root, promote_successful_workers, push_integration_branch, cmd_promote_commit, promotion_enabled
    └── companions.sh        # NEW: companion_state_dir, companion_last_ts, companion_cycles_since, should_run_*, run_companion, maybe_run_companions

gormes/scripts/orchestrator/tests/
├── .gitignore               # ignores vendor/, tmp/
├── bootstrap.sh             # downloads vendored bats-core, bats-assert, bats-support
├── run.sh                   # entry that sources bats-helpers and invokes test tiers
├── lib/
│   └── test_env.bash        # shared helpers for unit + integration tests
├── unit/
│   ├── noop.bats
│   ├── common.bats
│   ├── candidates.bats
│   ├── report.bats
│   ├── claim.bats
│   ├── worktree.bats
│   ├── promote.bats
│   └── companions.bats
├── integration/
│   ├── happy-path.bats
│   ├── cherry-pick-conflict.bats
│   ├── resume.bats
│   ├── poison-task-retry.bats      # @skip → Spec A target
│   └── companion-trigger.bats
└── fixtures/
    ├── bin/
    │   ├── fake-codexu
    │   ├── fake-planner
    │   ├── fake-doc-improver
    │   └── fake-landingpage
    ├── progress.fixture.json
    ├── planner_state.fixture.json
    └── reports/
        ├── good.final.md
        ├── bad-missing-section.final.md
        ├── bad-no-commit-hash.final.md
        ├── bad-all-zero-exits.final.md
        ├── bad-no-red-exit.final.md
        ├── bad-missing-branch.final.md
        └── bad-empty.final.md
```

**Modified files:**

- `gormes/scripts/gormes-auto-codexu-orchestrator.sh` — remove extracted function bodies, add `source` block near top, add `maybe_run_companions` call in `main` loop, export `PROMOTED_LAST_CYCLE` from `promote_successful_workers`.
- `gormes/Makefile` — add `orchestrator-test`, `orchestrator-test-all`, `orchestrator-lint` targets.

---

## Conventions used in every task

- Every new `.sh` or `.bats` file starts with `#!/usr/bin/env bash` / `#!/usr/bin/env bats` (where applicable) and uses `set -Eeuo pipefail` at the top of shell scripts.
- Extracted lib files define functions only. No top-level side effects. They may read `$VERBOSE`, `$LOGS_DIR`, etc. but never write to them.
- The entry script retains all env var defaults.
- Every task ends with a commit. All tests must pass before commit.
- Commit messages use conventional format: `refactor(orchestrator): ...` for extractions, `test(orchestrator): ...` for tests, `feat(orchestrator): ...` for the companion seam.
- All commits include the trailer `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.

---

## Task 1: Bootstrap — vendor bats, add runner, first green test

**Files:**
- Create: `gormes/scripts/orchestrator/tests/.gitignore`
- Create: `gormes/scripts/orchestrator/tests/bootstrap.sh`
- Create: `gormes/scripts/orchestrator/tests/run.sh`
- Create: `gormes/scripts/orchestrator/tests/lib/test_env.bash`
- Create: `gormes/scripts/orchestrator/tests/unit/noop.bats`

- [ ] **Step 1.1: Write `.gitignore`**

```
vendor/
tmp/
*.log
```

- [ ] **Step 1.2: Write `bootstrap.sh`**

```bash
#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENDOR_DIR="$SCRIPT_DIR/vendor"

BATS_CORE_VERSION="v1.11.0"
BATS_CORE_SHA256="90fc8fb0ba27c5d8b23d2ea5ce2bd77e9c62cc07c3c9e4d6b847d40bfad6b2a0"
BATS_ASSERT_VERSION="v2.1.0"
BATS_ASSERT_SHA256="98ca3b685f8b8993e48ec057565591c5ea55b1b1a1e9cff82c2e7c5b37d3d0c4"
BATS_SUPPORT_VERSION="v0.3.0"
BATS_SUPPORT_SHA256="7815237aafeb42ddcc1b8c698fc5808026d33317d8701d5ec2396e9634e2918f"

mkdir -p "$VENDOR_DIR"

download_and_verify() {
  local name="$1" url="$2" expected_sha="$3" dest="$4"
  [[ -d "$dest" ]] && { echo "$name already present, skipping"; return 0; }
  if [[ "${BATS_OFFLINE:-0}" == "1" ]]; then
    echo "ERROR: $name not vendored and BATS_OFFLINE=1" >&2
    return 1
  fi
  local tmp
  tmp="$(mktemp)"
  echo "Downloading $name from $url"
  curl -fsSL "$url" -o "$tmp"
  local actual_sha
  actual_sha="$(sha256sum "$tmp" | awk '{print $1}')"
  if [[ "$actual_sha" != "$expected_sha" ]]; then
    echo "ERROR: $name sha256 mismatch (expected $expected_sha, got $actual_sha)" >&2
    rm -f "$tmp"
    return 1
  fi
  mkdir -p "$dest"
  tar -xzf "$tmp" -C "$dest" --strip-components=1
  rm -f "$tmp"
}

download_and_verify "bats-core" \
  "https://github.com/bats-core/bats-core/archive/refs/tags/${BATS_CORE_VERSION}.tar.gz" \
  "$BATS_CORE_SHA256" \
  "$VENDOR_DIR/bats-core"

download_and_verify "bats-assert" \
  "https://github.com/bats-core/bats-assert/archive/refs/tags/${BATS_ASSERT_VERSION}.tar.gz" \
  "$BATS_ASSERT_SHA256" \
  "$VENDOR_DIR/bats-assert"

download_and_verify "bats-support" \
  "https://github.com/bats-core/bats-support/archive/refs/tags/${BATS_SUPPORT_VERSION}.tar.gz" \
  "$BATS_SUPPORT_SHA256" \
  "$VENDOR_DIR/bats-support"

echo "bootstrap complete"
```

**Note on SHA256 values:** the engineer MUST verify these against the actual GitHub release tarballs at implementation time. If mismatched, compute with `curl -fsSL <url> | sha256sum` and update this file. Do not silently accept a different value.

- [ ] **Step 1.3: Write `run.sh`**

```bash
#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BATS="$SCRIPT_DIR/vendor/bats-core/bin/bats"

if [[ ! -x "$BATS" ]]; then
  bash "$SCRIPT_DIR/bootstrap.sh"
fi

tiers=("$@")
[[ ${#tiers[@]} -eq 0 ]] && tiers=(unit)

declare -a bats_paths=()
for tier in "${tiers[@]}"; do
  case "$tier" in
    unit|integration)
      bats_paths+=("$SCRIPT_DIR/$tier")
      ;;
    *)
      echo "ERROR: unknown tier '$tier'" >&2
      exit 1
      ;;
  esac
done

exec "$BATS" --print-output-on-failure --timing "${bats_paths[@]}"
```

- [ ] **Step 1.4: Write `lib/test_env.bash`**

```bash
#!/usr/bin/env bash
# Shared helpers available to all bats files.
# Source this via `load '../lib/test_env'` inside bats setup().

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ORCHESTRATOR_SCRIPTS_DIR="$(cd "$TESTS_DIR/../.." && pwd)"
ORCHESTRATOR_LIB_DIR="$ORCHESTRATOR_SCRIPTS_DIR/orchestrator/lib"
ENTRY_SCRIPT="$ORCHESTRATOR_SCRIPTS_DIR/gormes-auto-codexu-orchestrator.sh"
FIXTURES_DIR="$TESTS_DIR/fixtures"

load_helpers() {
  load "$TESTS_DIR/vendor/bats-support/load"
  load "$TESTS_DIR/vendor/bats-assert/load"
}

mktmp_workspace() {
  local base="${BATS_TEST_TMPDIR:-$(mktemp -d)}"
  local dir
  dir="$(mktemp -d "$base/ws.XXXXXX")"
  echo "$dir"
}

source_lib() {
  local name="$1"
  # shellcheck disable=SC1090
  source "$ORCHESTRATOR_LIB_DIR/${name}.sh"
}

export -f load_helpers mktmp_workspace source_lib
```

- [ ] **Step 1.5: Write `unit/noop.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
}

@test "harness runs" {
  run true
  assert_success
}
```

- [ ] **Step 1.6: Make scripts executable**

Run: `chmod +x gormes/scripts/orchestrator/tests/{bootstrap.sh,run.sh}`

- [ ] **Step 1.7: Bootstrap + run**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: prints `1..1` and `ok 1 harness runs`, exit code 0. First run downloads vendor; subsequent runs use cache.

- [ ] **Step 1.8: Commit**

```bash
git add gormes/scripts/orchestrator/tests/
git commit -m "test(orchestrator): bootstrap bats harness with pinned vendor

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Extract `lib/common.sh`

**Files:**
- Create: `gormes/scripts/orchestrator/lib/common.sh`
- Create: `gormes/scripts/orchestrator/tests/unit/common.bats`
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh`

**Functions to extract** (current entry-script line ranges):
- `show_progress` (lines 40-56)
- `log_info`, `log_debug`, `log_warn`, `log_error` (lines 19-37)
- `safe_path_token` (lines 230-233)
- `require_cmd` (lines 220-225)
- `classify_worker_failure` (lines 843-852)
- `available_mem_mb` (lines 505-507)

- [ ] **Step 2.1: Create `lib/common.sh`**

Copy the exact function bodies from the entry script into:

```bash
#!/usr/bin/env bash
# Common logging, path, and small utility helpers for the orchestrator.
# Sourced by gormes-auto-codexu-orchestrator.sh and its tests.
# Depends on: $VERBOSE (reads; default 0 if unset).

log_info() { ... }       # verbatim from entry script
log_debug() { ... }
log_warn() { ... }
log_error() { ... }
show_progress() { ... }
require_cmd() { ... }
safe_path_token() { ... }
available_mem_mb() { ... }
classify_worker_failure() { ... }
```

Do NOT modify function bodies. Do NOT add `set -e`; libs are sourced, not executed.

- [ ] **Step 2.2: Write `unit/common.bats` (failing first)**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib common
}

@test "classify_worker_failure maps 124 to timeout" {
  run classify_worker_failure 124
  assert_success
  assert_output "timeout"
}

@test "classify_worker_failure maps 137 to killed" {
  run classify_worker_failure 137
  assert_output "killed"
}

@test "classify_worker_failure maps 1 to contract_or_test_failure" {
  run classify_worker_failure 1
  assert_output "contract_or_test_failure"
}

@test "classify_worker_failure maps other to worker_error" {
  run classify_worker_failure 42
  assert_output "worker_error"
}

@test "safe_path_token strips unsafe characters" {
  run safe_path_token "Feat/sub phase: X_Y.Z@v1"
  assert_output "Feat-sub-phase-X_Y.Z-v1"
}

@test "safe_path_token trims leading and trailing dashes" {
  run safe_path_token "///foo///"
  assert_output "foo"
}

@test "require_cmd succeeds for a real command" {
  run require_cmd bash
  assert_success
}

@test "require_cmd fails for a bogus command" {
  run require_cmd bogus_cmd_that_does_not_exist_xyz
  assert_failure
}

@test "available_mem_mb returns a positive integer" {
  run available_mem_mb
  assert_success
  [[ "$output" =~ ^[0-9]+$ ]]
  (( output > 0 ))
}
```

- [ ] **Step 2.3: Run failing test (common.sh doesn't exist yet)**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: fails at `source_lib common` with "No such file or directory".

- [ ] **Step 2.4: Create `lib/common.sh` with bodies copied verbatim** (see Step 2.1)

- [ ] **Step 2.5: Run tests — expect green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: 9 tests pass, 0 fail.

- [ ] **Step 2.6: Modify entry script — source common.sh, delete extracted bodies**

At the top of `gormes/scripts/gormes-auto-codexu-orchestrator.sh`, immediately after the `shopt -s inherit_errexit` line (around line 4), add:

```bash
ORCHESTRATOR_LIB_DIR="${ORCHESTRATOR_LIB_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/orchestrator/lib}"
# shellcheck source=/dev/null
for _lib in common; do
  source "$ORCHESTRATOR_LIB_DIR/${_lib}.sh"
done
unset _lib
```

Then delete the original bodies of:
- `log_info`, `log_debug`, `log_warn`, `log_error` (lines 19-37)
- `show_progress` (lines 40-56)
- `require_cmd` (lines 220-225)
- `safe_path_token` (lines 230-233)
- `available_mem_mb` (lines 505-507)
- `classify_worker_failure` (lines 843-852)

Use the Edit tool with the exact function-body text as `old_string` and empty `new_string`. Verify each deletion by grepping the entry script for the function name — should appear only as callsites, never as a definition.

- [ ] **Step 2.7: Smoke the entry script**

Run: `bash -n gormes/scripts/gormes-auto-codexu-orchestrator.sh && gormes/scripts/gormes-auto-codexu-orchestrator.sh --help | head -5`

Expected: no syntax errors; `--help` prints usage.

- [ ] **Step 2.8: Run unit tests — still green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: 9 + 1 (noop) = 10 tests pass.

- [ ] **Step 2.9: Commit**

```bash
git add gormes/scripts/orchestrator/lib/common.sh \
        gormes/scripts/orchestrator/tests/unit/common.bats \
        gormes/scripts/gormes-auto-codexu-orchestrator.sh
git commit -m "refactor(orchestrator): extract common.sh + unit tests

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Extract `lib/candidates.sh`

**Files:**
- Create: `gormes/scripts/orchestrator/lib/candidates.sh`
- Create: `gormes/scripts/orchestrator/tests/unit/candidates.bats`
- Create: `gormes/scripts/orchestrator/tests/fixtures/progress.fixture.json`
- Create: `gormes/scripts/orchestrator/tests/fixtures/progress.empty.json`
- Create: `gormes/scripts/orchestrator/tests/fixtures/progress.all-complete.json`
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh`

**Functions to extract:**
- `normalize_candidates` (lines ~568-598)
- `write_candidates_file` (lines ~601-603)
- `candidate_count` (lines ~605-607)
- `candidate_at` (lines ~609-612)
- `task_slug` (lines ~614-622)

- [ ] **Step 3.1: Create `fixtures/progress.fixture.json`**

```json
{
  "phases": {
    "1": {
      "name": "Phase One",
      "subphases": {
        "1.A": {
          "name": "Sub A",
          "items": [
            {"name": "Item A1", "status": "complete"},
            {"name": "Item A2", "status": "in_progress"}
          ]
        },
        "1.B": {
          "name": "Sub B",
          "items": [
            {"name": "Item B1", "status": "planned"},
            {"name": "Item B2", "status": "planned"}
          ]
        }
      }
    },
    "2": {
      "name": "Phase Two",
      "subphases": {
        "2.A": {
          "name": "Sub 2A",
          "items": [
            {"name": "Item 2A1", "status": "planned"},
            {"name": "Item 2A2", "status": "in_progress"}
          ]
        }
      }
    }
  }
}
```

- [ ] **Step 3.2: Create `fixtures/progress.empty.json`**

```json
{"phases": {}}
```

- [ ] **Step 3.3: Create `fixtures/progress.all-complete.json`**

Variant of `progress.fixture.json` with every `status` set to `"complete"`.

- [ ] **Step 3.4: Write `unit/candidates.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib candidates
  export PROGRESS_JSON="$FIXTURES_DIR/progress.fixture.json"
}

@test "normalize_candidates drops complete items" {
  export ACTIVE_FIRST=1
  run normalize_candidates
  assert_success
  # Should include 4 non-complete items: A2, B1, B2, 2A1, 2A2 = 5 items (not 6)
  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "5"
  echo "$output" | jq -e '.[].status != "complete"'
}

@test "normalize_candidates orders in_progress before planned when ACTIVE_FIRST=1" {
  export ACTIVE_FIRST=1
  run normalize_candidates
  assert_success
  local first_status
  first_status=$(echo "$output" | jq -r '.[0].status')
  assert_equal "$first_status" "in_progress"
}

@test "normalize_candidates does not prioritize when ACTIVE_FIRST=0" {
  export ACTIVE_FIRST=0
  run normalize_candidates
  assert_success
  # Must not group by status — lexical phase/subphase/item order instead
  local order
  order=$(echo "$output" | jq -r '[.[] | .phase_id + "/" + .subphase_id + "/" + .item_name] | join(",")')
  [[ "$order" == "1/1.A/Item A2,1/1.B/Item B1,1/1.B/Item B2,2/2.A/Item 2A1,2/2.A/Item 2A2" ]]
}

@test "normalize_candidates returns empty array for progress.empty.json" {
  export PROGRESS_JSON="$FIXTURES_DIR/progress.empty.json"
  run normalize_candidates
  assert_success
  assert_output "[]"
}

@test "normalize_candidates returns empty array when all items complete" {
  export PROGRESS_JSON="$FIXTURES_DIR/progress.all-complete.json"
  run normalize_candidates
  assert_success
  assert_output "[]"
}

@test "task_slug lowercases and sanitizes" {
  run task_slug "3" "3.E" "Cross Chat Merge (v2)"
  assert_output "3__3.e__cross-chat-merge-v2"
}

@test "candidate_count reports length from CANDIDATES_FILE" {
  local tmp
  tmp="$(mktmp_workspace)"
  echo '[{"a":1},{"b":2},{"c":3}]' > "$tmp/cands.json"
  export CANDIDATES_FILE="$tmp/cands.json"
  run candidate_count
  assert_output "3"
}

@test "candidate_at returns JSON object at index" {
  local tmp
  tmp="$(mktmp_workspace)"
  echo '[{"k":"a"},{"k":"b"}]' > "$tmp/cands.json"
  export CANDIDATES_FILE="$tmp/cands.json"
  run candidate_at 1
  assert_output '{"k":"b"}'
}
```

- [ ] **Step 3.5: Run — expect failure** (candidates.sh missing)

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: fails at `source_lib candidates`.

- [ ] **Step 3.6: Create `lib/candidates.sh`** by copying function bodies verbatim from entry-script lines 568-622 into a file starting with:

```bash
#!/usr/bin/env bash
# Candidate list normalization helpers.
# Depends on: $PROGRESS_JSON, $ACTIVE_FIRST, $CANDIDATES_FILE (reads only).
```

- [ ] **Step 3.7: Run tests — expect green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: all tests (including previous modules) pass. Total should be 18.

- [ ] **Step 3.8: Modify entry script**

Update the source loop to `for _lib in common candidates; do`. Delete the extracted function bodies (lines 568-622 in the pre-refactor file).

- [ ] **Step 3.9: Smoke the entry script**

Run: `bash -n gormes/scripts/gormes-auto-codexu-orchestrator.sh && gormes/scripts/gormes-auto-codexu-orchestrator.sh --help | head -5`

Expected: clean parse, help prints.

- [ ] **Step 3.10: Commit**

```bash
git add gormes/scripts/orchestrator/lib/candidates.sh \
        gormes/scripts/orchestrator/tests/unit/candidates.bats \
        gormes/scripts/orchestrator/tests/fixtures/progress.*.json \
        gormes/scripts/gormes-auto-codexu-orchestrator.sh
git commit -m "refactor(orchestrator): extract candidates.sh + unit tests

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Extract `lib/report.sh`

**Files:**
- Create: `gormes/scripts/orchestrator/lib/report.sh`
- Create: `gormes/scripts/orchestrator/tests/unit/report.bats`
- Create: `gormes/scripts/orchestrator/tests/fixtures/reports/good.final.md`
- Create: 6× `bad-*.final.md` fixtures
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh`

**Functions to extract:**
- `build_prompt` (lines ~881-1000)
- `collect_final_report_issues` (lines ~1004-1064)
- `verify_final_report` (lines ~1066-1077)
- `print_final_report_diagnostics` (lines ~1079-1104)
- `wait_for_valid_final_report` (lines ~1106-1130)
- `extract_report_field`, `extract_report_commit`, `extract_report_branch` (lines ~1132-1160)
- `extract_session_id` (lines ~1162-1166)

- [ ] **Step 4.1: Create `fixtures/reports/good.final.md`**

A valid final report with all 8 sections, RED exit nonzero, GREEN/REFACTOR/REGRESSION zeros, Branch, Commit hash:

```markdown
1) Selected task
Task: 1 / 1.A / Item A2

2) Pre-doc baseline
Files:
- docs/progress.json

3) RED proof
Command: go test ./internal/foo
Exit: 1
Snippet: FAIL: TestBar

4) GREEN proof
Command: go test ./internal/foo
Exit: 0
Snippet: PASS

5) REFACTOR proof
Command: go test ./internal/foo
Exit: 0
Snippet: PASS

6) Regression proof
Command: go test ./...
Exit: 0
Snippet: ok

7) Post-doc closeout
Files:
- docs/progress.json

8) Commit
Branch: codexu/test-run/worker1
Commit: abc1234def5678
Files:
- internal/foo/foo.go
```

- [ ] **Step 4.2: Create 6 bad fixtures**

- `bad-missing-section.final.md` — omit section `5) REFACTOR proof`.
- `bad-no-commit-hash.final.md` — `Commit:` line present but not a hex hash.
- `bad-all-zero-exits.final.md` — no `Exit: <nonzero>` anywhere.
- `bad-no-red-exit.final.md` — `Exit: 0` in RED section.
- `bad-missing-branch.final.md` — no `Branch:` line.
- `bad-empty.final.md` — empty file.

- [ ] **Step 4.3: Write `unit/report.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib report
}

@test "collect_final_report_issues passes on good fixture" {
  run collect_final_report_issues "$FIXTURES_DIR/reports/good.final.md"
  assert_success
  assert_output ""
}

@test "collect_final_report_issues fails on missing section" {
  run collect_final_report_issues "$FIXTURES_DIR/reports/bad-missing-section.final.md"
  assert_failure
  assert_output --partial "REFACTOR proof"
}

@test "collect_final_report_issues fails on missing commit hash" {
  run collect_final_report_issues "$FIXTURES_DIR/reports/bad-no-commit-hash.final.md"
  assert_failure
  assert_output --partial "Commit hash"
}

@test "collect_final_report_issues fails on all-zero exits" {
  run collect_final_report_issues "$FIXTURES_DIR/reports/bad-all-zero-exits.final.md"
  assert_failure
  assert_output --partial "non-zero RED exit"
}

@test "collect_final_report_issues fails on zero RED exit" {
  run collect_final_report_issues "$FIXTURES_DIR/reports/bad-no-red-exit.final.md"
  assert_failure
}

@test "collect_final_report_issues fails on missing branch" {
  run collect_final_report_issues "$FIXTURES_DIR/reports/bad-missing-branch.final.md"
  assert_failure
  assert_output --partial "Branch field"
}

@test "collect_final_report_issues fails on empty report" {
  run collect_final_report_issues "$FIXTURES_DIR/reports/bad-empty.final.md"
  assert_failure
}

@test "collect_final_report_issues errors when report file missing" {
  run collect_final_report_issues "/nonexistent/final.md"
  assert_failure
  assert_output --partial "Missing final report"
}

@test "extract_report_commit strips backticks" {
  local tmp
  tmp="$(mktmp_workspace)"
  printf 'Commit: `abc1234def5678`\n' > "$tmp/r.md"
  run extract_report_commit "$tmp/r.md"
  assert_output "abc1234def5678"
}

@test "extract_report_branch reads plain value" {
  local tmp
  tmp="$(mktmp_workspace)"
  printf 'Branch: codexu/foo/worker1\n' > "$tmp/r.md"
  run extract_report_branch "$tmp/r.md"
  assert_output "codexu/foo/worker1"
}

@test "extract_report_field returns empty when label absent" {
  local tmp
  tmp="$(mktmp_workspace)"
  printf 'hello\n' > "$tmp/r.md"
  run extract_report_field "Commit" "$tmp/r.md"
  assert_output ""
}
```

- [ ] **Step 4.4: Run — expect failure** (report.sh missing)

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: fails at `source_lib report`.

- [ ] **Step 4.5: Create `lib/report.sh`** with function bodies copied verbatim from entry script (lines 881-1166).

Header:
```bash
#!/usr/bin/env bash
# Report parsing and prompt construction.
# Depends on: $FINAL_REPORT_GRACE_SECONDS, $PROGRESS_JSON_REL, $BASE_COMMIT, worker_* helpers from worktree.sh.
# NOTE: build_prompt calls worker_repo_root/worker_branch_name — those stay in entry script until Task 6 extracts worktree.sh.
```

Important: `build_prompt` references `worker_repo_root` and `worker_branch_name`. Those functions are still defined in the entry script at this stage; since `lib/report.sh` is sourced AFTER the entry script defines them (wait — no, the sources are at the TOP of the script). This matters.

**Call order is determined by source order, not function-definition order.** Bash resolves function names at call time, not at source time. So `build_prompt` can reference `worker_repo_root` even if `worktree.sh` is sourced later or still inline. This works as long as `build_prompt` is never invoked until after all libs are sourced — which is true (it's only called inside `run_worker`).

- [ ] **Step 4.6: Run tests — expect green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: all unit tests pass.

- [ ] **Step 4.7: Modify entry script**

Update source loop: `for _lib in common candidates report; do`. Delete extracted function bodies (lines 881-1166).

- [ ] **Step 4.8: Smoke the entry script**

Run: `bash -n gormes/scripts/gormes-auto-codexu-orchestrator.sh && gormes/scripts/gormes-auto-codexu-orchestrator.sh --help | head -5`

Expected: clean.

- [ ] **Step 4.9: Commit**

```bash
git add gormes/scripts/orchestrator/lib/report.sh \
        gormes/scripts/orchestrator/tests/unit/report.bats \
        gormes/scripts/orchestrator/tests/fixtures/reports/ \
        gormes/scripts/gormes-auto-codexu-orchestrator.sh
git commit -m "refactor(orchestrator): extract report.sh + unit tests

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Extract `lib/claim.sh`

**Files:**
- Create: `gormes/scripts/orchestrator/lib/claim.sh`
- Create: `gormes/scripts/orchestrator/tests/unit/claim.bats`
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh`

**Functions to extract:**
- `claim_task` (lines ~660-697)
- `release_task` (lines ~699-709)
- `cleanup_stale_locks` (lines ~624-658)

- [ ] **Step 5.1: Write `unit/claim.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib common
  source_lib claim
  TMP_WS="$(mktmp_workspace)"
  export LOCKS_DIR="$TMP_WS/locks"
  export LOCK_TTL_SECONDS=10
  export RUN_ID="test-run-1"
  mkdir -p "$LOCKS_DIR"
}

teardown() {
  release_task "${CLAIM_LOCKS:-}" 2>/dev/null || true
}

@test "claim_task acquires both task + phase locks first time" {
  run claim_task "task-slug" 1 "phase-1"
  assert_success
  [[ -n "$CLAIM_LOCKS" ]]
  [[ -f "$LOCKS_DIR/task-slug.lock.claim.json" ]]
}

@test "claim_task returns 1 when same slug already locked in the same shell" {
  claim_task "task-slug" 1 "phase-1"
  local first_locks="$CLAIM_LOCKS"
  run claim_task "task-slug" 2 "phase-1"
  assert_failure
  release_task "$first_locks"
}

@test "claim_task blocks on same phase lock" {
  claim_task "slug-a" 1 "phase-x"
  local first_locks="$CLAIM_LOCKS"
  run claim_task "slug-b" 2 "phase-x"
  assert_failure
  release_task "$first_locks"
}

@test "cleanup_stale_locks removes locks with dead PIDs" {
  # Forge a claim.json with a PID that certainly does not exist
  echo "test" > "$LOCKS_DIR/stale.lock"
  jq -n --arg run "r1" --argjson pid 999999 --argjson ts 0 \
    '{run_id:$run,worker_id:1,pid:$pid,claimed_at_epoch:$ts,claimed_at_utc:"1970-01-01T00:00:00Z",host:"t"}' \
    > "$LOCKS_DIR/stale.lock.claim.json"
  run cleanup_stale_locks
  assert_success
  [[ ! -f "$LOCKS_DIR/stale.lock" ]]
  [[ ! -f "$LOCKS_DIR/stale.lock.claim.json" ]]
}

@test "cleanup_stale_locks keeps locks with live PIDs inside TTL" {
  echo "test" > "$LOCKS_DIR/live.lock"
  jq -n --arg run "r1" --argjson pid "$$" --argjson ts "$(date +%s)" \
    '{run_id:$run,worker_id:1,pid:$pid,claimed_at_epoch:$ts,claimed_at_utc:"now",host:"t"}' \
    > "$LOCKS_DIR/live.lock.claim.json"
  run cleanup_stale_locks
  assert_success
  [[ -f "$LOCKS_DIR/live.lock" ]]
}

@test "cleanup_stale_locks removes claim with missing pid field" {
  echo "test" > "$LOCKS_DIR/badpid.lock"
  echo '{}' > "$LOCKS_DIR/badpid.lock.claim.json"
  run cleanup_stale_locks
  [[ ! -f "$LOCKS_DIR/badpid.lock" ]]
}
```

- [ ] **Step 5.2: Run — expect failure**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: fails at `source_lib claim`.

- [ ] **Step 5.3: Create `lib/claim.sh`** with function bodies copied verbatim (lines 624-709).

Header:
```bash
#!/usr/bin/env bash
# Task + phase-level locking, and stale-lock reaping.
# Depends on: $LOCKS_DIR, $LOCK_TTL_SECONDS, $RUN_ID.
# Exports: global CLAIM_LOCKS (set by claim_task, consumed by release_task).
```

- [ ] **Step 5.4: Run tests — expect green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: all unit tests pass.

- [ ] **Step 5.5: Modify entry script**

Update source loop: `for _lib in common candidates report claim; do`. Delete extracted bodies.

- [ ] **Step 5.6: Smoke**

Run: `bash -n gormes/scripts/gormes-auto-codexu-orchestrator.sh && gormes/scripts/gormes-auto-codexu-orchestrator.sh --help | head -5`

- [ ] **Step 5.7: Commit**

```bash
git add gormes/scripts/orchestrator/lib/claim.sh \
        gormes/scripts/orchestrator/tests/unit/claim.bats \
        gormes/scripts/gormes-auto-codexu-orchestrator.sh
git commit -m "refactor(orchestrator): extract claim.sh + unit tests

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Extract `lib/worktree.sh`

**Files:**
- Create: `gormes/scripts/orchestrator/lib/worktree.sh`
- Create: `gormes/scripts/orchestrator/tests/unit/worktree.bats`
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh`

**Functions to extract:**
- `worker_branch_name`, `worker_worktree_root`, `worker_repo_root` (lines ~711-728)
- `create_worker_worktree` (lines ~732-740)
- `maybe_remove_worker_worktree` (lines ~807-815)
- `enforce_worktree_dir_cap` (lines ~817-841)
- `verify_worker_commit` (lines ~1168-1224 approx — verify in current entry)
- `branch_worktree_path` (lines ~238-251)

- [ ] **Step 6.1: Write `unit/worktree.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

make_fixture_repo() {
  local repo="$1"
  git init -q -b main "$repo"
  git -C "$repo" -c user.email=t@t -c user.name=T commit -q --allow-empty -m init
}

setup() {
  load_helpers
  source_lib common
  source_lib worktree
  TMP_WS="$(mktmp_workspace)"
  export GIT_ROOT="$TMP_WS/repo"
  export WORKTREES_DIR="$TMP_WS/wt"
  export REPO_SUBDIR="."
  export RUN_ID="wrt-run-1"
  export PROGRESS_JSON_REL="progress.json"
  make_fixture_repo "$GIT_ROOT"
  export BASE_COMMIT="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  mkdir -p "$WORKTREES_DIR"
}

@test "worker_branch_name format" {
  run worker_branch_name 3
  assert_output "codexu/wrt-run-1/worker3"
}

@test "worker_worktree_root format" {
  run worker_worktree_root 2
  assert_output "$WORKTREES_DIR/worker2"
}

@test "create_worker_worktree checks out base commit on new branch" {
  run create_worker_worktree 1
  assert_success
  [[ -d "$WORKTREES_DIR/worker1" ]]
  local head
  head="$(git -C "$WORKTREES_DIR/worker1" rev-parse HEAD)"
  assert_equal "$head" "$BASE_COMMIT"
  local branch
  branch="$(git -C "$WORKTREES_DIR/worker1" rev-parse --abbrev-ref HEAD)"
  assert_equal "$branch" "codexu/wrt-run-1/worker1"
}

@test "verify_worker_commit rejects unchanged HEAD" {
  create_worker_worktree 1
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$BASE_COMMIT" > "$report"
  run verify_worker_commit 1 "$report"
  assert_failure
  assert_output --partial "HEAD did not advance"
}

@test "verify_worker_commit rejects multiple commits" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo b > b && git -c user.email=t@t -c user.name=T add b && git -c user.email=t@t -c user.name=T commit -q -m b )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  run verify_worker_commit 1 "$report"
  assert_failure
  assert_output --partial "commit count"
}

@test "verify_worker_commit rejects dirty worktree" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo stray > stray )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  run verify_worker_commit 1 "$report"
  assert_failure
  assert_output --partial "not clean"
}

@test "verify_worker_commit accepts single valid commit" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  run verify_worker_commit 1 "$report"
  assert_success
}
```

- [ ] **Step 6.2: Run — expect failure**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: fails at `source_lib worktree`.

- [ ] **Step 6.3: Create `lib/worktree.sh`** with function bodies copied verbatim.

Header:
```bash
#!/usr/bin/env bash
# Git worktree lifecycle + post-run verification helpers.
# Depends on: $GIT_ROOT, $WORKTREES_DIR, $REPO_SUBDIR, $RUN_ID, $BASE_COMMIT, $KEEP_WORKTREES, $PINNED_RUNS_FILE, $MAX_RUN_WORKTREE_DIRS, $RUN_ROOT.
```

- [ ] **Step 6.4: Run tests — green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

- [ ] **Step 6.5: Modify entry script**

Update source loop: `for _lib in common candidates report claim worktree; do`. Delete extracted bodies.

- [ ] **Step 6.6: Smoke**

Run: `bash -n gormes/scripts/gormes-auto-codexu-orchestrator.sh && gormes/scripts/gormes-auto-codexu-orchestrator.sh --help | head -5`

- [ ] **Step 6.7: Commit**

```bash
git add gormes/scripts/orchestrator/lib/worktree.sh \
        gormes/scripts/orchestrator/tests/unit/worktree.bats \
        gormes/scripts/gormes-auto-codexu-orchestrator.sh
git commit -m "refactor(orchestrator): extract worktree.sh + unit tests

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Extract `lib/promote.sh`

**Files:**
- Create: `gormes/scripts/orchestrator/lib/promote.sh`
- Create: `gormes/scripts/orchestrator/tests/unit/promote.bats`
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh`

**Functions to extract:**
- `promotion_enabled` (lines ~226-228)
- `setup_integration_root` (lines ~254-294)
- `push_integration_branch` (lines ~791-805)
- `cmd_promote_commit` (lines ~1716-1729)
- `promote_successful_workers` (lines ~1731-1786)

**Also: add `PROMOTED_LAST_CYCLE` export to `promote_successful_workers`** (this is the one behavior change — no logic change, just an additional `export` line at the end).

- [ ] **Step 7.1: Write `unit/promote.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

make_fixture_repo() {
  local repo="$1"
  git init -q -b main "$repo"
  git -C "$repo" -c user.email=t@t -c user.name=T commit -q --allow-empty -m init
}

write_worker_state() {
  local id="$1" slug="$2" commit="$3" status="$4"
  local dir="$RUN_WORKER_STATE_DIR"
  mkdir -p "$dir"
  jq -n --arg run "$RUN_ID" --arg s "$status" --arg slug "$slug" --arg c "$commit" \
    '{run_id:$run,status:$s,slug:$slug,commit:$c}' > "$dir/worker_${id}.json"
}

setup() {
  load_helpers
  source_lib common
  source_lib promote
  TMP_WS="$(mktmp_workspace)"
  export GIT_ROOT="$TMP_WS/int"
  export INTEGRATION_BRANCH="codexu/autoloop"
  export AUTO_PROMOTE_SUCCESS=1
  export RUN_ID="prom-1"
  export RUN_WORKER_STATE_DIR="$TMP_WS/workers/$RUN_ID"
  export STATE_DIR="$TMP_WS/state"
  export RUNS_LEDGER="$STATE_DIR/runs.jsonl"
  export AUTO_PUSH=0
  mkdir -p "$STATE_DIR"
  make_fixture_repo "$GIT_ROOT"
  git -C "$GIT_ROOT" checkout -q -b "$INTEGRATION_BRANCH"
  # Re-source load_worker_state + log_event — they live in entry script until those extractions;
  # for promote.bats we define lightweight stubs if absent.
  type load_worker_state >/dev/null 2>&1 || load_worker_state() { cat "$RUN_WORKER_STATE_DIR/worker_$1.json" 2>/dev/null; }
  type log_event >/dev/null 2>&1 || log_event() { :; }
}

@test "promote_successful_workers skips when feature disabled" {
  export AUTO_PROMOTE_SUCCESS=0
  run promote_successful_workers 2
  assert_success
}

@test "promote_successful_workers cherry-picks one success" {
  # Build a branch that modifies a file, record its commit, then reset integration
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b feat )
  ( cd "$GIT_ROOT" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m add-a )
  local commit
  commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  write_worker_state 1 "foo__bar" "$commit" "success"
  run promote_successful_workers 1
  assert_success
  local head
  head="$(git -C "$GIT_ROOT" log --format=%s -n1 "$INTEGRATION_BRANCH")"
  assert_equal "$head" "add-a"
}

@test "promote_successful_workers aborts cherry-pick on conflict" {
  # Worker 1 commits a→"one"; integration then commits a→"two"; worker's cherry-pick will conflict.
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b feat )
  ( cd "$GIT_ROOT" && echo one > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m feat-a )
  local worker_commit
  worker_commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  ( cd "$GIT_ROOT" && echo two > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m int-a )
  write_worker_state 1 "foo__bar" "$worker_commit" "success"
  run promote_successful_workers 1
  assert_failure
  [[ ! -f "$GIT_ROOT/.git/CHERRY_PICK_HEAD" ]]
}

@test "promote_successful_workers exports PROMOTED_LAST_CYCLE" {
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b feat )
  ( cd "$GIT_ROOT" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m add-a )
  local commit
  commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  write_worker_state 1 "foo__bar" "$commit" "success"
  promote_successful_workers 1
  assert_equal "$PROMOTED_LAST_CYCLE" "1"
}
```

- [ ] **Step 7.2: Run — expect failure**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: fails at `source_lib promote`.

- [ ] **Step 7.3: Create `lib/promote.sh`** with function bodies copied verbatim.

At the end of `promote_successful_workers`, **immediately before `return "$rc"`**, add:

```bash
  export PROMOTED_LAST_CYCLE="$promoted"
```

Header for the file:
```bash
#!/usr/bin/env bash
# Promotion (cherry-pick) of successful worker commits onto the integration branch.
# Depends on: $AUTO_PROMOTE_SUCCESS, $GIT_ROOT, $INTEGRATION_BRANCH, $ORIGINAL_REPO_ROOT, $RUN_WORKER_STATE_DIR, $AUTO_PUSH, $REMOTE_NAME.
# Exports: PROMOTED_LAST_CYCLE (count of cherry-picks that landed this invocation).
```

- [ ] **Step 7.4: Run tests — green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

- [ ] **Step 7.5: Modify entry script**

Update source loop: `for _lib in common candidates report claim worktree promote; do`. Delete extracted bodies.

- [ ] **Step 7.6: Smoke**

Run: `bash -n gormes/scripts/gormes-auto-codexu-orchestrator.sh && gormes/scripts/gormes-auto-codexu-orchestrator.sh --help | head -5`

- [ ] **Step 7.7: Commit**

```bash
git add gormes/scripts/orchestrator/lib/promote.sh \
        gormes/scripts/orchestrator/tests/unit/promote.bats \
        gormes/scripts/gormes-auto-codexu-orchestrator.sh
git commit -m "refactor(orchestrator): extract promote.sh + export PROMOTED_LAST_CYCLE

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Integration test — happy path with `fake-codexu`

**Files:**
- Create: `gormes/scripts/orchestrator/tests/fixtures/bin/fake-codexu`
- Create: `gormes/scripts/orchestrator/tests/integration/happy-path.bats`

- [ ] **Step 8.1: Write `fake-codexu`**

```bash
#!/usr/bin/env bash
# Stub for codexu exec --json -c ... --output-last-message FILE PROMPT
set -Eeuo pipefail

FAKE_CODEXU_MODE="${FAKE_CODEXU_MODE:-success}"
FAKE_CODEXU_LOG="${FAKE_CODEXU_LOG:-/dev/null}"
final_file=""
prompt=""

while (( $# > 0 )); do
  case "$1" in
    exec|--json|-c)        shift; [[ "$1" != --* ]] && shift || true; continue ;;
    --sandbox)             shift 2; continue ;;
    --output-last-message) final_file="$2"; shift 2; continue ;;
    *)                     prompt="$1"; shift ;;
  esac
done

echo "fake-codexu mode=$FAKE_CODEXU_MODE final=$final_file" >> "$FAKE_CODEXU_LOG"
echo '{"type":"thread.started","thread_id":"fake-thread-1"}'

case "$FAKE_CODEXU_MODE" in
  timeout)
    sleep "${FAKE_CODEXU_SLEEP:-99999}"
    ;;
  contract_fail)
    # Write an incomplete report, exit 1
    cat > "$final_file" <<REPORT
1) Selected task
Task: x/y/z
REPORT
    exit 1
    ;;
  success)
    # Read prompt, extract branch from it, commit one file, write valid report
    # build_prompt emits lines like:
    #   Repository root:
    #     /path/to/worker_repo
    # ...
    #   - Git branch: codexu/<run>/worker<N>
    local_branch="$(grep -E '^- Git branch: ' <<<"$prompt" | head -n1 | sed -E 's/^- Git branch: *//')"
    local_wt="$(grep -E '^  /' <<<"$prompt" | head -n1 | sed -E 's/^  //')"
    : "${local_branch:=codexu/fake/worker1}"
    if [[ -n "$local_wt" && -d "$local_wt" ]]; then
      cd "$local_wt"
      printf 'from-fake-codexu\n' > "fake-output-$$.txt"
      git -c user.email=t@t -c user.name=T add "fake-output-$$.txt"
      git -c user.email=t@t -c user.name=T commit -q -m "fake: $$"
      head="$(git rev-parse HEAD)"
    else
      head="0000000000000000000000000000000000000000"
    fi
    cat > "$final_file" <<REPORT
1) Selected task
Task: 1 / 1.A / Item A2

2) Pre-doc baseline
Files:
- progress.json

3) RED proof
Command: go test ./...
Exit: 1
Snippet: FAIL

4) GREEN proof
Command: go test ./...
Exit: 0
Snippet: PASS

5) REFACTOR proof
Command: go test ./...
Exit: 0
Snippet: PASS

6) Regression proof
Command: go test ./...
Exit: 0
Snippet: ok

7) Post-doc closeout
Files:
- progress.json

8) Commit
Branch: $local_branch
Commit: $head
Files:
- fake-output-$$.txt
REPORT
    exit 0
    ;;
  conflict)
    cd "${FAKE_CODEXU_WORKTREE:-.}"
    printf 'fake-conflict-%s\n' "$$" > progress.fixture.json
    git -c user.email=t@t -c user.name=T add progress.fixture.json
    git -c user.email=t@t -c user.name=T commit -q -m "fake: $$"
    head="$(git rev-parse HEAD)"
    cat > "$final_file" <<REPORT
1) Selected task
Task: 1 / 1.A / Item A2

2) Pre-doc baseline
Files:
- progress.fixture.json

3) RED proof
Command: x
Exit: 1
Snippet: x

4) GREEN proof
Command: x
Exit: 0
Snippet: x

5) REFACTOR proof
Command: x
Exit: 0
Snippet: x

6) Regression proof
Command: x
Exit: 0
Snippet: x

7) Post-doc closeout
Files:
- progress.fixture.json

8) Commit
Branch: ${FAKE_CODEXU_BRANCH:-codexu/fake/worker1}
Commit: $head
Files:
- progress.fixture.json
REPORT
    exit 0
    ;;
  *)
    echo "fake-codexu: unknown mode $FAKE_CODEXU_MODE" >&2
    exit 2
    ;;
esac
```

Make executable: `chmod +x gormes/scripts/orchestrator/tests/fixtures/bin/fake-codexu`.

- [ ] **Step 8.2: Write `integration/happy-path.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"
  export PATH="$FIXTURES_DIR/bin:$PATH"
  export FAKE_CODEXU_MODE=success

  # Minimal Go-repo-shape fixture: a git repo with a progress.json and a dummy file
  export REPO_ROOT="$TMP_WS/repo"
  git init -q -b main "$REPO_ROOT"
  cp "$FIXTURES_DIR/progress.fixture.json" "$REPO_ROOT/progress.json"
  mkdir -p "$REPO_ROOT/docs/content/building-gormes/architecture_plan"
  cp "$FIXTURES_DIR/progress.fixture.json" \
     "$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
  ( cd "$REPO_ROOT" && git -c user.email=t@t -c user.name=T add -A && git -c user.email=t@t -c user.name=T commit -q -m init )

  export RUN_ROOT="$TMP_WS/run"
  export MAX_AGENTS=1
  export MODE=safe
  export ORCHESTRATOR_ONCE=1
  export HEARTBEAT_SECONDS=1
  export FINAL_REPORT_GRACE_SECONDS=1
  export WORKER_TIMEOUT_SECONDS=60
  export MIN_AVAILABLE_MEM_MB=1
  export MIN_MEM_PER_WORKER_MB=1
  export MAX_EXISTING_CHROMIUM=9999
  export AUTO_PROMOTE_SUCCESS=1
  export INTEGRATION_BRANCH="codexu/test-autoloop"
}

@test "one worker succeeds, promotes to integration branch" {
  run "$ENTRY_SCRIPT"
  assert_success
  # Integration branch got a new commit
  run git -C "$REPO_ROOT" log --oneline "$INTEGRATION_BRANCH"
  assert_success
  [[ "$(echo "$output" | wc -l)" -ge 2 ]]
  # Ledger recorded worker_success + worker_promoted
  grep -q 'worker_success' "$RUN_ROOT/state/runs.jsonl"
  grep -q 'worker_promoted' "$RUN_ROOT/state/runs.jsonl"
}
```

- [ ] **Step 8.3: Run — expect green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh integration`

Expected: 1 test passes.

- [ ] **Step 8.4: Commit**

```bash
git add gormes/scripts/orchestrator/tests/fixtures/bin/fake-codexu \
        gormes/scripts/orchestrator/tests/integration/happy-path.bats
git commit -m "test(orchestrator): add fake-codexu + happy-path integration test

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Integration test — cherry-pick conflict

**Files:**
- Create: `gormes/scripts/orchestrator/tests/integration/cherry-pick-conflict.bats`

- [ ] **Step 9.1: Write `integration/cherry-pick-conflict.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"
  export PATH="$FIXTURES_DIR/bin:$PATH"
  export FAKE_CODEXU_MODE=conflict
  export FAKE_CODEXU_WORKTREE="."   # fake uses cwd

  export REPO_ROOT="$TMP_WS/repo"
  git init -q -b main "$REPO_ROOT"
  echo '{}' > "$REPO_ROOT/progress.fixture.json"
  cp "$FIXTURES_DIR/progress.fixture.json" "$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json" 2>/dev/null || {
    mkdir -p "$REPO_ROOT/docs/content/building-gormes/architecture_plan"
    cp "$FIXTURES_DIR/progress.fixture.json" "$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
  }
  ( cd "$REPO_ROOT" && git -c user.email=t@t -c user.name=T add -A && git -c user.email=t@t -c user.name=T commit -q -m init )

  export RUN_ROOT="$TMP_WS/run"
  export MAX_AGENTS=2
  export MODE=safe
  export ORCHESTRATOR_ONCE=1
  export HEARTBEAT_SECONDS=1
  export FINAL_REPORT_GRACE_SECONDS=1
  export WORKER_TIMEOUT_SECONDS=60
  export MIN_AVAILABLE_MEM_MB=1
  export MIN_MEM_PER_WORKER_MB=1
  export MAX_EXISTING_CHROMIUM=9999
  export AUTO_PROMOTE_SUCCESS=1
  export INTEGRATION_BRANCH="codexu/test-autoloop-conflict"
}

@test "two conflicting workers: one promotes, other emits cherry_pick_failed, integration clean" {
  run "$ENTRY_SCRIPT"
  # The run itself may return non-zero because one worker fails promotion;
  # we care about the ledger + git state, not the exit code.
  grep -q 'worker_promoted' "$RUN_ROOT/state/runs.jsonl"
  grep -q 'cherry_pick_failed' "$RUN_ROOT/state/runs.jsonl"
  [[ ! -f "$REPO_ROOT/.git/CHERRY_PICK_HEAD" ]]
  # Exactly one promotion should have landed
  local promoted_count
  promoted_count="$(grep -c 'worker_promoted.*promoted$' "$RUN_ROOT/state/runs.jsonl" || true)"
  assert_equal "$promoted_count" "1"
}
```

- [ ] **Step 9.2: Run — expect green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh integration`

Expected: 2 tests pass.

- [ ] **Step 9.3: Commit**

```bash
git add gormes/scripts/orchestrator/tests/integration/cherry-pick-conflict.bats
git commit -m "test(orchestrator): cherry-pick conflict regression test

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Integration tests — resume + skipped poison-task-retry

**Files:**
- Create: `gormes/scripts/orchestrator/tests/integration/resume.bats`
- Create: `gormes/scripts/orchestrator/tests/integration/poison-task-retry.bats`

- [ ] **Step 10.1: Write `integration/poison-task-retry.bats` (skipped)**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

@test "poison-task retry cap (Spec A target)" {
  skip "Spec A target — current orchestrator has no retry cap; test enables when Spec A lands."
}
```

- [ ] **Step 10.2: Write `integration/resume.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"
  export PATH="$FIXTURES_DIR/bin:$PATH"

  export REPO_ROOT="$TMP_WS/repo"
  git init -q -b main "$REPO_ROOT"
  mkdir -p "$REPO_ROOT/docs/content/building-gormes/architecture_plan"
  cp "$FIXTURES_DIR/progress.fixture.json" "$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
  ( cd "$REPO_ROOT" && git -c user.email=t@t -c user.name=T add -A && git -c user.email=t@t -c user.name=T commit -q -m init )

  export RUN_ROOT="$TMP_WS/run"
  export MAX_AGENTS=1
  export MODE=safe
  export ORCHESTRATOR_ONCE=1
  export HEARTBEAT_SECONDS=1
  export FINAL_REPORT_GRACE_SECONDS=1
  export WORKER_TIMEOUT_SECONDS=3
  export MIN_AVAILABLE_MEM_MB=1
  export MIN_MEM_PER_WORKER_MB=1
  export MAX_EXISTING_CHROMIUM=9999
  export AUTO_PROMOTE_SUCCESS=0
  export INTEGRATION_BRANCH="codexu/test-resume"
}

@test "resume picks up interrupted run without duplicate claims" {
  export FAKE_CODEXU_MODE=timeout
  export FAKE_CODEXU_SLEEP=9999
  export RUN_ID="resume-test-1"

  # First run: worker will time out after WORKER_TIMEOUT_SECONDS=3
  run "$ENTRY_SCRIPT"
  # Expect failure, ledger records worker_failed timeout
  grep -q 'worker_failed.*timeout' "$RUN_ROOT/state/runs.jsonl"

  export FAKE_CODEXU_MODE=success
  run "$ENTRY_SCRIPT" --resume "$RUN_ID"
  assert_success
  grep -q 'worker_success' "$RUN_ROOT/state/runs.jsonl"
  # Ensure we didn't double-claim: should see at most 2 worker_claimed rows for that run_id (initial + resume)
  local claim_count
  claim_count="$(grep -c "\"run_id\":\"$RUN_ID\".*worker_claimed" "$RUN_ROOT/state/runs.jsonl" || true)"
  (( claim_count <= 2 ))
}
```

- [ ] **Step 10.3: Run — expect green (resume passes, poison skips)**

Run: `bash gormes/scripts/orchestrator/tests/run.sh integration`

Expected: 2 pass, 1 skip, 0 fail.

- [ ] **Step 10.4: Commit**

```bash
git add gormes/scripts/orchestrator/tests/integration/resume.bats \
        gormes/scripts/orchestrator/tests/integration/poison-task-retry.bats
git commit -m "test(orchestrator): add resume regression + skipped poison-task target

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: Add `lib/companions.sh` skeleton + unit tests

**Files:**
- Create: `gormes/scripts/orchestrator/lib/companions.sh`
- Create: `gormes/scripts/orchestrator/tests/unit/companions.bats`
- Create: `gormes/scripts/orchestrator/tests/fixtures/planner_state.fixture.json`

This task adds the predicates and state I/O. Wiring into the `main` loop happens in Task 12.

- [ ] **Step 11.1: Create `fixtures/planner_state.fixture.json`**

```json
{
  "last_run_utc": "2026-04-22T18:00:00Z",
  "upstream_hermes_dir": "/tmp/hermes",
  "local_branch": "main"
}
```

- [ ] **Step 11.2: Write `unit/companions.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib common
  source_lib companions

  TMP_WS="$(mktmp_workspace)"
  export RUN_ROOT="$TMP_WS/run"
  export STATE_DIR="$RUN_ROOT/state"
  export LOGS_DIR="$RUN_ROOT/logs"
  mkdir -p "$STATE_DIR" "$LOGS_DIR"

  export PLANNER_EVERY_N_CYCLES=4
  export DOC_IMPROVER_EVERY_N_CYCLES=6
  export LANDINGPAGE_EVERY_N_HOURS=24
  export PLANNER_ROOT="$TMP_WS/planner"
  mkdir -p "$PLANNER_ROOT"

  export CANDIDATES_FILE="$TMP_WS/cands.json"
  echo '[]' > "$CANDIDATES_FILE"

  # Ensure companion_state_dir is computed
  export ORCH_COMPANION_STATE_DIR="$(companion_state_dir)"
  mkdir -p "$ORCH_COMPANION_STATE_DIR"
}

write_companion_state() {
  local name="$1" ts="$2" cycle="$3"
  jq -n --arg ts "$ts" --argjson cycle "$cycle" --argjson rc 0 \
    '{ts_epoch:($ts|tonumber),cycle:$cycle,rc:$rc}' \
    > "$ORCH_COMPANION_STATE_DIR/${name}.last.json"
}

@test "companion_cycles_since returns large N when never run" {
  run companion_cycles_since planner 10
  assert_success
  (( output >= 10 ))
}

@test "companion_cycles_since returns diff since last run" {
  write_companion_state planner "$(date +%s)" 5
  run companion_cycles_since planner 9
  assert_output "4"
}

@test "should_run_planner fires on exhaustion (unclaimed<10%)" {
  # 10 candidates total, 0 unclaimed
  echo '[]' > "$CANDIDATES_FILE"
  write_companion_state planner "$(date +%s)" 0
  export _TOTAL_PROGRESS_ITEMS=10
  run should_run_planner 1
  assert_success
}

@test "should_run_planner fires on cycle interval" {
  echo '[]' > "$CANDIDATES_FILE"
  write_companion_state planner "$(date +%s)" 0
  export _TOTAL_PROGRESS_ITEMS=100
  echo '[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20]' > "$CANDIDATES_FILE"
  run should_run_planner 4
  assert_success
}

@test "should_run_planner skips if external systemd ran recently" {
  cp "$FIXTURES_DIR/planner_state.fixture.json" "$PLANNER_ROOT/planner_state.json"
  # Set the fixture's last_run to just now
  jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '.last_run_utc = $now' "$PLANNER_ROOT/planner_state.json" \
    > "$PLANNER_ROOT/planner_state.json.tmp" && mv "$PLANNER_ROOT/planner_state.json.tmp" "$PLANNER_ROOT/planner_state.json"
  write_companion_state planner 0 0
  run should_run_planner 99
  assert_failure
}

@test "should_run_doc_improver skips when no promotions last cycle" {
  write_companion_state doc_improver 0 0
  export PROMOTED_LAST_CYCLE=0
  run should_run_doc_improver 10
  assert_failure
}

@test "should_run_doc_improver fires when interval reached + promotion happened" {
  write_companion_state doc_improver 0 0
  export PROMOTED_LAST_CYCLE=1
  run should_run_doc_improver 10
  assert_success
}

@test "should_run_landingpage fires after 24h" {
  # 25 hours ago
  write_companion_state landingpage "$(( $(date +%s) - 25 * 3600 ))" 0
  run should_run_landingpage
  assert_success
}

@test "should_run_landingpage skips within 24h" {
  write_companion_state landingpage "$(( $(date +%s) - 3600 ))" 0
  run should_run_landingpage
  assert_failure
}
```

- [ ] **Step 11.3: Run — expect failure** (companions.sh missing)

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: fails at `source_lib companions`.

- [ ] **Step 11.4: Create `lib/companions.sh`**

```bash
#!/usr/bin/env bash
# Companion-script periodic invocation helpers.
# Depends on: $RUN_ROOT, $STATE_DIR, $PLANNER_EVERY_N_CYCLES, $DOC_IMPROVER_EVERY_N_CYCLES,
#             $LANDINGPAGE_EVERY_N_HOURS, $PLANNER_ROOT, $LOOP_SLEEP_SECONDS,
#             $PROMOTED_LAST_CYCLE, $DISABLE_COMPANIONS, $COMPANION_ON_IDLE,
#             $COMPANION_TIMEOUT_SECONDS, $COMPANION_PLANNER_CMD,
#             $COMPANION_DOC_IMPROVER_CMD, $COMPANION_LANDINGPAGE_CMD.
# Reads the candidates file + optional $_TOTAL_PROGRESS_ITEMS override (tests only).

companion_state_dir() {
  printf '%s/companions\n' "$RUN_ROOT"
}

companion_last_ts() {
  local name="$1"
  local f
  f="$(companion_state_dir)/${name}.last.json"
  if [[ -f "$f" ]]; then
    jq -r '.ts_epoch // 0' "$f"
  else
    printf '0\n'
  fi
}

companion_last_cycle() {
  local name="$1"
  local f
  f="$(companion_state_dir)/${name}.last.json"
  if [[ -f "$f" ]]; then
    jq -r '.cycle // 0' "$f"
  else
    printf '0\n'
  fi
}

companion_cycles_since() {
  local name="$1"
  local current_cycle="$2"
  local last
  last="$(companion_last_cycle "$name")"
  printf '%d\n' $(( current_cycle - last ))
}

_candidates_remaining() {
  [[ -f "$CANDIDATES_FILE" ]] || { printf '0\n'; return; }
  jq 'length' "$CANDIDATES_FILE"
}

_planner_external_recent() {
  local state="$PLANNER_ROOT/planner_state.json"
  [[ -f "$state" ]] || return 1
  local ts
  ts="$(jq -r '.last_run_utc // empty' "$state")"
  [[ -n "$ts" ]] || return 1
  local epoch
  epoch="$(date -d "$ts" +%s 2>/dev/null || true)"
  [[ -n "$epoch" ]] || return 1
  local threshold=$(( PLANNER_EVERY_N_CYCLES * ${LOOP_SLEEP_SECONDS:-30} * 2 ))
  local now
  now="$(date +%s)"
  (( now - epoch < threshold ))
}

should_run_planner() {
  local cycle="$1"
  [[ "${DISABLE_COMPANIONS:-0}" == "1" ]] && return 1
  _planner_external_recent && return 1
  local remaining total
  remaining="$(_candidates_remaining)"
  total="${_TOTAL_PROGRESS_ITEMS:-$remaining}"
  (( total > 0 )) || return 0
  # Exhaustion trigger: unclaimed < 10%
  if (( remaining * 10 < total )); then
    return 0
  fi
  local since
  since="$(companion_cycles_since planner "$cycle")"
  (( since >= PLANNER_EVERY_N_CYCLES ))
}

should_run_doc_improver() {
  local cycle="$1"
  [[ "${DISABLE_COMPANIONS:-0}" == "1" ]] && return 1
  local promoted="${PROMOTED_LAST_CYCLE:-0}"
  (( promoted >= 1 )) || return 1
  local since
  since="$(companion_cycles_since doc_improver "$cycle")"
  (( since >= DOC_IMPROVER_EVERY_N_CYCLES ))
}

should_run_landingpage() {
  [[ "${DISABLE_COMPANIONS:-0}" == "1" ]] && return 1
  local last
  last="$(companion_last_ts landingpage)"
  local now
  now="$(date +%s)"
  local delta=$(( now - last ))
  (( delta >= LANDINGPAGE_EVERY_N_HOURS * 3600 ))
}

run_companion() {
  local name="$1"
  local cmd_var_name
  case "$name" in
    planner)       cmd_var_name="COMPANION_PLANNER_CMD" ;;
    doc_improver)  cmd_var_name="COMPANION_DOC_IMPROVER_CMD" ;;
    landingpage)   cmd_var_name="COMPANION_LANDINGPAGE_CMD" ;;
    *) echo "run_companion: unknown companion '$name'" >&2; return 1 ;;
  esac
  local cmd="${!cmd_var_name:-}"
  if [[ -z "$cmd" ]]; then
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
    case "$name" in
      planner)       cmd="$script_dir/gormes-architecture-planner-tasks-manager.sh" ;;
      doc_improver)  cmd="$script_dir/documentation-improver.sh" ;;
      landingpage)   cmd="$script_dir/landingpage-improver.sh" ;;
    esac
  fi

  mkdir -p "$(companion_state_dir)"
  local ts_start
  ts_start="$(date +%s)"
  local ts_utc
  ts_utc="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  local rc=0
  (
    cd "$GIT_ROOT"
    AUTO_COMMIT=1 AUTO_PUSH=0 PLANNER_INSTALL_SCHEDULE=0 \
      timeout "${COMPANION_TIMEOUT_SECONDS:-1800}" bash "$cmd" \
      >"$LOGS_DIR/companion_${name}.out.log" 2>"$LOGS_DIR/companion_${name}.err.log"
  ) || rc=$?

  jq -n \
    --arg name "$name" \
    --argjson ts_epoch "$ts_start" \
    --arg ts_utc "$ts_utc" \
    --argjson cycle "${ORCH_CURRENT_CYCLE:-0}" \
    --argjson rc "$rc" \
    '{name:$name,ts_epoch:$ts_epoch,ts_utc:$ts_utc,cycle:$cycle,rc:$rc}' \
    > "$(companion_state_dir)/${name}.last.json"

  type log_event >/dev/null 2>&1 && log_event "companion_${name}_completed" null "rc=$rc" "completed" || true
  return "$rc"
}

maybe_run_companions() {
  local cycle="$1"
  local promoted="${2:-0}"
  export ORCH_CURRENT_CYCLE="$cycle"
  export PROMOTED_LAST_CYCLE="$promoted"

  [[ "${DISABLE_COMPANIONS:-0}" == "1" ]] && return 0

  local exhausted=0
  local remaining
  remaining="$(_candidates_remaining)"
  local total="${_TOTAL_PROGRESS_ITEMS:-$remaining}"
  if (( total > 0 )) && (( remaining * 10 < total )); then
    exhausted=1
  fi

  if [[ "${COMPANION_ON_IDLE:-1}" == "1" && "$exhausted" == "0" && "$promoted" == "0" ]]; then
    return 0
  fi

  if should_run_planner "$cycle"; then
    run_companion planner || true
    export EXHAUSTION_TRIGGERED="$exhausted"
  fi
  if should_run_doc_improver "$cycle"; then
    run_companion doc_improver || true
  fi
  if should_run_landingpage; then
    run_companion landingpage || true
  fi
}
```

- [ ] **Step 11.5: Run tests — green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit`

Expected: all unit tests pass, including 9 new `companions.bats` assertions.

- [ ] **Step 11.6: Commit**

```bash
git add gormes/scripts/orchestrator/lib/companions.sh \
        gormes/scripts/orchestrator/tests/unit/companions.bats \
        gormes/scripts/orchestrator/tests/fixtures/planner_state.fixture.json
git commit -m "feat(orchestrator): add companions.sh predicates + unit tests

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 12: Wire `maybe_run_companions` into main loop + integration test

**Files:**
- Create: `gormes/scripts/orchestrator/tests/fixtures/bin/fake-planner`
- Create: `gormes/scripts/orchestrator/tests/fixtures/bin/fake-doc-improver`
- Create: `gormes/scripts/orchestrator/tests/fixtures/bin/fake-landingpage`
- Create: `gormes/scripts/orchestrator/tests/integration/companion-trigger.bats`
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh`

- [ ] **Step 12.1: Write `fake-planner`**

```bash
#!/usr/bin/env bash
set -Eeuo pipefail
marker_dir="${FAKE_COMPANION_MARKER_DIR:-/tmp}"
mkdir -p "$marker_dir"
printf '%s planner cycle=%s promoted=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${ORCH_CURRENT_CYCLE:-?}" "${PROMOTED_LAST_CYCLE:-?}" \
  >> "$marker_dir/planner.marker"
exit "${FAKE_PLANNER_RC:-0}"
```

Chmod +x. Identical `fake-doc-improver` writes `doc_improver.marker`, identical `fake-landingpage` writes `landingpage.marker`.

- [ ] **Step 12.2: Modify entry script**

Source loop becomes:
```bash
for _lib in common candidates report claim worktree promote companions; do
```

In `main()`, inside the forever loop, change the tail from:

```bash
    if run_once; then
      cycle_rc=0
    else
      cycle_rc="$?"
    fi

    echo
    echo "Loop cycle $cycle completed with exit $cycle_rc; sleeping ${LOOP_SLEEP_SECONDS}s before next run."
    sleep "$LOOP_SLEEP_SECONDS"
```

to:

```bash
    if run_once; then
      cycle_rc=0
    else
      cycle_rc="$?"
    fi

    maybe_run_companions "$cycle" "${PROMOTED_LAST_CYCLE:-0}"

    if [[ "${EXHAUSTION_TRIGGERED:-0}" == "1" ]]; then
      EXHAUSTION_TRIGGERED=0
      echo "Loop cycle $cycle completed with exit $cycle_rc; exhausted → skipping sleep."
      continue
    fi

    echo
    echo "Loop cycle $cycle completed with exit $cycle_rc; sleeping ${LOOP_SLEEP_SECONDS}s before next run."
    sleep "$LOOP_SLEEP_SECONDS"
```

Update `usage()` text to document new env vars (copy verbatim from spec §"New env vars"). Add a new section near the end of the env list.

- [ ] **Step 12.3: Write `integration/companion-trigger.bats`**

```bash
#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"
  export PATH="$FIXTURES_DIR/bin:$PATH"
  export FAKE_CODEXU_MODE=success
  export FAKE_COMPANION_MARKER_DIR="$TMP_WS/markers"
  mkdir -p "$FAKE_COMPANION_MARKER_DIR"

  export REPO_ROOT="$TMP_WS/repo"
  git init -q -b main "$REPO_ROOT"
  mkdir -p "$REPO_ROOT/docs/content/building-gormes/architecture_plan"
  cp "$FIXTURES_DIR/progress.fixture.json" "$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
  ( cd "$REPO_ROOT" && git -c user.email=t@t -c user.name=T add -A && git -c user.email=t@t -c user.name=T commit -q -m init )

  export RUN_ROOT="$TMP_WS/run"
  export MAX_AGENTS=1
  export MODE=safe
  export ORCHESTRATOR_ONCE=1
  export HEARTBEAT_SECONDS=1
  export FINAL_REPORT_GRACE_SECONDS=1
  export WORKER_TIMEOUT_SECONDS=60
  export MIN_AVAILABLE_MEM_MB=1
  export MIN_MEM_PER_WORKER_MB=1
  export MAX_EXISTING_CHROMIUM=9999
  export AUTO_PROMOTE_SUCCESS=1
  export INTEGRATION_BRANCH="codexu/test-comp"

  export PLANNER_EVERY_N_CYCLES=1
  export DOC_IMPROVER_EVERY_N_CYCLES=1
  export LANDINGPAGE_EVERY_N_HOURS=0
  export COMPANION_ON_IDLE=0
  export COMPANION_PLANNER_CMD="$FIXTURES_DIR/bin/fake-planner"
  export COMPANION_DOC_IMPROVER_CMD="$FIXTURES_DIR/bin/fake-doc-improver"
  export COMPANION_LANDINGPAGE_CMD="$FIXTURES_DIR/bin/fake-landingpage"
  export PLANNER_ROOT="$TMP_WS/planner"
  mkdir -p "$PLANNER_ROOT"
}

@test "orchestrator triggers companions after successful cycle" {
  run "$ENTRY_SCRIPT"
  # planner marker should exist (no external systemd state means should_run_planner fires)
  [[ -f "$FAKE_COMPANION_MARKER_DIR/planner.marker" ]]
  # doc-improver only if there was a promotion
  if grep -q 'worker_promoted' "$RUN_ROOT/state/runs.jsonl"; then
    [[ -f "$FAKE_COMPANION_MARKER_DIR/doc_improver.marker" ]]
  fi
  # landing page marker should exist (24h=0 → always fires first time)
  [[ -f "$FAKE_COMPANION_MARKER_DIR/landingpage.marker" ]]
}

@test "DISABLE_COMPANIONS=1 blocks all companion runs" {
  export DISABLE_COMPANIONS=1
  run "$ENTRY_SCRIPT"
  [[ ! -f "$FAKE_COMPANION_MARKER_DIR/planner.marker" ]]
  [[ ! -f "$FAKE_COMPANION_MARKER_DIR/doc_improver.marker" ]]
  [[ ! -f "$FAKE_COMPANION_MARKER_DIR/landingpage.marker" ]]
}
```

- [ ] **Step 12.4: Run — expect green**

Run: `bash gormes/scripts/orchestrator/tests/run.sh unit integration`

Expected: all tests pass, ~5 integration tests.

- [ ] **Step 12.5: Smoke the entry script**

Run: `bash -n gormes/scripts/gormes-auto-codexu-orchestrator.sh && gormes/scripts/gormes-auto-codexu-orchestrator.sh --help | grep -E 'DISABLE_COMPANIONS|PLANNER_EVERY'`

Expected: help text shows new env vars.

- [ ] **Step 12.6: Commit**

```bash
git add gormes/scripts/orchestrator/lib/companions.sh \
        gormes/scripts/orchestrator/tests/fixtures/bin/fake-planner \
        gormes/scripts/orchestrator/tests/fixtures/bin/fake-doc-improver \
        gormes/scripts/orchestrator/tests/fixtures/bin/fake-landingpage \
        gormes/scripts/orchestrator/tests/integration/companion-trigger.bats \
        gormes/scripts/gormes-auto-codexu-orchestrator.sh
git commit -m "feat(orchestrator): wire maybe_run_companions into main loop

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 13: Add Makefile targets + orchestrator README

**Files:**
- Modify: `gormes/Makefile`
- Create: `gormes/scripts/orchestrator/README.md`

- [ ] **Step 13.1: Add Make targets**

Append to `gormes/Makefile`:

```makefile
.PHONY: orchestrator-test orchestrator-test-all orchestrator-lint

orchestrator-test:
	@bash scripts/orchestrator/tests/run.sh unit

orchestrator-test-all:
	@bash scripts/orchestrator/tests/run.sh unit integration

orchestrator-lint:
	@if command -v shellcheck >/dev/null 2>&1; then \
	  shellcheck scripts/gormes-auto-codexu-orchestrator.sh scripts/orchestrator/lib/*.sh; \
	else \
	  echo "shellcheck not installed; skipping"; \
	fi
```

Also add `orchestrator-test orchestrator-test-all orchestrator-lint` to the existing `.PHONY` line if the user prefers one consolidated list.

- [ ] **Step 13.2: Run Make targets**

Run: `cd gormes && make orchestrator-test`

Expected: unit tier passes.

Run: `cd gormes && make orchestrator-test-all`

Expected: unit + integration pass.

- [ ] **Step 13.3: Write `gormes/scripts/orchestrator/README.md`**

```markdown
# Orchestrator Internals

Companion libraries and tests for `gormes/scripts/gormes-auto-codexu-orchestrator.sh`.

## Layout

- `lib/` — sourced modules. Each file is side-effect-free; they declare functions that the entry script or tests call. Module docstrings at the top of each file list env vars they read.
- `tests/bootstrap.sh` — downloads and verifies vendored bats-core, bats-assert, bats-support into `tests/vendor/` (gitignored).
- `tests/run.sh unit` / `tests/run.sh integration` / `tests/run.sh unit integration` — test runner.
- `tests/fixtures/` — canned progress JSON, report markdown (good + 6 bad), and mock backend binaries (`fake-codexu`, `fake-planner`, `fake-doc-improver`, `fake-landingpage`).

## Running tests

```sh
make -C .. orchestrator-test        # unit only, <5s
make -C .. orchestrator-test-all    # unit + integration, <2min
```

## Adding a new library module

1. Create `lib/<name>.sh`. Start with the standard header (module doc + depends-on list).
2. Add a `unit/<name>.bats` test file, load `'../lib/test_env'` in setup, call `source_lib <name>` after `load_helpers`.
3. Add the new name to the `for _lib in ...` loop at the top of `gormes-auto-codexu-orchestrator.sh`.
4. Run `make orchestrator-test-all`.

## Companion scheduling

The orchestrator's forever loop interleaves three companion scripts between cycles:

| Companion | Predicate | Typical cadence |
|---|---|---|
| `gormes-architecture-planner-tasks-manager.sh` | exhaustion (<10% candidates remain) OR cycles since last ≥ `PLANNER_EVERY_N_CYCLES` (default 4). Skipped if external systemd timer ran within `PLANNER_EVERY_N_CYCLES × LOOP_SLEEP_SECONDS × 2` seconds. | ~ every 4 cycles |
| `documentation-improver.sh` | cycles since last ≥ `DOC_IMPROVER_EVERY_N_CYCLES` (default 6) AND last cycle promoted ≥ 1 commit. | ~ every 6 productive cycles |
| `landingpage-improver.sh` | hours since last ≥ `LANDINGPAGE_EVERY_N_HOURS` (default 24). | daily |

Companions run serially on the integration worktree with `AUTO_COMMIT=1 AUTO_PUSH=0 PLANNER_INSTALL_SCHEDULE=0`, so their commits become the next cycle's `BASE_COMMIT`.

Escape hatches: `DISABLE_COMPANIONS=1` fully disables. `COMPANION_ON_IDLE=0` allows companions to run on any cycle (default `1` gates them to idle/post-promotion cycles).
```

- [ ] **Step 13.4: Commit**

```bash
git add gormes/Makefile gormes/scripts/orchestrator/README.md
git commit -m "docs(orchestrator): add Make targets + internals README

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 14: Final smoke — real orchestrator invocation + schema parity

- [ ] **Step 14.1: Full test suite**

Run: `cd gormes && make orchestrator-test-all`

Expected: all unit + integration tests pass.

- [ ] **Step 14.2: shellcheck (if installed)**

Run: `cd gormes && make orchestrator-lint`

Expected: either `shellcheck not installed; skipping` or zero warnings on the entry script + new libs.

- [ ] **Step 14.3: Schema parity check — real repo, one cycle**

Run (on a short-lived branch, not the running prod loop):

```sh
cd gormes/scripts
# Save a snapshot of the current ledger BEFORE the test cycle
cp .codex/orchestrator/state/runs.jsonl /tmp/runs.before.jsonl 2>/dev/null || true

MAX_AGENTS=1 ORCHESTRATOR_ONCE=1 DISABLE_COMPANIONS=1 ./gormes-auto-codexu-orchestrator.sh \
  > /tmp/smoke.out 2> /tmp/smoke.err || true

# Diff the newly-written events against the schema (all fields match old format)
jq -c 'keys_unsorted | sort' .codex/orchestrator/state/runs.jsonl | sort -u > /tmp/schema.new
jq -c 'keys_unsorted | sort' /tmp/runs.before.jsonl | sort -u > /tmp/schema.old
diff /tmp/schema.old /tmp/schema.new || {
  echo "SCHEMA REGRESSION DETECTED" >&2
  exit 1
}
```

Expected: `diff` is empty.

If the user's running orchestrator is still live, pick `RUN_ROOT=/tmp/smoke-run` to isolate.

- [ ] **Step 14.4: Verify forever-loop picks up changes on next cycle**

If the prod forever loop is running: tail `runs.jsonl` and wait for the next `run_started`. Confirm it references the same entry script path and emits `run_completed` with the same schema.

- [ ] **Step 14.5: Final commit (if any doc adjustments needed)**

If Step 14.3 revealed any gap (e.g. missing `companion_*` event schema in README), update `README.md` and commit:

```bash
git commit -am "docs(orchestrator): refine companion event schema notes

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

Otherwise, no commit needed — task is a verification step.

---

## Acceptance summary

- `make orchestrator-test` passes in <5s.
- `make orchestrator-test-all` passes in <2min.
- Each commit in the sequence keeps all previously-green tests green.
- `runs.jsonl` schema has exactly the same keys as the pre-refactor version plus optional `companion_*` events.
- The running prod forever loop picks up the refactor on its next cycle boundary without restart.
- Spec A's `poison-task-retry.bats` is in place but skipped, ready to be enabled once Spec A lands.

## Follow-up specs

- **Spec A** — Effectiveness fixes: enable poison-task-retry test, add per-task failure memory, rebase-first promotion, file-scope partitioning, failure-context injection into retry prompts.
- **Spec B** — Backend adapter: `--claudeu` / `--opencode` flags, per-backend argv/exec/report parsing via a new `lib/backend.sh`.
