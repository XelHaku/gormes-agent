package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// branchForkTimeout caps how long /branch waits on the injected helper.
// Forks are local SQLite + bbolt writes — five seconds is generous; if they
// outrun this budget the TUI surfaces a status line instead of blocking the
// editor on a failed disk.
const branchForkTimeout = 5 * time.Second

// BranchRequest is the input to SessionBranchFunc. ParentSessionID is the
// session whose persisted history the caller wants forked into a fresh
// child; Title is the operator-supplied label (empty if /branch was typed
// without a name); HistoryCount is the in-memory render-frame turn count
// the TUI saw at fork time, included so the helper can surface a
// `branch: switched (N turns)` status without re-reading the store.
type BranchRequest struct {
	ParentSessionID string
	Title           string
	HistoryCount    int
}

// BranchResult is the helper's response. SessionID is the freshly minted
// child id the TUI must switch to; ParentSessionID echoes the parent so
// callers can audit the helper observed the right one; TranscriptCopied
// reports how many parent turns were duplicated under the child.
type BranchResult struct {
	SessionID        string
	ParentSessionID  string
	Title            string
	TranscriptCopied int
}

// SessionBranchFunc is the injection point for the TUI /branch command.
// cmd/gormes binds the production implementation that calls
// session.Fork+transcript.ForkTurns; tests wire fakes.
type SessionBranchFunc func(ctx context.Context, req BranchRequest) (BranchResult, error)

// branchSlashHandler implements /branch. The handler MUST consume the input
// (Handled=true) on every error branch so the slash text never falls through
// to kernel.Submit; the parent session is left active on every failure so
// degraded-mode operators retain their existing context.
func branchSlashHandler(input string, model *Model) SlashResult {
	if model == nil {
		return SlashResult{Handled: true, StatusMessage: "branch: store unavailable"}
	}
	if len(model.frame.History) == 0 {
		return SlashResult{Handled: true, StatusMessage: "branch: no conversation"}
	}
	if model.sessionBranch == nil {
		return SlashResult{Handled: true, StatusMessage: "branch: store unavailable"}
	}
	parent := strings.TrimSpace(model.SessionID())
	if parent == "" {
		return SlashResult{Handled: true, StatusMessage: "branch: no active session"}
	}

	title := branchTitleFromInput(input)

	ctx, cancel := context.WithTimeout(context.Background(), branchForkTimeout)
	defer cancel()
	res, err := model.sessionBranch(ctx, BranchRequest{
		ParentSessionID: parent,
		Title:           title,
		HistoryCount:    len(model.frame.History),
	})
	if err != nil {
		return SlashResult{Handled: true, StatusMessage: fmt.Sprintf("branch: fork failed: %v", err)}
	}

	model.sessionID = res.SessionID
	model.inFlight = false
	model.frame.DraftText = ""
	return SlashResult{
		Handled:       true,
		StatusMessage: branchSuccessStatus(res),
	}
}

func branchTitleFromInput(input string) string {
	trimmed := strings.TrimSpace(input)
	idx := strings.IndexAny(trimmed, " \t")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(trimmed[idx+1:])
}

func branchSuccessStatus(res BranchResult) string {
	if res.Title != "" {
		return fmt.Sprintf("branch: switched to %s (%q, %d turns)", res.SessionID, res.Title, res.TranscriptCopied)
	}
	return fmt.Sprintf("branch: switched to %s (%d turns)", res.SessionID, res.TranscriptCopied)
}
