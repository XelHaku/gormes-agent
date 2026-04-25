package autoloop

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PullRequestIntakeOptions struct {
	Runner   Runner
	RepoRoot string
	RunRoot  string
	RunID    string
}

type PullRequestIntakeSummary struct {
	Listed  int
	Merged  int
	Failed  int
	Skipped int
}

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

	list := opts.Runner.Run(ctx, Command{
		Name: "gh",
		Args: []string{"pr", "list", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"},
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
		ready = append(ready, pr)
	}

	for _, pr := range ready {
		merge := opts.Runner.Run(ctx, Command{
			Name: "gh",
			Args: []string{"pr", "merge", fmt.Sprint(pr.Number), "--merge", "--delete-branch", "--admin"},
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
		pull := opts.Runner.Run(ctx, Command{
			Name: "git",
			Args: []string{"pull", "--ff-only"},
			Dir:  opts.RepoRoot,
		})
		if pull.Err != nil {
			_ = appendPRIntakeEvent(opts, LedgerEvent{
				TS:     time.Now().UTC(),
				RunID:  opts.RunID,
				Event:  "pr_intake_failed",
				Status: "pull_failed",
				Detail: fmt.Sprintf("listed=%d merged=%d failed=%d skipped=%d %s", summary.Listed, summary.Merged, summary.Failed, summary.Skipped, strings.TrimSpace(pull.Stderr)),
			})
			return summary, fmt.Errorf("pull after merging pull requests: %w: %s", pull.Err, strings.TrimSpace(pull.Stderr))
		}
	}

	return summary, appendPRIntakeEvent(opts, LedgerEvent{
		TS:     time.Now().UTC(),
		RunID:  opts.RunID,
		Event:  "pr_intake_completed",
		Status: "completed",
		Detail: fmt.Sprintf("listed=%d merged=%d failed=%d skipped=%d", summary.Listed, summary.Merged, summary.Failed, summary.Skipped),
	})
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
