#!/usr/bin/env bash
# Git worktree lifecycle + post-run verification helpers.
# Depends on: $GIT_ROOT, $WORKTREES_DIR, $REPO_SUBDIR, $RUN_ID, $BASE_COMMIT,
#             $KEEP_WORKTREES, $PINNED_RUNS_FILE, $MAX_RUN_WORKTREE_DIRS, $RUN_ROOT.

branch_worktree_path() {
  local git_root="$1"
  local branch="$2"

  git -C "$git_root" worktree list --porcelain \
    | awk -v branch_ref="refs/heads/${branch}" '
        /^worktree / { path = substr($0, 10) }
        /^branch / {
          if (!found && substr($0, 8) == branch_ref) {
            print path
            found = 1
          }
        }
      '
}

worker_branch_name() {
  local worker_id="$1"
  printf 'codexu/%s/worker%d' "$RUN_ID" "$worker_id"
}

worker_worktree_root() {
  local worker_id="$1"
  printf '%s/worker%d' "$WORKTREES_DIR" "$worker_id"
}

worker_repo_root() {
  local worker_id="$1"
  local worktree_root
  worktree_root="$(worker_worktree_root "$worker_id")"
  if [[ "$REPO_SUBDIR" == "." ]]; then
    printf '%s\n' "$worktree_root"
  else
    printf '%s/%s\n' "$worktree_root" "$REPO_SUBDIR"
  fi
}

worker_salvage_report() {
  local target_run="${1:-$RUN_ID}"
  local run_worktrees_dir="$RUN_ROOT/worktrees/$target_run"
  local run_worker_state_dir="$STATE_DIR/workers/$target_run"
  local ids=()
  local id f d

  declare -A seen_ids=()
  shopt -s nullglob
  for f in "$run_worker_state_dir"/worker_*.json; do
    id="$(basename "$f")"
    id="${id#worker_}"
    id="${id%.json}"
    [[ "$id" =~ ^[0-9]+$ ]] && seen_ids["$id"]=1
  done
  for d in "$run_worktrees_dir"/worker*; do
    [[ -d "$d" ]] || continue
    id="$(basename "$d")"
    id="${id#worker}"
    [[ "$id" =~ ^[0-9]+$ ]] && seen_ids["$id"]=1
  done
  shopt -u nullglob

  if ((${#seen_ids[@]} > 0)); then
    mapfile -t ids < <(printf '%s\n' "${!seen_ids[@]}" | sort -n)
  fi

  printf 'Run: %s\n' "$target_run"
  printf 'Worktrees: %s\n' "$run_worktrees_dir"
  printf 'Worker state: %s\n' "$run_worker_state_dir"

  if ((${#ids[@]} == 0)); then
    printf 'No worker state or worktrees found for run %s\n' "$target_run"
    return 0
  fi

  for id in "${ids[@]}"; do
    local state_file="$run_worker_state_dir/worker_${id}.json"
    local worktree="$run_worktrees_dir/worker${id}"
    local status="unknown"
    local reason="-"
    local commit="-"
    local slug="-"
    local task="-"

    if [[ -f "$state_file" ]] && command -v jq >/dev/null 2>&1; then
      status="$(jq -r '.status // "unknown"' "$state_file" 2>/dev/null || printf 'unknown')"
      reason="$(jq -r '.reason // "-"' "$state_file" 2>/dev/null || printf '-')"
      commit="$(jq -r '.commit // "-"' "$state_file" 2>/dev/null || printf '-')"
      slug="$(jq -r '.slug // "-"' "$state_file" 2>/dev/null || printf '-')"
      local phase subphase item
      phase="$(jq -r '.phase_id // ""' "$state_file" 2>/dev/null || true)"
      subphase="$(jq -r '.subphase_id // ""' "$state_file" 2>/dev/null || true)"
      item="$(jq -r '.item_name // ""' "$state_file" 2>/dev/null || true)"
      if [[ -n "$item" && "$item" != "null" ]]; then
        task="${phase}/${subphase}: ${item}"
      elif [[ "$slug" != "-" && "$slug" != "null" ]]; then
        task="$slug"
      fi
    fi

    if [[ -d "$worktree" ]] && git -C "$worktree" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      local branch head status_output dirty_count
      branch="$(git -C "$worktree" rev-parse --abbrev-ref HEAD 2>/dev/null || printf '?')"
      head="$(git -C "$worktree" rev-parse --short HEAD 2>/dev/null || printf '?')"
      status_output="$(git -C "$worktree" status --short 2>/dev/null || true)"
      dirty_count="$(printf '%s\n' "$status_output" | sed '/^$/d' | wc -l | tr -d ' ')"
      printf 'worker%s status=%s reason=%s commit=%s branch=%s head=%s dirty=%s inspect=%s\n' \
        "$id" "$status" "$reason" "$commit" "$branch" "$head" "$dirty_count" "$worktree"
      printf '  task=%s\n' "$task"
      if ((dirty_count > 0)); then
        printf '%s\n' "$status_output" | sed -n '1,10s/^/  /p'
      fi
    else
      printf 'worker%s status=%s reason=%s commit=%s worktree=missing inspect=%s\n' \
        "$id" "$status" "$reason" "$commit" "$worktree"
      printf '  task=%s\n' "$task"
    fi
  done
}

dirty_worker_worktree_report() {
  local worktrees_base="$RUN_ROOT/worktrees"
  local run_dir worktree run_id worker_id status_output dirty_count

  [[ -d "$worktrees_base" ]] || return 0

  shopt -s nullglob
  for run_dir in "$worktrees_base"/*; do
    [[ -d "$run_dir" ]] || continue
    run_id="$(basename "$run_dir")"
    for worktree in "$run_dir"/worker*; do
      [[ -d "$worktree" ]] || continue
      git -C "$worktree" rev-parse --is-inside-work-tree >/dev/null 2>&1 || continue
      status_output="$(git -C "$worktree" status --short 2>/dev/null || true)"
      dirty_count="$(printf '%s\n' "$status_output" | sed '/^$/d' | wc -l | tr -d ' ')"
      if ((dirty_count == 0)); then
        continue
      fi
      worker_id="$(basename "$worktree")"
      worker_id="${worker_id#worker}"
      printf 'run=%s worker=%s dirty=%s path=%s\n' "$run_id" "$worker_id" "$dirty_count" "$worktree"
      printf '%s\n' "$status_output" | sed -n '1,10s/^/  /p'
    done
  done
  shopt -u nullglob
}

refuse_dirty_worker_worktrees() {
  local report
  report="$(dirty_worker_worktree_report)"
  [[ -n "$report" ]] || return 0

  if [[ "${ALLOW_DIRTY_WORKER_WORKTREES:-0}" == "1" ]]; then
    echo "Warning: ALLOW_DIRTY_WORKER_WORKTREES=1; continuing with dirty retained worker worktrees." >&2
    printf '%s\n' "$report" >&2
    return 0
  fi

  echo "Refusing to launch workers: dirty retained worker worktrees found." >&2
  echo "Salvage or remove these before rerunning, or set ALLOW_DIRTY_WORKER_WORKTREES=1 after manual review." >&2
  printf '%s\n' "$report" >&2
  printf '%s\n' "$report" \
    | awk -F'[ =]' '/^run=/ && !seen[$2]++ { print "  scripts/gormes-auto-codexu-orchestrator.sh salvage " $2 }' >&2
  return 1
}

create_worker_worktree() {
  local worker_id="$1"
  local worktree_root branch
  worktree_root="$(worker_worktree_root "$worker_id")"
  branch="$(worker_branch_name "$worker_id")"

  mkdir -p "$(dirname "$worktree_root")"
  git -C "$GIT_ROOT" worktree add -b "$branch" "$worktree_root" "$BASE_COMMIT" >/dev/null 2>&1
}

maybe_remove_worker_worktree() {
  local worker_id="$1"
  local worktree_root
  worktree_root="$(worker_worktree_root "$worker_id")"

  if [[ "$KEEP_WORKTREES" == "0" && -d "$worktree_root" ]]; then
    git -C "$GIT_ROOT" worktree remove --force "$worktree_root" >/dev/null 2>&1 || true
  fi
}

enforce_worktree_dir_cap() {
  local keep="$MAX_RUN_WORKTREE_DIRS"
  local dirs=()
  local d

  while IFS= read -r d; do
    [[ -n "$d" ]] && dirs+=("$d")
  done < <(find "$RUN_ROOT/worktrees" -mindepth 1 -maxdepth 1 -type d -printf '%T@ %p\n' 2>/dev/null | sort -nr | awk '{print $2}')

  local idx=0
  for d in "${dirs[@]}"; do
    idx=$((idx + 1))
    if (( idx <= keep )); then
      continue
    fi
    if [[ "$(basename "$d")" == "$RUN_ID" ]]; then
      continue
    fi
    if grep -Fxq "$(basename "$d")" "$PINNED_RUNS_FILE" 2>/dev/null; then
      continue
    fi
    git -C "$GIT_ROOT" worktree remove --force "$d" >/dev/null 2>&1 || true
    rm -rf "$d" 2>/dev/null || true
  done
}

report_has_runtime_flag() {
  # Args: <final_file> <flag_name>
  # Returns 0 if final report has a "10) Runtime flags" section containing
  # `<flag_name>: true` (case-insensitive), 1 otherwise. Accepts markdown
  # header prefix and bold markers on the section title, same flexibility as
  # the rest of the report validator. Historical note: Runtime flags lived
  # at section 9 before Orchestrator Final Polish; Acceptance check took 9
  # and Runtime flags moved to 10.
  local final_file="$1"
  local flag_name="$2"

  [[ -f "$final_file" ]] || return 1

  awk -v flag="$flag_name" '
    BEGIN { IGNORECASE = 1; in_section = 0 }
    /^[[:space:]]*(#{1,6}[[:space:]]+)?(\*\*)?10[).][[:space:]]*(\*\*)?Runtime flags(\*\*)?/ {
      in_section = 1
      next
    }
    # Any other "N) Title" or "N. Title" section header ends the runtime-flags
    # block. Match a leading integer followed by ) or . to avoid bleeding.
    in_section && /^[[:space:]]*(#{1,6}[[:space:]]+)?(\*\*)?[0-9]+[).][[:space:]]/ {
      in_section = 0
    }
    in_section {
      # Allow optional leading "- " bullet prefix and trimming of whitespace.
      line = $0
      sub(/^[[:space:]]*[-*][[:space:]]*/, "", line)
      sub(/^[[:space:]]+/, "", line)
      if (match(line, "^" flag "[[:space:]]*:[[:space:]]*true[[:space:]]*$")) {
        found = 1
        exit
      }
    }
    END { exit(found ? 0 : 1) }
  ' "$final_file"
}

verify_worker_commit() {
  local worker_id="$1"
  local final_file="$2"
  local worktree_root branch head_commit report_commit report_branch commit_count status_output changed_files file
  local allow_multi_commit tolerate_untracked status_cmd_flag

  # Reset reason so a successful run doesn't leak a stale value from
  # a prior invocation.
  LAST_VERIFY_REASON=""
  export LAST_VERIFY_REASON

  # Env defaults are rigid; per-worker report flags can opt into softer
  # behavior for a single run without setting global env vars.
  allow_multi_commit="${ALLOW_MULTI_COMMIT:-0}"
  tolerate_untracked="${TOLERATE_WORKTREE_UNTRACKED:-0}"
  if report_has_runtime_flag "$final_file" "AllowMultiCommit"; then
    allow_multi_commit=1
  fi
  if report_has_runtime_flag "$final_file" "TolerateWorktreeUntracked"; then
    tolerate_untracked=1
  fi

  worktree_root="$(worker_worktree_root "$worker_id")"
  branch="$(worker_branch_name "$worker_id")"
  head_commit="$(git -C "$worktree_root" rev-parse HEAD)"
  if [[ "$head_commit" == "$BASE_COMMIT" ]]; then
    LAST_VERIFY_REASON="no_commit_made"
    echo "worker[$worker_id]: HEAD did not advance beyond $BASE_COMMIT" >&2
    return 1
  fi

  commit_count="$(git -C "$worktree_root" rev-list --count "${BASE_COMMIT}..HEAD")"
  if [[ "$allow_multi_commit" == "1" ]]; then
    if (( commit_count < 1 )); then
      LAST_VERIFY_REASON="wrong_commit_count"
      echo "worker[$worker_id]: commit count = $commit_count, want >= 1" >&2
      return 1
    fi
  else
    if [[ "$commit_count" != "1" ]]; then
      LAST_VERIFY_REASON="wrong_commit_count"
      echo "worker[$worker_id]: commit count = $commit_count, want exactly 1" >&2
      return 1
    fi
  fi

  if [[ "$tolerate_untracked" == "1" ]]; then
    status_output="$(git -C "$worktree_root" status --porcelain --untracked-files=no)"
  else
    status_output="$(git -C "$worktree_root" status --short)"
  fi
  if [[ -n "$status_output" ]]; then
    LAST_VERIFY_REASON="worktree_dirty"
    echo "worker[$worker_id]: worktree not clean after run" >&2
    printf '%s\n' "$status_output" >&2
    return 1
  fi

  report_commit="$(extract_report_commit "$final_file")"
  if [[ -z "$report_commit" || "$head_commit" != "$report_commit"* ]]; then
    LAST_VERIFY_REASON="report_commit_mismatch"
    echo "worker[$worker_id]: report commit does not match HEAD ($report_commit vs $head_commit)" >&2
    return 1
  fi

  report_branch="$(extract_report_branch "$final_file")"
  if [[ "$report_branch" != "$branch" ]]; then
    LAST_VERIFY_REASON="branch_mismatch"
    echo "worker[$worker_id]: report branch does not match expected branch ($report_branch vs $branch)" >&2
    return 1
  fi

  changed_files="$(git -C "$worktree_root" diff --name-only "${BASE_COMMIT}..HEAD")"
  if [[ -z "$changed_files" ]]; then
    LAST_VERIFY_REASON="no_commit_made"
    echo "worker[$worker_id]: commit contains no file changes" >&2
    return 1
  fi

  while IFS= read -r file; do
    [[ -z "$file" ]] && continue
    if [[ "$REPO_SUBDIR" != "." && "$file" != "$REPO_SUBDIR/"* ]]; then
      LAST_VERIFY_REASON="scope_violation"
      echo "worker[$worker_id]: changed file escaped allowed scope: $file" >&2
      return 1
    fi
  done <<< "$changed_files"

  return 0
}
