#!/usr/bin/env bash
# Promotion of successful worker commits onto the integration branch.
#
# Two modes, selected by PROMOTION_MODE (default: pr):
#   - pr          : push the worker branch and open a GitHub PR. If the PR
#                   flow fails for any reason (gh missing, auth bad, push or
#                   create failing), fall back to the cherry-pick path for
#                   that worker AND mark PR mode broken for the rest of this
#                   cycle so subsequent workers go straight to cherry-pick.
#   - cherry-pick : original behavior — cherry-pick commits directly onto
#                   the integration worktree with -Xtheirs.
#
# Depends on: $AUTO_PROMOTE_SUCCESS, $GIT_ROOT, $INTEGRATION_BRANCH, $ORIGINAL_REPO_ROOT,
#             $RUN_WORKER_STATE_DIR, $AUTO_PUSH, $REMOTE_NAME, $PROMOTION_MODE,
#             $PR_REPO_SLUG, $LOGS_DIR.
# Exports: PROMOTED_LAST_CYCLE (count of promotions that landed this invocation).

: "${PROMOTION_MODE:=pr}"
: "${PR_REPO_SLUG:=TrebuchetDynamics/gormes-agent}"
: "${PR_BODY_MAX_BYTES:=60000}"

promotion_enabled() {
  [[ "$AUTO_PROMOTE_SUCCESS" == "1" ]]
}

setup_integration_root() {
  promotion_enabled || return 0

  require_cmd git

  local source_git_root source_subdir safe_branch existing_worktree
  source_git_root="$(git -C "$ORIGINAL_REPO_ROOT" rev-parse --show-toplevel)"
  source_subdir="."
  if [[ "$ORIGINAL_REPO_ROOT" != "$source_git_root" ]]; then
    source_subdir="${ORIGINAL_REPO_ROOT#"$source_git_root"/}"
  fi

  if ! git -C "$source_git_root" show-ref --verify --quiet "refs/heads/$INTEGRATION_BRANCH"; then
    git -C "$source_git_root" branch "$INTEGRATION_BRANCH" HEAD
  fi

  # Repo moves can leave git worktree metadata pointing at deleted
  # orchestrator paths. Prune before trusting branch_worktree_path.
  git -C "$source_git_root" worktree prune >/dev/null 2>&1 || true

  existing_worktree="$(branch_worktree_path "$source_git_root" "$INTEGRATION_BRANCH")"
  if [[ -n "$existing_worktree" ]]; then
    INTEGRATION_WORKTREE="$existing_worktree"
  else
    if [[ -z "$INTEGRATION_WORKTREE" ]]; then
      safe_branch="$(safe_path_token "$INTEGRATION_BRANCH")"
      INTEGRATION_WORKTREE="$RUN_ROOT/integration/$safe_branch"
    fi
    mkdir -p "$(dirname "$INTEGRATION_WORKTREE")"
    git -C "$source_git_root" worktree add "$INTEGRATION_WORKTREE" "$INTEGRATION_BRANCH" >/dev/null
  fi

  # Clear stale .git/index.lock (e.g. from a crashed prior cherry-pick) so
  # subsequent git operations on the integration worktree don't wedge.
  if [[ -n "$INTEGRATION_WORKTREE" && -d "$INTEGRATION_WORKTREE" ]]; then
    local git_dir lock_age_s
    git_dir="$(git -C "$INTEGRATION_WORKTREE" rev-parse --absolute-git-dir 2>/dev/null || true)"
    if [[ -n "$git_dir" && -f "$git_dir/index.lock" ]]; then
      lock_age_s=$(( $(date +%s) - $(stat -c %Y "$git_dir/index.lock" 2>/dev/null || echo 0) ))
      if (( lock_age_s > 60 )); then
        echo "setup_integration_root: removing stale $git_dir/index.lock (age=${lock_age_s}s)"
        rm -f "$git_dir/index.lock"
        if type log_event >/dev/null 2>&1; then
          log_event "git_lock_cleaned" null "path=$git_dir/index.lock age=$lock_age_s" "cleaned" || true
        fi
      fi
    fi
  fi

  if [[ -n "$(git -C "$INTEGRATION_WORKTREE" status --short)" ]]; then
    echo "ERROR: integration worktree is dirty: $INTEGRATION_WORKTREE" >&2
    echo "Resolve or remove it before running the forever orchestrator." >&2
    exit 1
  fi

  git -C "$INTEGRATION_WORKTREE" reset --hard "$INTEGRATION_BRANCH" >/dev/null

  if [[ "$source_subdir" == "." ]]; then
    REPO_ROOT="$INTEGRATION_WORKTREE"
  else
    REPO_ROOT="$INTEGRATION_WORKTREE/$source_subdir"
  fi
  refresh_repo_paths
}

# Push integration branch to remote
push_integration_branch() {
  if [[ "$AUTO_PUSH" != "1" ]]; then
    return 0
  fi

  log_info "Pushing $INTEGRATION_BRANCH to $REMOTE_NAME"

  if ! git -C "$GIT_ROOT" push "$REMOTE_NAME" "$INTEGRATION_BRANCH"; then
    log_error "Failed to push $INTEGRATION_BRANCH to $REMOTE_NAME"
    return 1
  fi

  log_info "Successfully pushed $INTEGRATION_BRANCH"
  return 0
}

cmd_promote_commit() {
  local target_run="$1"
  local worker_id="$2"
  local target_branch="${3:-$(git -C "$GIT_ROOT" rev-parse --abbrev-ref HEAD)}"
  local prefix commit
  prefix="$(latest_worker_log_prefix "$target_run" "$worker_id")"
  [[ -n "$prefix" ]] || { echo "No logs for run=$target_run worker=$worker_id" >&2; return 1; }
  commit="$(cat "$LOGS_DIR/${prefix}.head" 2>/dev/null || true)"
  [[ -n "$commit" ]] || { echo "No commit head found for $prefix" >&2; return 1; }

  git -C "$GIT_ROOT" checkout "$target_branch" >/dev/null
  git -C "$GIT_ROOT" cherry-pick "$commit"
  echo "promoted commit $commit onto $target_branch"
}

# Build a markdown PR body describing the worker's commit.
#   $1 slug        Task slug ("<phase>__<subphase>__<item>")
#   $2 commit      Full commit hash
#   $3 final_file  Path to the worker's final report (may be absent)
# Writes the body to stdout, capped at $PR_BODY_MAX_BYTES.
generate_pr_body() {
  local slug="$1"
  local commit="$2"
  local final_file="$3"
  local branch="${4:-}"

  # Locate a stderr tail matching the worker's most recent log prefix. The
  # final_file lives alongside its .stderr sibling in $LOGS_DIR.
  local stderr_file=""
  if [[ -n "$final_file" ]]; then
    local base
    base="${final_file%.final.md}"
    if [[ -f "$base.stderr" ]]; then
      stderr_file="$base.stderr"
    elif [[ -f "${final_file}.stderr" ]]; then
      stderr_file="${final_file}.stderr"
    fi
  fi

  local body
  body="$(
    printf '### Autoloop promotion\n\n'
    printf -- '- Slug: `%s`\n' "$slug"
    [[ -n "$branch" ]] && printf -- '- Worker branch: `%s`\n' "$branch"
    printf -- '- Commit: `%s`\n\n' "$commit"
    if [[ -n "$stderr_file" && -f "$stderr_file" ]]; then
      printf '#### Last 40 lines of stderr\n\n'
      printf '```\n'
      tail -n 40 "$stderr_file"
      printf '```\n\n'
    fi
    if [[ -n "$final_file" && -f "$final_file" ]]; then
      printf '#### Final report\n\n'
      printf '```markdown\n'
      cat "$final_file"
      printf '\n```\n'
    fi
  )"

  # GitHub caps PR body at ~65k; keep a safety margin.
  if (( ${#body} > PR_BODY_MAX_BYTES )); then
    local keep=$(( PR_BODY_MAX_BYTES - 32 ))
    (( keep < 1 )) && keep=1
    body="${body:0:$keep}"$'\n[...truncated...]'
  fi

  printf '%s' "$body"
}

# Locate the final report file for a given worker / slug in $LOGS_DIR.
# Prefers the newest matching *.final.md. Prints the absolute path or
# empty on miss.
_latest_worker_final_file() {
  local worker_id="$1"
  [[ -n "${LOGS_DIR:-}" && -d "$LOGS_DIR" ]] || return 0
  local f
  f="$(ls -t "$LOGS_DIR"/*"__worker${worker_id}__"*.final.md 2>/dev/null | head -n1 || true)"
  [[ -n "$f" ]] && printf '%s' "$f"
}

# Run gh auth status and gh repo view. Used by the verify-gh-auth subcommand
# and by the pre-flight check in promote_successful_workers.
# Args: $1 = (optional) repo slug for gh repo view.
# Prints a diagnostic line and returns 0 on full pass, non-zero otherwise.
orchestrator_verify_gh_auth() {
  local repo_slug="${1:-$PR_REPO_SLUG}"
  if ! command -v gh >/dev/null 2>&1; then
    echo "FAIL: gh CLI not on PATH"
    return 1
  fi
  if ! gh auth status >/dev/null 2>&1; then
    echo "FAIL: gh auth status"
    return 1
  fi
  if ! gh repo view "$repo_slug" >/dev/null 2>&1; then
    echo "FAIL: gh repo view $repo_slug"
    return 1
  fi
  echo "PASS: gh auth status + gh repo view $repo_slug"
  return 0
}

# Attempt to open a PR for a single successful worker.
# Returns 0 on success (worker_pr_opened emitted), non-zero on any failure.
# Sets the shared local ${pr_fail_reason} variable to one of
# no_gh_cli|auth_failure|push_failed|pr_create_failed on failure (caller reads).
_try_open_worker_pr() {
  local worker_id="$1"
  local slug="$2"
  local commit="$3"
  local branch="$4"
  local final_file="$5"

  pr_fail_reason=""

  if ! command -v gh >/dev/null 2>&1; then
    pr_fail_reason="no_gh_cli"
    return 1
  fi

  # gh push for the worker branch happens against the worker's worktree
  # (the branch lives there). Fall back to GIT_ROOT if worker_worktree_root
  # is unavailable.
  local push_dir="$GIT_ROOT"
  if type worker_worktree_root >/dev/null 2>&1; then
    local candidate
    candidate="$(worker_worktree_root "$worker_id" 2>/dev/null || true)"
    [[ -n "$candidate" && -d "$candidate" ]] && push_dir="$candidate"
  fi

  if ! git -C "$push_dir" push "${REMOTE_NAME:-origin}" "$branch" >/dev/null 2>&1; then
    pr_fail_reason="push_failed"
    return 1
  fi

  local body pr_url pr_rc
  body="$(generate_pr_body "$slug" "$commit" "$final_file" "$branch")"

  # Try with --label first. If the label doesn't exist on the repo, retry
  # without it (soft fail). Capture stdout (url) and exit code.
  pr_url="$(gh pr create \
    --repo "$PR_REPO_SLUG" \
    --head "$branch" \
    --base "${MAIN_BRANCH:-main}" \
    --title "autoloop: $slug" \
    --body "$body" \
    --label autoloop-bot 2>/dev/null)" || pr_rc=$?

  if [[ -z "${pr_url:-}" ]]; then
    # Retry without the label.
    pr_url="$(gh pr create \
      --repo "$PR_REPO_SLUG" \
      --head "$branch" \
      --base "${MAIN_BRANCH:-main}" \
      --title "autoloop: $slug" \
      --body "$body" 2>/dev/null)" || pr_rc=$?
  fi

  if [[ -z "${pr_url:-}" ]]; then
    pr_fail_reason="pr_create_failed"
    return 1
  fi

  log_event "worker_pr_opened" "$worker_id" "$slug@$pr_url" "opened"
  return 0
}

# Cherry-pick a single worker commit onto the integration worktree.
# Returns 0 on success, 1 otherwise (with cherry-pick aborted).
_cherry_pick_worker_commit() {
  local worker_id="$1"
  local slug="$2"
  local commit="$3"

  echo "worker[$worker_id]: promoting -> $slug ($commit) onto $INTEGRATION_BRANCH"
  if git -C "$GIT_ROOT" cherry-pick -Xtheirs "$commit" >/dev/null; then
    log_event "worker_promoted" "$worker_id" "$slug@$commit" "promoted"
    return 0
  fi

  git -C "$GIT_ROOT" cherry-pick --abort >/dev/null 2>&1 || true
  echo "worker[$worker_id]: promotion failed -> $slug ($commit)" >&2
  log_event "worker_promotion_failed" "$worker_id" "$slug@$commit" "cherry_pick_failed"
  return 1
}

promote_successful_workers() {
  local workers="$1"
  promotion_enabled || return 0

  local rc=0 promoted=0 i state_json status commit slug branch
  local pr_mode_broken=0
  local pr_fail_reason=""
  local final_file

  if [[ -n "$(git -C "$GIT_ROOT" status --short)" ]]; then
    echo "ERROR: integration branch worktree is dirty before promotion: $GIT_ROOT" >&2
    return 1
  fi

  # PR mode pre-flight: verify gh auth once. If it fails, downgrade to
  # cherry-pick for the whole cycle and emit a single
  # pr_mode_unavailable event with stderr detail.
  if [[ "$PROMOTION_MODE" == "pr" ]]; then
    if ! command -v gh >/dev/null 2>&1; then
      pr_mode_broken=1
      log_event "pr_mode_unavailable" null "no_gh_cli" "unavailable"
    else
      local auth_err
      auth_err="$(gh auth status 2>&1 >/dev/null || true)"
      if ! gh auth status >/dev/null 2>&1; then
        pr_mode_broken=1
        # Keep the detail compact — first line of stderr is enough.
        local auth_detail
        auth_detail="$(printf '%s' "$auth_err" | head -n1)"
        log_event "pr_mode_unavailable" null "${auth_detail:-auth_failure}" "unavailable"
      fi
    fi
  else
    # Non-pr mode: treat as "already broken" so we take the cherry-pick path.
    pr_mode_broken=1
  fi

  for (( i = 1; i <= workers; i++ )); do
    state_json="$(load_worker_state "$i" 2>/dev/null || true)"
    [[ -n "$state_json" ]] || continue

    status="$(jq -r '.status // ""' <<<"$state_json")"
    [[ "$status" == "success" ]] || continue

    commit="$(jq -r '.commit // ""' <<<"$state_json")"
    slug="$(jq -r '.slug // ""' <<<"$state_json")"
    if [[ -z "$commit" || "$commit" == "null" ]]; then
      echo "worker[$i]: success state missing commit; cannot promote" >&2
      log_event "worker_promotion_failed" "$i" "$slug" "missing_commit"
      rc=1
      continue
    fi

    if git -C "$GIT_ROOT" merge-base --is-ancestor "$commit" HEAD 2>/dev/null; then
      echo "worker[$i]: already promoted -> $slug ($commit)"
      log_event "worker_promoted" "$i" "$slug@$commit" "already_promoted"
      continue
    fi

    # PR path: try once per successful worker unless the mode is broken.
    if [[ "$PROMOTION_MODE" == "pr" && "$pr_mode_broken" == "0" ]]; then
      branch=""
      if type worker_branch_name >/dev/null 2>&1; then
        branch="$(worker_branch_name "$i")"
      fi
      final_file="$(_latest_worker_final_file "$i" || true)"

      if _try_open_worker_pr "$i" "$slug" "$commit" "$branch" "$final_file"; then
        promoted=$((promoted + 1))
        continue
      fi

      # PR flow failed — log fallback and flip the cycle-wide flag so the
      # rest of the cycle goes straight to cherry-pick.
      echo "worker[$i]: PR flow failed ($pr_fail_reason); falling back to cherry-pick"
      log_event "worker_pr_fallback" "$i" "$slug:$pr_fail_reason" "fallback"
      pr_mode_broken=1
      # Fall through to cherry-pick path below.
    fi

    if _cherry_pick_worker_commit "$i" "$slug" "$commit"; then
      promoted=$((promoted + 1))
    else
      rc=1
    fi
  done

  if (( promoted > 0 )); then
    echo "Promoted worker commits: $promoted"
    echo "Integration head: $(git -C "$GIT_ROOT" rev-parse --short HEAD)"

    # Push integration branch to remote if AUTO_PUSH is enabled. This only
    # matters for cherry-picks that landed locally; PRs already pushed the
    # worker branch.
    if [[ "$AUTO_PUSH" == "1" ]]; then
      push_integration_branch || rc=1
    fi
  fi

  export PROMOTED_LAST_CYCLE="$promoted"
  return "$rc"
}
