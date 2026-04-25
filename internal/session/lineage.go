package session

import (
	"errors"
	"fmt"
	"sort"
)

const (
	// LineageKindPrimary marks a root session with no parent.
	LineageKindPrimary = "primary"
	// LineageKindCompression marks a continuation produced by context compression.
	LineageKindCompression = "compression"
	// LineageKindFork marks a branch/fork child that should not be treated as a live continuation.
	LineageKindFork = "fork"
)

// ErrLineageLoop reports self-parenting or a parent chain that loops back to
// the session being written.
var ErrLineageLoop = errors.New("session: lineage loop")

// ErrLineageConflict reports an attempt to rewrite already-recorded lineage.
var ErrLineageConflict = errors.New("session: lineage metadata conflict")

const (
	LineageStatusOK      = "ok"
	LineageStatusMissing = "missing"
	LineageStatusOrphan  = "orphan"
	LineageStatusLoop    = "loop"
)

// LineageAuditEntry is the read model rendered by the operator session mirror.
type LineageAuditEntry struct {
	Metadata Metadata
	Children []string
	Status   string
}

// LineageResolution describes a read-only lineage-tip walk. It does not create
// compression children or rewrite any ancestor metadata.
type LineageResolution struct {
	RequestedSessionID string
	LiveSessionID      string
	Path               []string
	Status             string
}

func effectiveLineageKind(meta Metadata) string {
	if meta.LineageKind == "" {
		return LineageKindPrimary
	}
	return meta.LineageKind
}

func finalizeMetadata(meta Metadata) Metadata {
	meta = normalizeMetadata(meta)
	meta.LineageKind = effectiveLineageKind(meta)
	return meta
}

func validateLineageKind(kind string) error {
	switch kind {
	case "", LineageKindPrimary, LineageKindCompression, LineageKindFork:
		return nil
	default:
		return fmt.Errorf("session: unsupported lineage_kind %q", kind)
	}
}

func validateMetadataIdentity(meta Metadata) error {
	if meta.SessionID == "" {
		return errors.New("session: metadata session_id is required")
	}
	if (meta.Source == "") != (meta.ChatID == "") {
		return errors.New("session: metadata source and chat_id must both be set or both be empty")
	}
	if meta.UserID != "" && (meta.Source == "" || meta.ChatID == "") {
		return errors.New("session: metadata user_id requires source and chat_id")
	}
	if meta.ParentSessionID != "" && meta.ParentSessionID == meta.SessionID {
		return fmt.Errorf("%w: %s cannot parent itself", ErrLineageLoop, meta.SessionID)
	}
	return validateLineageKind(meta.LineageKind)
}

func validateMetadata(meta Metadata) error {
	if err := validateMetadataIdentity(meta); err != nil {
		return err
	}
	kind := effectiveLineageKind(meta)
	if meta.ParentSessionID == "" && kind != LineageKindPrimary {
		return errors.New("session: non-primary lineage_kind requires parent_session_id")
	}
	if meta.ParentSessionID != "" && kind == LineageKindPrimary {
		return errors.New("session: parent_session_id requires compression or fork lineage_kind")
	}
	return nil
}

func detectLineageLoop(sessionID, parentSessionID string, lookup func(string) (Metadata, bool, error)) error {
	if parentSessionID == "" {
		return nil
	}
	seen := map[string]struct{}{sessionID: {}}
	for current := parentSessionID; current != ""; {
		if _, ok := seen[current]; ok {
			return fmt.Errorf("%w: %s reaches %s", ErrLineageLoop, sessionID, current)
		}
		seen[current] = struct{}{}

		parent, ok, err := lookup(current)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		current = parent.ParentSessionID
	}
	return nil
}

func buildLineageAudit(items []Metadata) []LineageAuditEntry {
	byID := make(map[string]Metadata, len(items))
	children := make(map[string][]string, len(items))
	for _, item := range items {
		item = finalizeMetadata(item)
		byID[item.SessionID] = item
		if item.ParentSessionID != "" {
			children[item.ParentSessionID] = append(children[item.ParentSessionID], item.SessionID)
		}
	}

	out := make([]LineageAuditEntry, 0, len(byID))
	for _, item := range byID {
		childIDs := append([]string(nil), children[item.SessionID]...)
		sort.Strings(childIDs)
		out = append(out, LineageAuditEntry{
			Metadata: item,
			Children: childIDs,
			Status:   lineageAuditStatus(item, byID),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.SessionID < out[j].Metadata.SessionID
	})
	return out
}

func lineageAuditStatus(meta Metadata, byID map[string]Metadata) string {
	meta = finalizeMetadata(meta)
	seen := map[string]struct{}{meta.SessionID: {}}
	for current := meta.ParentSessionID; current != ""; {
		if _, ok := seen[current]; ok {
			return LineageStatusLoop
		}
		seen[current] = struct{}{}

		parent, ok := byID[current]
		if !ok {
			return LineageStatusOrphan
		}
		current = parent.ParentSessionID
	}
	return LineageStatusOK
}

func resolveLineageTipFromMetadata(sessionID string, items []Metadata) LineageResolution {
	res := LineageResolution{
		RequestedSessionID: sessionID,
		LiveSessionID:      sessionID,
		Path:               []string{sessionID},
		Status:             LineageStatusOK,
	}
	if sessionID == "" {
		res.Path = nil
		res.Status = LineageStatusMissing
		return res
	}

	byID := make(map[string]Metadata, len(items))
	children := make(map[string][]Metadata, len(items))
	for _, item := range items {
		item = finalizeMetadata(item)
		byID[item.SessionID] = item
		if item.ParentSessionID != "" && item.LineageKind == LineageKindCompression {
			children[item.ParentSessionID] = append(children[item.ParentSessionID], item)
		}
	}
	if _, ok := byID[sessionID]; !ok {
		res.Status = LineageStatusMissing
		return res
	}

	seen := map[string]struct{}{sessionID: {}}
	current := sessionID
	for {
		next, ok := newestLineageChild(children[current])
		if !ok {
			res.LiveSessionID = current
			return res
		}
		if _, loop := seen[next.SessionID]; loop {
			res.LiveSessionID = current
			res.Status = LineageStatusLoop
			return res
		}
		seen[next.SessionID] = struct{}{}
		res.Path = append(res.Path, next.SessionID)
		current = next.SessionID
	}
}

func newestLineageChild(children []Metadata) (Metadata, bool) {
	if len(children) == 0 {
		return Metadata{}, false
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].UpdatedAt != children[j].UpdatedAt {
			return children[i].UpdatedAt > children[j].UpdatedAt
		}
		return children[i].SessionID > children[j].SessionID
	})
	return children[0], true
}
