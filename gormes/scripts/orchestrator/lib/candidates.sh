#!/usr/bin/env bash
# Candidate list normalization helpers.
# Depends on: $PROGRESS_JSON, $ACTIVE_FIRST, $CANDIDATES_FILE (reads only).

normalize_candidates() {
  # PHASE_PRIORITY_BOOST: comma-separated list of subphase_id values (e.g.
  # "3.E.7,2.F.3,2.E.2"). Matching items get priority_rank=0 so they come
  # before every other candidate regardless of phase_id. Respects the
  # human-curated TDD priority queue documented in phase-2-gateway.md.
  jq -c --arg active_first "$ACTIVE_FIRST" --arg boost_csv "${PHASE_PRIORITY_BOOST:-}" '
    def status_rank(s):
      if ($active_first == "1") then
        if (s == "in_progress") then 0
        elif (s == "planned") then 1
        else 2 end
      else 0 end;

    ($boost_csv | split(",") | map(ascii_downcase | gsub("[[:space:]]+"; ""))
      | map(select(. != ""))) as $boost |

    [
      (.phases // {})
      | to_entries[]
      | .key as $phase_id
      | (.value.subphases // .value.sub_phases // {})
      | to_entries[]
      | .key as $subphase_id
      | (.value.items // [])[]
      | {
          phase_id: $phase_id,
          subphase_id: $subphase_id,
          item_name: (.item_name // .name // .title // .id),
          status: ((.status // "unknown") | tostring | ascii_downcase)
        }
      | select(.item_name != null and .item_name != "")
      | select(.status != "complete")
      | . + {
          status_rank: status_rank(.status),
          priority_rank: ((.subphase_id | ascii_downcase) as $sp_lower | if ($boost | index($sp_lower)) != null then 0 else 1 end)
        }
    ]
    | unique_by([.phase_id, .subphase_id, .item_name])
    | sort_by([.priority_rank, .status_rank, .phase_id, .subphase_id, .item_name])
    | map(del(.status_rank, .priority_rank))
  ' "$PROGRESS_JSON"
}

poisoned_slugs() {
  # Emit one slug per line for tasks where:
  #   (worker_failed + worker_promotion_failed) - worker_promoted >= MAX_RETRIES
  # in the lifetime of $RUNS_LEDGER. Stops runaway retry storms.
  local max="${MAX_RETRIES:-3}"
  [[ -f "${RUNS_LEDGER:-}" ]] || return 0
  jq -rs --argjson max "$max" '
    [ .[]
      | select(.event == "worker_failed" or .event == "worker_promoted" or .event == "worker_promotion_failed")
      | {slug: ((.detail // "") | split("@")[0]), event: .event}
      | select(.slug != "")
    ]
    | group_by(.slug)
    | map({
        slug: .[0].slug,
        score: ((map(select(.event == "worker_failed" or .event == "worker_promotion_failed")) | length)
                - (map(select(.event == "worker_promoted")) | length))
      })
    | map(select(.score >= $max))
    | .[].slug
  ' "$RUNS_LEDGER" 2>/dev/null || true
}

apply_phase_floor() {
  # Optional filter: when PHASE_FLOOR is set to a positive integer, keep
  # only candidates whose numeric phase_id <= PHASE_FLOOR. Lets the
  # operator prioritize lower phases when upper phases would otherwise
  # dominate a stale candidate set. No-op when unset or empty.
  local input="$1" output="$2"
  local floor="${PHASE_FLOOR:-}"
  if [[ -z "$floor" || ! "$floor" =~ ^[0-9]+$ ]]; then
    cp "$input" "$output"
    return 0
  fi
  jq -c --argjson floor "$floor" '
    map(select((.phase_id | tonumber? // 999) <= $floor))
  ' "$input" > "$output"
}

apply_phase_skip() {
  # Optional filter: drop candidates whose subphase_id appears in
  # PHASE_SKIP_SUBPHASES (comma-separated, case-insensitive match).
  # Used to defer regional / low-priority adapters while keeping the
  # rest of the phase eligible. No-op when unset or empty.
  local input="$1" output="$2"
  local skip_csv="${PHASE_SKIP_SUBPHASES:-}"
  if [[ -z "$skip_csv" ]]; then
    cp "$input" "$output"
    return 0
  fi
  jq -c --arg skip_csv "$skip_csv" '
    ($skip_csv | split(",") | map(ascii_downcase | gsub("[[:space:]]+"; ""))
      | map(select(. != ""))) as $skip |
    map(select((.subphase_id | ascii_downcase) as $sp_lower
               | ($skip | index($sp_lower)) == null))
  ' "$input" > "$output"
}

write_candidates_file() {
  local skip_json
  skip_json="$(poisoned_slugs | jq -Rnc '[inputs | select(length > 0)]')"
  local tmp
  tmp="$(mktemp "${CANDIDATES_FILE}.XXXXXX")" || return 1
  if [[ "$skip_json" == "[]" || -z "$skip_json" ]]; then
    normalize_candidates > "$tmp"
  else
    normalize_candidates \
      | jq -c --argjson skip "$skip_json" --arg active_first "${ACTIVE_FIRST:-1}" '
          def mk_slug(p; s; i):
            (p + "__" + s + "__" + i)
            | ascii_downcase
            | gsub("[^a-z0-9._-]+"; "-")
            | sub("^-+"; "")
            | sub("-+$"; "")
            | gsub("--+"; "-");
          map(select(mk_slug(.phase_id; .subphase_id; .item_name) as $s
                     | ($skip | index($s)) == null))
        ' > "$tmp"
  fi
  # Apply optional phase-floor + subphase-skip filters on the already-
  # poison-pruned set. Ordering: floor first (cheap numeric cut), then skip
  # (string membership) so the skip list only checks items still in scope.
  local tmp2
  tmp2="$(mktemp "${CANDIDATES_FILE}.XXXXXX")" || { rm -f "$tmp"; return 1; }
  apply_phase_floor "$tmp" "$tmp2"
  apply_phase_skip "$tmp2" "$CANDIDATES_FILE"
  rm -f "$tmp" "$tmp2"
}

candidate_count() {
  jq 'length' "$CANDIDATES_FILE"
}

candidate_at() {
  local idx="$1"
  jq -c ".[$idx]" "$CANDIDATES_FILE"
}

task_slug() {
  local phase_id="$1"
  local subphase_id="$2"
  local item_name="$3"

  printf '%s__%s__%s' "$phase_id" "$subphase_id" "$item_name" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^a-z0-9._-]+/-/g; s/^-+//; s/-+$//; s/--+/-/g'
}
