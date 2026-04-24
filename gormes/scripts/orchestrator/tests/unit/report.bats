#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib common
  source_lib candidates
  source_lib report
  source_lib failures
  source_lib worktree
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

@test "collect_final_report_issues accepts optional section 10 Runtime flags" {
  local tmp
  tmp="$(mktmp_workspace)"
  local report="$tmp/good-with-section10.final.md"
  cat "$FIXTURES_DIR/reports/good.final.md" > "$report"
  printf '\n10) Runtime flags\nAllowMultiCommit: true\nTolerateWorktreeUntracked: true\n' >> "$report"
  run collect_final_report_issues "$report"
  assert_success
  assert_output ""
}

@test "collect_final_report_issues accepts sections 9 (Acceptance) and 10 (Runtime flags) together" {
  local tmp
  tmp="$(mktmp_workspace)"
  local report="$tmp/good-with-sections-9-and-10.final.md"
  # good.final.md already has section 9 Acceptance check; append section 10.
  cat "$FIXTURES_DIR/reports/good.final.md" > "$report"
  printf '\n10) Runtime flags\nAllowMultiCommit: true\n' >> "$report"
  run collect_final_report_issues "$report"
  assert_success
  assert_output ""
}

@test "collect_final_report_issues rejects report with no Acceptance section" {
  run collect_final_report_issues "$FIXTURES_DIR/reports/bad-no-acceptance.final.md"
  assert_failure
  assert_output --partial "Acceptance check"
}

@test "collect_final_report_issues rejects Acceptance section with a FAIL criterion" {
  run collect_final_report_issues "$FIXTURES_DIR/reports/bad-acceptance-fail.final.md"
  assert_failure
  assert_output --partial "failing criterion"
}

@test "build_prompt announces acceptance-criteria contract" {
  local tmp
  tmp="$(mktmp_workspace)"
  export STATE_DIR="$tmp/state"
  export WORKTREES_DIR="$tmp/worktrees"
  export REPO_SUBDIR="."
  export RUN_ID="testrun"
  export BASE_COMMIT="abc1234"
  export PROGRESS_JSON_REL="docs/progress.json"
  mkdir -p "$STATE_DIR"
  local selected='{"phase_id":"1","subphase_id":"1.A","item_name":"Item A1","status":"planned"}'
  local prompt_file="$tmp/prompt.txt"
  run build_prompt 1 "$selected" "0:1/1.A/Item A1" "$prompt_file"
  assert_success
  run cat "$prompt_file"
  assert_success
  assert_output --partial "ACCEPTANCE CRITERIA"
  assert_output --partial "9) Acceptance check"
  assert_output --partial "Criterion:"
}

@test "build_prompt omits PRIOR ATTEMPT FEEDBACK when no failure record" {
  local tmp
  tmp="$(mktmp_workspace)"
  export STATE_DIR="$tmp/state"
  export WORKTREES_DIR="$tmp/worktrees"
  export REPO_SUBDIR="."
  export RUN_ID="testrun"
  export BASE_COMMIT="abc1234"
  export PROGRESS_JSON_REL="docs/progress.json"
  mkdir -p "$STATE_DIR"
  local selected
  selected='{"phase_id":"1","subphase_id":"1.A","item_name":"Item A1","status":"planned"}'
  local prompt_file="$tmp/prompt.txt"
  run build_prompt 1 "$selected" "0:1/1.A/Item A1" "$prompt_file"
  assert_success
  run cat "$prompt_file"
  assert_success
  refute_output --partial "PRIOR ATTEMPT FEEDBACK"
  assert_output --partial "Mission:"
}

@test "build_prompt injects PRIOR ATTEMPT FEEDBACK when failure record exists" {
  local tmp
  tmp="$(mktmp_workspace)"
  export STATE_DIR="$tmp/state"
  export WORKTREES_DIR="$tmp/worktrees"
  export REPO_SUBDIR="."
  export RUN_ID="testrun"
  export BASE_COMMIT="abc1234"
  export PROGRESS_JSON_REL="docs/progress.json"
  mkdir -p "$STATE_DIR"

  local stderr_file="$tmp/stderr.log"
  printf 'panic: explosive failure at line 9000\nstack trace blah\n' > "$stderr_file"
  local slug
  slug="$(task_slug "1" "1.A" "Item A1")"
  failure_record_write "$slug" "1" "report_validation_failed" "$stderr_file" '["Missing section GREEN proof","Missing Commit hash"]'

  local selected
  selected='{"phase_id":"1","subphase_id":"1.A","item_name":"Item A1","status":"planned"}'
  local prompt_file="$tmp/prompt.txt"
  run build_prompt 1 "$selected" "0:1/1.A/Item A1" "$prompt_file"
  assert_success
  run cat "$prompt_file"
  assert_success
  assert_output --partial "PRIOR ATTEMPT FEEDBACK"
  assert_output --partial "This task has been attempted 1 times before"
  assert_output --partial "report_validation_failed"
  assert_output --partial "Missing section GREEN proof"
  assert_output --partial "Missing Commit hash"
  assert_output --partial "panic: explosive failure"
  assert_output --partial "Mission:"
}
