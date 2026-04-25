package goncho

import (
	"context"

	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

// Config controls the minimal Goncho service defaults for a runtime.
type Config struct {
	WorkspaceID      string
	ObserverPeerID   string
	RecentMessages   int
	SessionDirectory SessionDirectory
}

// SessionDirectory exposes the canonical user->session metadata seam needed
// for user-scoped cross-chat search.
type SessionDirectory interface {
	ListMetadataByUserID(ctx context.Context, userID string) ([]session.Metadata, error)
}

// ProfileResult is the external shape used by profile reads and updates.
type ProfileResult struct {
	WorkspaceID    string   `json:"workspace_id"`
	Peer           string   `json:"peer"`
	Target         string   `json:"target,omitempty"`
	ObserverPeerID string   `json:"observer_peer_id,omitempty"`
	ObservedPeerID string   `json:"observed_peer_id,omitempty"`
	Card           []string `json:"card"`
}

// ConcludeParams controls manual conclusion writes and deletes.
type ConcludeParams struct {
	Peer       string `json:"peer"`
	Conclusion string `json:"conclusion,omitempty"`
	DeleteID   int64  `json:"delete_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
}

// ConcludeResult is the stable JSON shape for honcho_conclude.
type ConcludeResult struct {
	WorkspaceID string `json:"workspace_id"`
	Peer        string `json:"peer"`
	ID          int64  `json:"id,omitempty"`
	Status      string `json:"status"`
	Deleted     bool   `json:"deleted,omitempty"`
}

// SearchParams controls retrieval for honcho_search.
type SearchParams struct {
	Peer       string   `json:"peer"`
	Query      string   `json:"query"`
	MaxTokens  int      `json:"max_tokens,omitempty"`
	SessionKey string   `json:"session_key,omitempty"`
	Scope      string   `json:"scope,omitempty"`
	Sources    []string `json:"sources,omitempty"`
}

// SearchHit is one result entry returned by search.
type SearchHit struct {
	ID           int64  `json:"id,omitempty"`
	Source       string `json:"source"`
	OriginSource string `json:"origin_source,omitempty"`
	Content      string `json:"content"`
	SessionKey   string `json:"session_key,omitempty"`
}

// SearchResultSet is the stable JSON shape for honcho_search.
type SearchResultSet struct {
	WorkspaceID string      `json:"workspace_id"`
	Peer        string      `json:"peer"`
	Query       string      `json:"query"`
	Results     []SearchHit `json:"results"`
}

// ContextParams controls honcho_context reads.
type ContextParams struct {
	Peer                string   `json:"peer"`
	Query               string   `json:"query,omitempty"`
	MaxTokens           int      `json:"max_tokens,omitempty"`
	SessionKey          string   `json:"session_key,omitempty"`
	Scope               string   `json:"scope,omitempty"`
	Sources             []string `json:"sources,omitempty"`
	PeerTarget          string   `json:"peer_target,omitempty"`
	PeerPerspective     string   `json:"peer_perspective,omitempty"`
	LimitToSession      *bool    `json:"limit_to_session,omitempty"`
	SearchTopK          *int     `json:"search_top_k,omitempty"`
	SearchMaxDistance   *float64 `json:"search_max_distance,omitempty"`
	IncludeMostFrequent *bool    `json:"include_most_frequent,omitempty"`
	MaxConclusions      *int     `json:"max_conclusions,omitempty"`
}

// MessageSlice is one recent message excerpt included in context responses.
type MessageSlice struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ContextUnavailableEvidence names a requested context capability that Goncho
// accepted but cannot yet fulfill with the current local storage model.
type ContextUnavailableEvidence struct {
	Field      string `json:"field"`
	Capability string `json:"capability"`
	Reason     string `json:"reason"`
}

// ContextResult is the stable JSON shape for honcho_context.
type ContextResult struct {
	WorkspaceID    string                       `json:"workspace_id"`
	Peer           string                       `json:"peer"`
	ObserverPeerID string                       `json:"observer_peer_id,omitempty"`
	ObservedPeerID string                       `json:"observed_peer_id,omitempty"`
	SessionKey     string                       `json:"session_key,omitempty"`
	PeerCard       []string                     `json:"peer_card"`
	Representation string                       `json:"representation"`
	Summary        string                       `json:"summary,omitempty"`
	Conclusions    []string                     `json:"conclusions,omitempty"`
	RecentMessages []MessageSlice               `json:"recent_messages,omitempty"`
	Unavailable    []ContextUnavailableEvidence `json:"unavailable,omitempty"`
}
