#!/usr/bin/env bash
# Per-task failure records to drive poison-pill decisions and retry-context
# injection. State lives at $STATE_DIR/task-failures/<slug>.json.
# Depends on: $STATE_DIR.

failures_dir() { printf '%s/task-failures\n' "$STATE_DIR"; }

failure_record_path() {
  local slug="$1"
  printf '%s/%s.json\n' "$(failures_dir)" "$slug"
}

failure_record_read() {
  local slug="$1"
  local p; p="$(failure_record_path "$slug")"
  [[ -f "$p" ]] || return 1
  cat "$p"
}

# Args: slug, rc, reason, stderr_file, final_errors_json (JSON array of strings)
failure_record_write() {
  local slug="$1" rc="$2" reason="$3" stderr_file="${4:-}" final_errors_json="${5:-[]}"
  mkdir -p "$(failures_dir)"
  local p; p="$(failure_record_path "$slug")"
  local prev_count=0
  if [[ -f "$p" ]]; then
    prev_count="$(jq -r '.count // 0' "$p" 2>/dev/null || echo 0)"
  fi
  local stderr_tail=""
  if [[ -n "$stderr_file" && -f "$stderr_file" ]]; then
    stderr_tail="$(tail -n 40 "$stderr_file" 2>/dev/null || true)"
  fi
  jq -n \
    --arg slug "$slug" \
    --argjson count "$((prev_count + 1))" \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg rc "$rc" \
    --arg reason "$reason" \
    --arg stderr_file "$stderr_file" \
    --arg stderr_tail "$stderr_tail" \
    --argjson final_errors "$final_errors_json" \
    '{slug:$slug, count:$count, last_ts:$ts, last_rc:($rc|tonumber? // 0), last_reason:$reason, last_stderr_file:$stderr_file, last_stderr_tail:$stderr_tail, last_final_errors:$final_errors}' \
    > "$p"
}

failure_record_reset() {
  local slug="$1"
  rm -f "$(failure_record_path "$slug")" 2>/dev/null || true
}

is_poisoned() {
  local slug="$1" max="${2:-${MAX_RETRIES:-3}}"
  local record; record="$(failure_record_read "$slug" 2>/dev/null || echo '{}')"
  local count; count="$(jq -r '.count // 0' <<<"$record")"
  (( count >= max ))
}
