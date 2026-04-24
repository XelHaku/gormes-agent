#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib common
  source_lib failures
  STATE_DIR="$(mktmp_workspace)"
  export STATE_DIR
}

@test "failure_record_write creates record file" {
  run failure_record_write "task-a" "1" "report_validation_failed" "" "[]"
  assert_success
  [[ -f "$STATE_DIR/task-failures/task-a.json" ]]
}

@test "failure_record_read returns JSON with count=1 after first write" {
  failure_record_write "task-a" "1" "report_validation_failed" "" "[]"
  run failure_record_read "task-a"
  assert_success
  local count
  count="$(jq -r '.count' <<<"$output")"
  assert_equal "$count" "1"
  local reason
  reason="$(jq -r '.last_reason' <<<"$output")"
  assert_equal "$reason" "report_validation_failed"
}

@test "failure_record_write increments count on repeated writes" {
  failure_record_write "task-b" "1" "report_validation_failed" "" "[]"
  failure_record_write "task-b" "1" "no_commit_made" "" "[]"
  failure_record_write "task-b" "1" "worktree_dirty" "" "[]"
  run failure_record_read "task-b"
  assert_success
  local count
  count="$(jq -r '.count' <<<"$output")"
  assert_equal "$count" "3"
  local reason
  reason="$(jq -r '.last_reason' <<<"$output")"
  assert_equal "$reason" "worktree_dirty"
}

@test "failure_record_reset removes the record file" {
  failure_record_write "task-c" "1" "timeout" "" "[]"
  [[ -f "$STATE_DIR/task-failures/task-c.json" ]]
  run failure_record_reset "task-c"
  assert_success
  [[ ! -f "$STATE_DIR/task-failures/task-c.json" ]]
}

@test "failure_record_reset on missing file is a no-op" {
  run failure_record_reset "task-nonexistent"
  assert_success
}

@test "failure_record_read fails when record does not exist" {
  run failure_record_read "task-missing"
  assert_failure
}

@test "is_poisoned is false when no record exists" {
  run is_poisoned "fresh-task"
  assert_failure
}

@test "is_poisoned is false at count=1 with default MAX_RETRIES=3" {
  failure_record_write "task-d" "1" "report_validation_failed" "" "[]"
  run is_poisoned "task-d"
  assert_failure
}

@test "is_poisoned is true at count=3 with default MAX_RETRIES=3" {
  failure_record_write "task-e" "1" "report_validation_failed" "" "[]"
  failure_record_write "task-e" "1" "report_validation_failed" "" "[]"
  failure_record_write "task-e" "1" "report_validation_failed" "" "[]"
  run is_poisoned "task-e"
  assert_success
}

@test "is_poisoned respects explicit max argument" {
  failure_record_write "task-f" "1" "x" "" "[]"
  failure_record_write "task-f" "1" "x" "" "[]"
  run is_poisoned "task-f" 2
  assert_success
  run is_poisoned "task-f" 5
  assert_failure
}

@test "failure_record_write captures stderr tail when file provided" {
  local tmp
  tmp="$(mktmp_workspace)"
  local stderr_file="$tmp/err.log"
  local i
  for (( i = 1; i <= 60; i++ )); do
    printf 'line %d\n' "$i" >> "$stderr_file"
  done
  failure_record_write "task-g" "1" "report_validation_failed" "$stderr_file" "[]"
  run failure_record_read "task-g"
  assert_success
  local tail_text
  tail_text="$(jq -r '.last_stderr_tail' <<<"$output")"
  [[ "$tail_text" == *"line 60"* ]]
  [[ "$tail_text" == *"line 21"* ]]
  [[ "$tail_text" != *"line 20"* ]]
}

@test "failure_record_write stores final_errors JSON array" {
  local errors_json
  errors_json='["Missing section Commit","Bad Exit code"]'
  failure_record_write "task-h" "1" "report_validation_failed" "" "$errors_json"
  run failure_record_read "task-h"
  assert_success
  local first_err
  first_err="$(jq -r '.last_final_errors[0]' <<<"$output")"
  assert_equal "$first_err" "Missing section Commit"
  local len
  len="$(jq -r '.last_final_errors | length' <<<"$output")"
  assert_equal "$len" "2"
}

@test "failure_record_write defaults invalid final_errors_json to [] (no zero-byte)" {
  # Production bug repro: caller piped invalid JSON (two values concatenated).
  run failure_record_write "task-invalid" "1" "contract_or_test_failure" "" '["a"]
[]'
  assert_success
  local p="$STATE_DIR/task-failures/task-invalid.json"
  [[ -f "$p" && -s "$p" ]]
  run jq -r '.last_final_errors | length' "$p"
  assert_output "0"
}

@test "failure_record_write with empty string defaults to [] and writes content" {
  run failure_record_write "task-empty" "1" "timeout" "" ""
  assert_success
  local p="$STATE_DIR/task-failures/task-empty.json"
  [[ -f "$p" && -s "$p" ]]
}

@test "failure_record_write writes atomically: no partial file on jq failure" {
  # Malformed --argjson input would kill jq; we should return non-zero AND not
  # leave a zero-byte record behind.
  local p="$STATE_DIR/task-failures/task-atomic.json"
  # Pre-populate a valid record
  failure_record_write "task-atomic" "1" "timeout" "" '[]'
  local before_size; before_size="$(wc -c <"$p")"
  # Corrupt-JSON case: caller gave non-array JSON. Defensive validation should
  # swap in [] and succeed; OR if any other jq -n failure occurs, the file
  # must be left intact.
  run failure_record_write "task-atomic" "1" "timeout" "" 'this-is-not-json'
  local after_size; after_size="$(wc -c <"$p")"
  (( after_size >= before_size ))  # never truncated
}
