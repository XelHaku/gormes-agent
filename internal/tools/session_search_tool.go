package tools

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

const (
	sessionSearchDefaultLimit = 3
	sessionSearchMaxLimit     = 5
)

// SessionSearchDirectory exposes the canonical session metadata needed to bind
// session_search execution to the current chat before any recall widening.
type SessionSearchDirectory interface {
	GetMetadata(ctx context.Context, sessionID string) (session.Metadata, bool, error)
	ListMetadataByUserID(ctx context.Context, userID string) ([]session.Metadata, error)
}

// SessionSearchToolConfig wires the execution wrapper without registering it
// globally.
type SessionSearchToolConfig struct {
	DB       *sql.DB
	Sessions SessionSearchDirectory
}

// SessionSearchTool exposes the model-facing session_search descriptor and,
// when configured, executes against the local memory/session catalog.
type SessionSearchTool struct {
	db       *sql.DB
	sessions SessionSearchDirectory
}

func NewSessionSearchTool(config SessionSearchToolConfig) *SessionSearchTool {
	return &SessionSearchTool{
		db:       config.DB,
		sessions: config.Sessions,
	}
}

// SessionSearchArgs is the normalized argument shape for session_search.
type SessionSearchArgs struct {
	Query            string   `json:"query,omitempty"`
	Scope            string   `json:"scope"`
	Sources          []string `json:"sources,omitempty"`
	Mode             string   `json:"mode"`
	Limit            int      `json:"limit"`
	CurrentSessionID string   `json:"current_session_id,omitempty"`
}

// SessionSearchEvidence is degraded-mode evidence returned for invalid input.
type SessionSearchEvidence struct {
	Status    string `json:"status"`
	Field     string `json:"field,omitempty"`
	Reason    string `json:"reason"`
	SessionID string `json:"session_id,omitempty"`
}

type sessionSearchResult struct {
	Success       bool                            `json:"success"`
	Args          *SessionSearchArgs              `json:"args,omitempty"`
	Mode          string                          `json:"mode,omitempty"`
	Query         string                          `json:"query,omitempty"`
	Count         int                             `json:"count"`
	Results       []SessionSearchHit              `json:"results,omitempty"`
	Evidence      []SessionSearchEvidence         `json:"evidence,omitempty"`
	ScopeEvidence *memory.CrossChatRecallEvidence `json:"scope_evidence,omitempty"`
}

// SessionSearchHit is one model-facing hit from local session search.
type SessionSearchHit struct {
	SessionID      string                `json:"session_id"`
	ChatID         string                `json:"chat_id,omitempty"`
	Source         string                `json:"source,omitempty"`
	OriginSource   string                `json:"origin_source,omitempty"`
	Role           string                `json:"role,omitempty"`
	Content        string                `json:"content,omitempty"`
	LatestTurnUnix int64                 `json:"latest_turn_unix,omitempty"`
	Lineage        *SessionSearchLineage `json:"lineage,omitempty"`
}

type SessionSearchLineage struct {
	ParentSessionID string   `json:"parent_session_id,omitempty"`
	LineageKind     string   `json:"lineage_kind,omitempty"`
	ChildSessionIDs []string `json:"child_session_ids,omitempty"`
	Status          string   `json:"status"`
}

func (*SessionSearchTool) Name() string { return "session_search" }

func (*SessionSearchTool) Description() string {
	return "Search prior session transcripts or browse recent sessions using explicit same-chat or user-scoped recall controls."
}

func (*SessionSearchTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Optional keyword, phrase, or boolean search query. Omit to browse recent sessions."},"scope":{"type":"string","enum":["same-chat","user"],"default":"same-chat","description":"Recall scope. same-chat is the safe default; user may widen only when later execution can prove the current session binding."},"sources":{"type":"array","items":{"type":"string"},"description":"Optional source allowlist such as discord, telegram, slack, or matrix."},"mode":{"type":"string","enum":["default","recent","search"],"default":"default","description":"default chooses recent or search behavior from query presence; recent browses sessions; search runs keyword recall."},"limit":{"type":"integer","default":3,"minimum":0,"maximum":5,"description":"Maximum sessions to return. Defaults to 3 and is capped at 5."},"current_session_id":{"type":"string","description":"Current chat/session identifier used by later execution to prove same-chat or user-scope boundaries."}},"required":[]}`)
}

func (*SessionSearchTool) Timeout() time.Duration { return 5 * time.Second }

func (t *SessionSearchTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	normalized, evidence := ValidateSessionSearchArgs(args)
	if evidence != nil {
		return json.Marshal(sessionSearchResult{
			Success:  false,
			Evidence: []SessionSearchEvidence{*evidence},
		})
	}

	if t.db == nil || t.sessions == nil {
		return json.Marshal(sessionSearchResult{
			Success: false,
			Args:    &normalized,
			Evidence: []SessionSearchEvidence{{
				Status: "session_search_unavailable",
				Reason: "session_search execution requires a SQLite memory store and session directory",
			}},
		})
	}

	current, ok, err := t.sessions.GetMetadata(ctx, normalized.CurrentSessionID)
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(current.UserID) == "" {
		return json.Marshal(sessionSearchResult{
			Success: false,
			Args:    &normalized,
			Evidence: []SessionSearchEvidence{{
				Status:    "session_search_unavailable",
				SessionID: normalized.CurrentSessionID,
				Reason:    "current session metadata is unavailable; session_search cannot prove same-chat scope",
			}},
		})
	}

	metas, err := t.sessions.ListMetadataByUserID(ctx, current.UserID)
	if err != nil {
		return nil, err
	}
	mode := sessionSearchMode(normalized)
	if normalized.Scope == "user" {
		filter := memory.SearchFilter{
			UserID:           current.UserID,
			Sources:          normalized.Sources,
			Query:            normalized.Query,
			CurrentSessionID: normalized.CurrentSessionID,
			CurrentChatKey:   sessionSearchChatKey(current),
		}
		evidence := memory.ExplainCrossChatRecall(metas, filter)
		if evidence.Decision != memory.CrossChatDecisionAllowed {
			return json.Marshal(sessionSearchResult{
				Success:       false,
				Args:          &normalized,
				Mode:          mode,
				Evidence:      []SessionSearchEvidence{sourceFilterDeniedEvidence(evidence.Reason)},
				ScopeEvidence: &evidence,
			})
		}
		return t.executeSessionSearch(ctx, normalized, mode, metas, filter, &evidence, normalized.Limit)
	}

	filter := memory.SearchFilter{
		UserID:           current.UserID,
		Sources:          normalized.Sources,
		SessionIDs:       sameChatSessionIDs(metas, current),
		Query:            normalized.Query,
		CurrentSessionID: normalized.CurrentSessionID,
	}
	if !sameChatSourceFilterAllowed(normalized.Sources, current) {
		return json.Marshal(sessionSearchResult{
			Success: false,
			Args:    &normalized,
			Mode:    mode,
			Evidence: []SessionSearchEvidence{sourceFilterDeniedEvidence(
				"same-chat source filter does not include the current chat source; use scope=user with valid binding to widen recall",
			)},
		})
	}
	return t.executeSessionSearch(ctx, normalized, mode, metas, filter, nil, normalized.Limit)
}

func (t *SessionSearchTool) executeSessionSearch(ctx context.Context, args SessionSearchArgs, mode string, metas []session.Metadata, filter memory.SearchFilter, scopeEvidence *memory.CrossChatRecallEvidence, limit int) (json.RawMessage, error) {
	if mode == "recent" {
		searchLimit := limit + sessionSearchMaxLimit
		sessions, err := memory.SearchSessions(ctx, t.db, metas, filter, searchLimit)
		if err != nil {
			return nil, err
		}
		results := make([]SessionSearchHit, 0, len(sessions))
		for _, hit := range sessions {
			results = append(results, sessionSearchHitFromSession(hit))
		}
		var evidence []SessionSearchEvidence
		var lineageEvidence *SessionSearchEvidence
		results, lineageEvidence = excludeCurrentLineageFromRecent(results, metas, args.CurrentSessionID)
		if lineageEvidence != nil {
			evidence = append(evidence, *lineageEvidence)
		}
		if len(results) > limit {
			results = results[:limit]
		}
		return json.Marshal(sessionSearchResult{
			Success:       true,
			Args:          &args,
			Mode:          mode,
			Count:         len(results),
			Results:       results,
			Evidence:      evidence,
			ScopeEvidence: scopeEvidence,
		})
	}

	messages, err := memory.SearchMessages(ctx, t.db, metas, filter, limit)
	if err != nil {
		return nil, err
	}
	results := make([]SessionSearchHit, 0, len(messages))
	for _, hit := range messages {
		results = append(results, sessionSearchHitFromMessage(hit))
	}
	return json.Marshal(sessionSearchResult{
		Success:       true,
		Args:          &args,
		Mode:          mode,
		Query:         args.Query,
		Count:         len(results),
		Results:       results,
		ScopeEvidence: scopeEvidence,
	})
}

func sourceFilterDeniedEvidence(reason string) SessionSearchEvidence {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "source-filtered user-scope widening was denied"
	}
	return SessionSearchEvidence{
		Status: "source_filter_denied",
		Reason: reason,
	}
}

func excludeCurrentLineageFromRecent(results []SessionSearchHit, metas []session.Metadata, currentSessionID string) ([]SessionSearchHit, *SessionSearchEvidence) {
	currentRoot, ok := sessionSearchLineageRoot(metas, currentSessionID)
	if !ok || currentRoot == "" {
		return results, nil
	}
	out := make([]SessionSearchHit, 0, len(results))
	excluded := false
	for _, result := range results {
		if result.SessionID == strings.TrimSpace(currentSessionID) || result.SessionID == currentRoot {
			excluded = true
			continue
		}
		if resultRoot, ok := sessionSearchLineageRoot(metas, result.SessionID); ok && resultRoot == currentRoot {
			excluded = true
			continue
		}
		out = append(out, result)
	}
	if !excluded {
		return out, nil
	}
	return out, &SessionSearchEvidence{
		Status:    "lineage_root_excluded",
		SessionID: currentRoot,
		Reason:    "recent mode excluded the current session lineage root",
	}
}

func sessionSearchLineageRoot(metas []session.Metadata, sessionID string) (string, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", false
	}
	byID := make(map[string]session.Metadata, len(metas))
	for _, meta := range metas {
		meta.SessionID = strings.TrimSpace(meta.SessionID)
		if meta.SessionID != "" {
			byID[meta.SessionID] = meta
		}
	}
	if _, ok := byID[sessionID]; !ok {
		return sessionID, false
	}
	root := sessionID
	seen := map[string]struct{}{}
	for {
		if _, ok := seen[root]; ok {
			return root, true
		}
		seen[root] = struct{}{}
		meta := byID[root]
		parent := strings.TrimSpace(meta.ParentSessionID)
		if parent == "" {
			return root, true
		}
		if _, ok := byID[parent]; !ok {
			return parent, true
		}
		root = parent
	}
}

func sessionSearchMode(args SessionSearchArgs) string {
	switch args.Mode {
	case "recent":
		return "recent"
	case "search":
		return "search"
	default:
		if strings.TrimSpace(args.Query) == "" {
			return "recent"
		}
		return "search"
	}
}

func sameChatSessionIDs(metas []session.Metadata, current session.Metadata) []string {
	currentKey := sessionSearchChatKey(current)
	if currentKey == "" {
		return []string{strings.TrimSpace(current.SessionID)}
	}
	out := make([]string, 0, len(metas))
	for _, meta := range metas {
		if sessionSearchChatKey(meta) != currentKey {
			continue
		}
		sessionID := strings.TrimSpace(meta.SessionID)
		if sessionID == "" || slices.Contains(out, sessionID) {
			continue
		}
		out = append(out, sessionID)
	}
	if len(out) == 0 {
		return []string{strings.TrimSpace(current.SessionID)}
	}
	return out
}

func sameChatSourceFilterAllowed(sources []string, current session.Metadata) bool {
	if len(sources) == 0 {
		return true
	}
	return slices.Contains(sources, normalizeSessionSearchLabel(current.Source))
}

func sessionSearchChatKey(meta session.Metadata) string {
	source := normalizeSessionSearchLabel(meta.Source)
	chatID := strings.TrimSpace(meta.ChatID)
	if source == "" || chatID == "" {
		return ""
	}
	return source + ":" + chatID
}

func sessionSearchHitFromMessage(hit memory.MessageSearchHit) SessionSearchHit {
	return SessionSearchHit{
		SessionID:    hit.SessionID,
		ChatID:       hit.ChatID,
		Source:       hit.Source,
		OriginSource: hit.Source,
		Role:         hit.Role,
		Content:      hit.Content,
		Lineage:      sessionSearchLineageFromMemory(hit.Lineage),
	}
}

func sessionSearchHitFromSession(hit memory.SessionSearchHit) SessionSearchHit {
	return SessionSearchHit{
		SessionID:      hit.SessionID,
		ChatID:         hit.ChatID,
		Source:         hit.Source,
		OriginSource:   hit.Source,
		LatestTurnUnix: hit.LatestTurnUnix,
		Lineage:        sessionSearchLineageFromMemory(hit.Lineage),
	}
}

func sessionSearchLineageFromMemory(lineage memory.SearchLineage) *SessionSearchLineage {
	if strings.TrimSpace(lineage.Status) == "" &&
		strings.TrimSpace(lineage.ParentSessionID) == "" &&
		strings.TrimSpace(lineage.LineageKind) == "" &&
		len(lineage.ChildSessionIDs) == 0 {
		return nil
	}
	status := strings.TrimSpace(lineage.Status)
	if status == "" {
		status = memory.SearchLineageStatusUnavailable
	}
	return &SessionSearchLineage{
		ParentSessionID: strings.TrimSpace(lineage.ParentSessionID),
		LineageKind:     strings.TrimSpace(lineage.LineageKind),
		ChildSessionIDs: append([]string(nil), lineage.ChildSessionIDs...),
		Status:          status,
	}
}

// ValidateSessionSearchArgs normalizes safe defaults and rejects arguments that
// would make a later execution wrapper widen recall without explicit evidence.
func ValidateSessionSearchArgs(raw json.RawMessage) (SessionSearchArgs, *SessionSearchEvidence) {
	args := SessionSearchArgs{
		Scope:   "same-chat",
		Sources: []string{},
		Mode:    "default",
		Limit:   sessionSearchDefaultLimit,
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return args, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return args, invalidSessionSearchArgs("args", "arguments must be a JSON object")
	}
	if fields == nil {
		return args, invalidSessionSearchArgs("args", "arguments must be a JSON object")
	}

	if value, ok := fields["query"]; ok {
		query, err := sessionSearchString(value)
		if err != nil {
			return args, invalidSessionSearchArgs("query", err.Error())
		}
		args.Query = strings.TrimSpace(query)
	}

	if value, ok := fields["scope"]; ok {
		scope, err := sessionSearchString(value)
		if err != nil {
			return args, invalidSessionSearchArgs("scope", err.Error())
		}
		args.Scope = normalizeSessionSearchLabel(scope)
		if args.Scope == "" {
			args.Scope = "same-chat"
		}
		switch args.Scope {
		case "same-chat", "user":
		default:
			return args, invalidSessionSearchArgs("scope", fmt.Sprintf("unsupported scope %q; supported scopes are same-chat and user", scope))
		}
	}

	if value, ok := fields["sources"]; ok {
		var sources []string
		if err := json.Unmarshal(value, &sources); err != nil {
			return args, invalidSessionSearchArgs("sources", "sources must be an array of strings")
		}
		for _, source := range sources {
			source = normalizeSessionSearchLabel(source)
			if source == "" {
				return args, invalidSessionSearchArgs("sources", "sources must not contain empty values")
			}
			args.Sources = append(args.Sources, source)
		}
	}

	if value, ok := fields["mode"]; ok {
		mode, err := sessionSearchString(value)
		if err != nil {
			return args, invalidSessionSearchArgs("mode", err.Error())
		}
		args.Mode = normalizeSessionSearchLabel(mode)
		if args.Mode == "" {
			args.Mode = "default"
		}
		switch args.Mode {
		case "default", "recent", "search":
		default:
			return args, invalidSessionSearchArgs("mode", fmt.Sprintf("unsupported mode %q; supported modes are default, recent, and search", mode))
		}
	}

	if value, ok := fields["limit"]; ok {
		var limit int
		if err := json.Unmarshal(value, &limit); err != nil {
			return args, invalidSessionSearchArgs("limit", "limit must be an integer")
		}
		switch {
		case limit < 0:
			return args, invalidSessionSearchArgs("limit", "limit must be non-negative")
		case limit == 0:
			args.Limit = sessionSearchDefaultLimit
		case limit > sessionSearchMaxLimit:
			return args, invalidSessionSearchArgs("limit", fmt.Sprintf("limit must be <= %d", sessionSearchMaxLimit))
		default:
			args.Limit = limit
		}
	}

	if value, ok := fields["current_session_id"]; ok {
		currentSessionID, err := sessionSearchString(value)
		if err != nil {
			return args, invalidSessionSearchArgs("current_session_id", err.Error())
		}
		args.CurrentSessionID = strings.TrimSpace(currentSessionID)
	}

	return args, nil
}

func sessionSearchString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("value must be a string")
	}
	return value, nil
}

func normalizeSessionSearchLabel(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func invalidSessionSearchArgs(field, reason string) *SessionSearchEvidence {
	return &SessionSearchEvidence{
		Status: "session_search_invalid_args",
		Field:  field,
		Reason: reason,
	}
}
