package builderloop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PullRequestIntakeOptions struct {
	Runner         Runner
	RepoRoot       string
	RunRoot        string
	RunID          string
	Repo           string
	Remote         string
	BaseBranch     string
	ConflictAction string
}

type PullRequestIntakeSummary struct {
	Listed  int
	Merged  int
	Closed  int
	Failed  int
	Skipped int
}

const (
	PRConflictActionClose = "close"
	PRConflictActionSkip  = "skip"

	prDirtyCloseComment = "Closed by Gormes PR intake: mergeStateStatus=DIRTY means this PR has merge conflicts and cannot be merged automatically at cycle start."
)

type pullRequestInfo struct {
	Number           int    `json:"number"`
	Title            string `json:"title"`
	IsDraft          bool   `json:"isDraft"`
	MergeStateStatus string `json:"mergeStateStatus"`
	HeadRefName      string `json:"headRefName"`
	URL              string `json:"url"`
}

func MergeOpenPullRequests(ctx context.Context, opts PullRequestIntakeOptions) (PullRequestIntakeSummary, error) {
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if !repoHasGit(opts.RepoRoot) {
		return PullRequestIntakeSummary{}, nil
	}

	if err := appendPRIntakeEvent(opts, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  opts.RunID,
		Event:  "pr_intake_started",
		Status: "started",
	}); err != nil {
		return PullRequestIntakeSummary{}, err
	}

	repo := strings.TrimSpace(opts.Repo)
	if repo == "" {
		repo = detectGitHubRepo(ctx, opts)
	}

	list := opts.Runner.Run(ctx, Command{
		Name: "gh",
		Args: prListArgs(repo),
		Dir:  opts.RepoRoot,
	})
	if list.Err != nil {
		_ = appendPRIntakeEvent(opts, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  opts.RunID,
			Event:  "pr_intake_failed",
			Status: "list_failed",
			Detail: strings.TrimSpace(list.Stderr),
		})
		return PullRequestIntakeSummary{}, fmt.Errorf("list open pull requests: %w: %s", list.Err, strings.TrimSpace(list.Stderr))
	}

	var prs []pullRequestInfo
	if err := json.Unmarshal([]byte(list.Stdout), &prs); err != nil {
		_ = appendPRIntakeEvent(opts, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  opts.RunID,
			Event:  "pr_intake_failed",
			Status: "list_parse_failed",
			Detail: err.Error(),
		})
		return PullRequestIntakeSummary{}, fmt.Errorf("parse open pull requests: %w", err)
	}

	summary := PullRequestIntakeSummary{Listed: len(prs)}
	sort.Slice(prs, func(i, j int) bool { return prs[i].Number < prs[j].Number })

	var ready []pullRequestInfo
	conflictAction := prConflictAction(opts)
	for _, pr := range prs {
		if pr.IsDraft {
			summary.Skipped++
			if err := appendPRIntakeEvent(opts, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  opts.RunID,
				Event:  "pr_intake_skipped",
				Status: "draft",
				Detail: prDetail(pr),
			}); err != nil {
				return summary, err
			}
			continue
		}
		if prIsDirty(pr) {
			switch conflictAction {
			case PRConflictActionClose:
				closeResult := opts.Runner.Run(ctx, Command{
					Name: "gh",
					Args: prCloseArgs(pr.Number, repo),
					Dir:  opts.RepoRoot,
				})
				if closeResult.Err != nil {
					summary.Failed++
					_ = appendPRIntakeEvent(opts, LedgerEvent{
						TS:     time.Now().UTC(),
						RunID:  opts.RunID,
						Event:  "pr_intake_failed",
						Status: "close_failed",
						Detail: prDetail(pr) + " " + strings.TrimSpace(closeResult.Stderr),
					})
					continue
				}
				summary.Closed++
				if err := appendPRIntakeEvent(opts, LedgerEvent{
					TS:     time.Now().UTC(),
					RunID:  opts.RunID,
					Event:  "pr_intake_closed",
					Status: "dirty",
					Detail: prDetail(pr),
				}); err != nil {
					return summary, err
				}
			case PRConflictActionSkip:
				summary.Skipped++
				if err := appendPRIntakeEvent(opts, LedgerEvent{
					TS:     time.Now().UTC(),
					RunID:  opts.RunID,
					Event:  "pr_intake_skipped",
					Status: "dirty",
					Detail: prDetail(pr),
				}); err != nil {
					return summary, err
				}
			}
			continue
		}
		ready = append(ready, pr)
	}

	for _, pr := range ready {
		merge := opts.Runner.Run(ctx, Command{
			Name: "gh",
			Args: prMergeArgs(pr.Number, repo),
			Dir:  opts.RepoRoot,
		})
		if merge.Err != nil {
			summary.Failed++
			_ = appendPRIntakeEvent(opts, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  opts.RunID,
				Event:  "pr_intake_failed",
				Status: "merge_failed",
				Detail: prDetail(pr) + " " + strings.TrimSpace(merge.Stderr),
			})
			continue
		}

		summary.Merged++
		if err := appendPRIntakeEvent(opts, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  opts.RunID,
			Event:  "pr_intake_merged",
			Status: "merged",
			Detail: prDetail(pr),
		}); err != nil {
			return summary, err
		}
	}

	if summary.Merged > 0 {
		if err := syncMergedPullRequests(ctx, opts, summary); err != nil {
			return summary, err
		}
	}

	return summary, appendPRIntakeEvent(opts, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  opts.RunID,
		Event:  "pr_intake_completed",
		Status: "completed",
		Detail: fmt.Sprintf("listed=%d merged=%d closed=%d failed=%d skipped=%d", summary.Listed, summary.Merged, summary.Closed, summary.Failed, summary.Skipped),
	})
}

func detectGitHubRepo(ctx context.Context, opts PullRequestIntakeOptions) string {
	remote := prIntakeRemote(opts)
	result := opts.Runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"remote", "get-url", remote},
		Dir:  opts.RepoRoot,
	})
	if result.Err != nil {
		return ""
	}
	return parseGitHubRepo(result.Stdout)
}

func parseGitHubRepo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
		return repoNameWithHost(parsed.Hostname(), parsed.Path)
	}

	if at := strings.Index(raw, "@"); at >= 0 {
		rest := raw[at+1:]
		if colon := strings.Index(rest, ":"); colon >= 0 {
			return repoNameWithHost(rest[:colon], rest[colon+1:])
		}
	}

	return trimGitSuffix(strings.Trim(raw, "/"))
}

func repoNameWithHost(host, path string) string {
	host = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(host, ".")))
	path = trimGitSuffix(strings.Trim(path, "/"))
	if path == "" {
		return ""
	}
	if host == "" || host == "github.com" {
		return path
	}
	return host + "/" + path
}

func trimGitSuffix(value string) string {
	return strings.TrimSuffix(value, ".git")
}

func prListArgs(repo string) []string {
	args := []string{"pr", "list"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	return append(args, "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url")
}

func prMergeArgs(number int, repo string) []string {
	args := []string{"pr", "merge", fmt.Sprint(number)}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	return append(args, "--merge", "--delete-branch", "--admin")
}

func prCloseArgs(number int, repo string) []string {
	args := []string{"pr", "close", fmt.Sprint(number)}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	return append(args, "--delete-branch", "--comment", prDirtyCloseComment)
}

func prIsDirty(pr pullRequestInfo) bool {
	return strings.EqualFold(strings.TrimSpace(pr.MergeStateStatus), "DIRTY")
}

func prConflictAction(opts PullRequestIntakeOptions) string {
	switch strings.ToLower(strings.TrimSpace(opts.ConflictAction)) {
	case "", PRConflictActionClose:
		return PRConflictActionClose
	case PRConflictActionSkip:
		return PRConflictActionSkip
	default:
		return PRConflictActionClose
	}
}

func syncMergedPullRequests(ctx context.Context, opts PullRequestIntakeOptions, summary PullRequestIntakeSummary) error {
	remote := prIntakeRemote(opts)
	base := prIntakeBaseBranch(opts)

	fetch := opts.Runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"fetch", remote, base},
		Dir:  opts.RepoRoot,
	})
	if fetch.Err != nil {
		_ = appendPRIntakeEvent(opts, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  opts.RunID,
			Event:  "pr_intake_failed",
			Status: "sync_fetch_failed",
			Detail: prSyncDetail(summary, fetch),
		})
		return fmt.Errorf("fetch after merging pull requests: %w: %s", fetch.Err, strings.TrimSpace(fetch.Stderr))
	}

	fastForward := opts.Runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"merge", "--ff-only", "FETCH_HEAD"},
		Dir:  opts.RepoRoot,
	})
	if fastForward.Err == nil {
		return appendPRIntakeEvent(opts, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  opts.RunID,
			Event:  "pr_intake_synced",
			Status: "fast_forward",
			Detail: prSyncDetail(summary, fastForward),
		})
	}

	merge := opts.Runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"merge", "--no-edit", "FETCH_HEAD"},
		Dir:  opts.RepoRoot,
	})
	if merge.Err == nil {
		return appendPRIntakeEvent(opts, LedgerEvent{
			TS:     time.Now().UTC(),
			RunID:  opts.RunID,
			Event:  "pr_intake_synced",
			Status: "merged",
			Detail: prSyncDetail(summary, merge),
		})
	}

	_ = opts.Runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"merge", "--abort"},
		Dir:  opts.RepoRoot,
	})
	_ = appendPRIntakeEvent(opts, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  opts.RunID,
		Event:  "pr_intake_failed",
		Status: "sync_merge_failed",
		Detail: prSyncDetail(summary, merge),
	})
	return fmt.Errorf("sync after merging pull requests: %w: %s", merge.Err, strings.TrimSpace(merge.Stderr))
}

func prIntakeRemote(opts PullRequestIntakeOptions) string {
	if remote := strings.TrimSpace(opts.Remote); remote != "" {
		return remote
	}
	return "origin"
}

func prIntakeBaseBranch(opts PullRequestIntakeOptions) string {
	if branch := strings.TrimSpace(opts.BaseBranch); branch != "" {
		return branch
	}
	return "main"
}

func prSyncDetail(summary PullRequestIntakeSummary, result Result) string {
	detail := fmt.Sprintf("listed=%d merged=%d closed=%d failed=%d skipped=%d", summary.Listed, summary.Merged, summary.Closed, summary.Failed, summary.Skipped)
	output := strings.TrimSpace(result.Stderr)
	if output == "" {
		output = strings.TrimSpace(result.Stdout)
	}
	if output == "" {
		return detail
	}
	return detail + " " + output
}

func appendPRIntakeEvent(opts PullRequestIntakeOptions, event LedgerEvent) error {
	if opts.RunRoot == "" {
		return nil
	}
	return AppendLedgerEvent(filepath.Join(opts.RunRoot, "state", "runs.jsonl"), event)
}

func prDetail(pr pullRequestInfo) string {
	parts := []string{fmt.Sprintf("#%d", pr.Number)}
	if pr.Title != "" {
		parts = append(parts, pr.Title)
	}
	if pr.HeadRefName != "" {
		parts = append(parts, "head="+pr.HeadRefName)
	}
	if pr.MergeStateStatus != "" {
		parts = append(parts, "state="+pr.MergeStateStatus)
	}
	if pr.URL != "" {
		parts = append(parts, pr.URL)
	}
	return strings.Join(parts, " ")
}
