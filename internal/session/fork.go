package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MetadataWriter is the subset of *BoltMap / *MemMap used by Fork to persist
// the child's lineage row. Defining it as an interface keeps the orchestration
// independently testable from the backing store.
type MetadataWriter interface {
	PutMetadata(ctx context.Context, meta Metadata) error
}

// TurnCopier transfers persisted turns from a parent session to a freshly
// minted child. The transcript package owns the SQL-level implementation;
// session.Fork stays decoupled so it can be exercised with a fake copier.
type TurnCopier interface {
	CopyTurns(ctx context.Context, parentSessionID, childSessionID string) (int, error)
}

// ForkRequest is the input to Fork. All three IDs are trimmed before use;
// empty parent/child or self-parent is rejected. Title is opaque metadata
// the caller surfaces (TUI status line, future session browser).
type ForkRequest struct {
	ParentSessionID string
	ChildSessionID  string
	Title           string
}

// ForkResult mirrors the BranchResult contract used by TUI and other
// frontends. TranscriptCopied reflects what the TurnCopier reported, not a
// re-count of stored rows; callers wanting end-to-end verification should
// query the transcript store directly.
type ForkResult struct {
	SessionID        string
	ParentSessionID  string
	Title            string
	TranscriptCopied int
}

// Fork copies persisted turns from parent to child via copier and writes
// session.Metadata{ParentSessionID, LineageKindFork} for the child. Order
// matters: if the copier fails we never write the lineage row, so a
// half-copied child is never observable to ResolveLineageTip or the index
// mirror. Callers MAY pass copier=nil to skip the copy step entirely
// (used for tests that only care about lineage metadata).
func Fork(ctx context.Context, m MetadataWriter, copier TurnCopier, req ForkRequest) (ForkResult, error) {
	parent := strings.TrimSpace(req.ParentSessionID)
	child := strings.TrimSpace(req.ChildSessionID)
	title := strings.TrimSpace(req.Title)

	if parent == "" {
		return ForkResult{}, errors.New("session: Fork parent_session_id required")
	}
	if child == "" {
		return ForkResult{}, errors.New("session: Fork child_session_id required")
	}
	if parent == child {
		return ForkResult{}, fmt.Errorf("%w: %s cannot fork to itself", ErrLineageLoop, parent)
	}
	if m == nil {
		return ForkResult{}, errors.New("session: Fork metadata writer required")
	}

	var copied int
	if copier != nil {
		n, err := copier.CopyTurns(ctx, parent, child)
		if err != nil {
			return ForkResult{}, fmt.Errorf("session: Fork copy turns parent=%s child=%s: %w", parent, child, err)
		}
		copied = n
	}

	meta := Metadata{
		SessionID:       child,
		ParentSessionID: parent,
		LineageKind:     LineageKindFork,
		UpdatedAt:       time.Now().Unix(),
	}
	if err := m.PutMetadata(ctx, meta); err != nil {
		return ForkResult{}, fmt.Errorf("session: Fork put metadata for %s: %w", child, err)
	}

	return ForkResult{
		SessionID:        child,
		ParentSessionID:  parent,
		Title:            title,
		TranscriptCopied: copied,
	}, nil
}
