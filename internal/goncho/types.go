package goncho

import (
	"context"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

// Config controls the minimal Goncho service defaults for a runtime.
type Config struct {
	Enabled                      bool
	WorkspaceID                  string
	ObserverPeerID               string
	RecentMessages               int
	MaxMessageSize               int
	MaxFileSize                  int
	GetContextMaxTokens          int
	ReasoningEnabled             bool
	PeerCardEnabled              bool
	SummaryEnabled               bool
	DreamEnabled                 bool
	DeriverWorkers               int
	RepresentationBatchMaxTokens int
	DialecticDefaultLevel        DialecticLevel
	SessionDirectory             SessionDirectory
}

type DialecticLevel string

const (
	DialecticLevelMinimal DialecticLevel = "minimal"
	DialecticLevelLow     DialecticLevel = "low"
	DialecticLevelMedium  DialecticLevel = "medium"
	DialecticLevelHigh    DialecticLevel = "high"
	DialecticLevelMax     DialecticLevel = "max"
)

const (
	DefaultRecentMessages               = 4
	DefaultMaxMessageSize               = 25_000
	DefaultMaxFileSize                  = 5_242_880
	DefaultGetContextMaxTokens          = 100_000
	DefaultDeriverWorkers               = 1
	DefaultRepresentationBatchMaxTokens = 1024
)

// Effective fills the Go-native Goncho defaults used when older callers still
// construct Config directly instead of going through internal/config.
func (c Config) Effective() Config {
	out := c
	out.Enabled = true
	if strings.TrimSpace(out.WorkspaceID) == "" {
		out.WorkspaceID = DefaultWorkspaceID
	}
	if strings.TrimSpace(out.ObserverPeerID) == "" {
		out.ObserverPeerID = DefaultObserverPeerID
	}
	if out.RecentMessages <= 0 {
		out.RecentMessages = DefaultRecentMessages
	}
	if out.MaxMessageSize <= 0 {
		out.MaxMessageSize = DefaultMaxMessageSize
	}
	if out.MaxFileSize <= 0 {
		out.MaxFileSize = DefaultMaxFileSize
	}
	if out.GetContextMaxTokens <= 0 {
		out.GetContextMaxTokens = DefaultGetContextMaxTokens
	}
	out.ReasoningEnabled = true
	out.PeerCardEnabled = true
	out.SummaryEnabled = true
	if out.DeriverWorkers <= 0 {
		out.DeriverWorkers = DefaultDeriverWorkers
	}
	if out.RepresentationBatchMaxTokens <= 0 {
		out.RepresentationBatchMaxTokens = DefaultRepresentationBatchMaxTokens
	}
	if !ValidDialecticLevel(string(out.DialecticDefaultLevel)) {
		out.DialecticDefaultLevel = DialecticLevelLow
	}
	return out
}

func ValidDialecticLevel(level string) bool {
	switch DialecticLevel(strings.ToLower(strings.TrimSpace(level))) {
	case DialecticLevelMinimal, DialecticLevelLow, DialecticLevelMedium, DialecticLevelHigh, DialecticLevelMax:
		return true
	default:
		return false
	}
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
	Peer       string         `json:"peer"`
	Query      string         `json:"query"`
	MaxTokens  int            `json:"max_tokens,omitempty"`
	SessionKey string         `json:"session_key,omitempty"`
	Scope      string         `json:"scope,omitempty"`
	Sources    []string       `json:"sources,omitempty"`
	Filters    map[string]any `json:"filters,omitempty"`
	Limit      int            `json:"limit,omitempty"`
}

// SearchHit is one result entry returned by search.
type SearchHit struct {
	ID           int64          `json:"id,omitempty"`
	Source       string         `json:"source"`
	OriginSource string         `json:"origin_source,omitempty"`
	Content      string         `json:"content"`
	SessionKey   string         `json:"session_key,omitempty"`
	Lineage      *SearchLineage `json:"lineage,omitempty"`
}

// SearchLineage is operator evidence for the session lineage attached to a
// search hit.
type SearchLineage struct {
	ParentSessionID string   `json:"parent_session_id,omitempty"`
	LineageKind     string   `json:"lineage_kind,omitempty"`
	ChildSessionIDs []string `json:"child_session_ids,omitempty"`
	Status          string   `json:"status"`
}

// SearchResultSet is the stable JSON shape for honcho_search.
type SearchResultSet struct {
	WorkspaceID   string                          `json:"workspace_id"`
	Peer          string                          `json:"peer"`
	Query         string                          `json:"query"`
	ScopeEvidence *memory.CrossChatRecallEvidence `json:"scope_evidence,omitempty"`
	Results       []SearchHit                     `json:"results"`
}

// ContextParams controls honcho_context reads.
type ContextParams struct {
	Peer                string   `json:"peer"`
	Query               string   `json:"query,omitempty"`
	SearchQuery         string   `json:"search_query,omitempty"`
	MaxTokens           int      `json:"max_tokens,omitempty"`
	Tokens              int      `json:"tokens,omitempty"`
	Summary             *bool    `json:"summary,omitempty"`
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

// SessionSummary is the summary component returned by session context when a
// short or long summary slot fits inside the requested summary budget.
type SessionSummary struct {
	Content     string `json:"content"`
	MessageID   int64  `json:"message_id"`
	SummaryType string `json:"summary_type"`
	CreatedAt   int64  `json:"created_at"`
	TokenCount  int    `json:"token_count"`
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
	WorkspaceID    string                          `json:"workspace_id"`
	Peer           string                          `json:"peer"`
	ObserverPeerID string                          `json:"observer_peer_id,omitempty"`
	ObservedPeerID string                          `json:"observed_peer_id,omitempty"`
	SessionKey     string                          `json:"session_key,omitempty"`
	PeerCard       []string                        `json:"peer_card"`
	Representation string                          `json:"representation"`
	Summary        *SessionSummary                 `json:"summary,omitempty"`
	Conclusions    []string                        `json:"conclusions,omitempty"`
	SearchResults  []SearchHit                     `json:"search_results,omitempty"`
	ScopeEvidence  *memory.CrossChatRecallEvidence `json:"scope_evidence,omitempty"`
	RecentMessages []MessageSlice                  `json:"recent_messages,omitempty"`
	Unavailable    []ContextUnavailableEvidence    `json:"unavailable,omitempty"`
}

// ChatParams mirrors Honcho's DialecticOptions request body for peer.chat().
// The peer itself is path/tool context, so it is passed separately to Service.Chat.
type ChatParams struct {
	SessionID      string `json:"session_id,omitempty"`
	Target         string `json:"target,omitempty"`
	Query          string `json:"query"`
	Stream         bool   `json:"stream,omitempty"`
	ReasoningLevel string `json:"reasoning_level,omitempty"`
}

// ChatResult is Honcho's non-streaming dialectic response shape.
type ChatResult struct {
	Content string `json:"content"`
}
