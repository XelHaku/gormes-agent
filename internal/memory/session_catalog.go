package memory

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

// SearchFilter narrows cross-session search to one canonical user and an
// optional set of transport sources.
type SearchFilter struct {
	UserID  string
	Sources []string
	Query   string
}

// MessageSearchHit is one turn-level result from the session catalog.
type MessageSearchHit struct {
	SessionID string
	ChatID    string
	Source    string
	Role      string
	Content   string
	TSUnix    int64
}

// SessionSearchHit is one session-level result ordered by latest matching turn.
type SessionSearchHit struct {
	SessionID      string
	ChatID         string
	Source         string
	LatestTurnUnix int64
}

// SearchMessages returns matching turns across the canonical sessions bound to
// one user, optionally narrowed to a subset of sources.
func SearchMessages(ctx context.Context, db *sql.DB, metas []session.Metadata, filter SearchFilter, limit int) ([]MessageSearchHit, error) {
	selected := selectMetadata(metas, filter)
	if len(selected) == 0 || limit == 0 {
		return nil, nil
	}

	sessionIDs, chatKeys, metaBySession, metaByChat := metadataIndexes(selected)
	query, args := buildTurnSearchQuery(filter.Query, sessionIDs, chatKeys, limit, false)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("session catalog: search messages: %w", err)
	}
	defer rows.Close()

	var hits []MessageSearchHit
	for rows.Next() {
		var hit MessageSearchHit
		if err := rows.Scan(&hit.SessionID, &hit.ChatID, &hit.Role, &hit.Content, &hit.TSUnix); err != nil {
			return nil, fmt.Errorf("session catalog: scan message hit: %w", err)
		}
		if meta, ok := metaBySession[hit.SessionID]; ok {
			hit.Source = meta.Source
		} else if meta, ok := metaByChat[hit.ChatID]; ok {
			hit.Source = meta.Source
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("session catalog: iterate message hits: %w", err)
	}
	return hits, nil
}

// SearchSessions returns one row per matching session ordered by latest turn.
func SearchSessions(ctx context.Context, db *sql.DB, metas []session.Metadata, filter SearchFilter, limit int) ([]SessionSearchHit, error) {
	selected := selectMetadata(metas, filter)
	if len(selected) == 0 || limit == 0 {
		return nil, nil
	}

	sessionIDs, chatKeys, metaBySession, metaByChat := metadataIndexes(selected)
	query, args := buildTurnSearchQuery(filter.Query, sessionIDs, chatKeys, limit, true)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("session catalog: search sessions: %w", err)
	}
	defer rows.Close()

	var hits []SessionSearchHit
	for rows.Next() {
		var hit SessionSearchHit
		if err := rows.Scan(&hit.SessionID, &hit.ChatID, &hit.LatestTurnUnix); err != nil {
			return nil, fmt.Errorf("session catalog: scan session hit: %w", err)
		}
		if meta, ok := metaBySession[hit.SessionID]; ok {
			hit.Source = meta.Source
		} else if meta, ok := metaByChat[hit.ChatID]; ok {
			hit.Source = meta.Source
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("session catalog: iterate session hits: %w", err)
	}
	return hits, nil
}

func selectMetadata(metas []session.Metadata, filter SearchFilter) []session.Metadata {
	userID := strings.TrimSpace(filter.UserID)
	if userID == "" {
		return nil
	}
	allowedSources := normalizeSources(filter.Sources)
	selected := make([]session.Metadata, 0, len(metas))
	for _, meta := range metas {
		if strings.TrimSpace(meta.UserID) != userID {
			continue
		}
		if len(allowedSources) > 0 && !slices.Contains(allowedSources, strings.ToLower(strings.TrimSpace(meta.Source))) {
			continue
		}
		selected = append(selected, meta)
	}
	return selected
}

func normalizeSources(sources []string) []string {
	if len(sources) == 0 {
		return nil
	}
	out := make([]string, 0, len(sources))
	for _, src := range sources {
		src = strings.ToLower(strings.TrimSpace(src))
		if src == "" || slices.Contains(out, src) {
			continue
		}
		out = append(out, src)
	}
	return out
}

func metadataIndexes(metas []session.Metadata) ([]string, []string, map[string]session.Metadata, map[string]session.Metadata) {
	sessionIDs := make([]string, 0, len(metas))
	chatKeys := make([]string, 0, len(metas))
	metaBySession := make(map[string]session.Metadata, len(metas))
	metaByChat := make(map[string]session.Metadata, len(metas))
	for _, meta := range metas {
		if sessionID := strings.TrimSpace(meta.SessionID); sessionID != "" {
			sessionIDs = append(sessionIDs, sessionID)
			metaBySession[sessionID] = meta
		}
		if chatKey := canonicalChatKey(meta); chatKey != "" {
			chatKeys = append(chatKeys, chatKey)
			metaByChat[chatKey] = meta
		}
	}
	return sessionIDs, chatKeys, metaBySession, metaByChat
}

func canonicalChatKey(meta session.Metadata) string {
	source := strings.TrimSpace(meta.Source)
	chatID := strings.TrimSpace(meta.ChatID)
	if source == "" || chatID == "" {
		return ""
	}
	return source + ":" + chatID
}

func buildTurnSearchQuery(rawQuery string, sessionIDs, chatKeys []string, limit int, sessionsOnly bool) (string, []any) {
	var b strings.Builder
	args := make([]any, 0, len(sessionIDs)+len(chatKeys)+2)
	if sessionsOnly {
		b.WriteString(`SELECT t.session_id, t.chat_id, MAX(t.ts_unix) AS latest_turn_unix FROM turns t`)
	} else {
		b.WriteString(`SELECT t.session_id, t.chat_id, t.role, t.content, t.ts_unix FROM turns t`)
	}

	query := sanitizeFTS5Pattern(rawQuery)
	if query != "" {
		b.WriteString(` JOIN turns_fts fts ON fts.rowid = t.id WHERE turns_fts MATCH ?`)
		args = append(args, query)
	} else {
		b.WriteString(` WHERE 1=1`)
	}

	b.WriteString(` AND (`)
	appendInClause(&b, "t.session_id", sessionIDs, &args)
	if len(chatKeys) > 0 {
		b.WriteString(` OR `)
		appendInClause(&b, "t.chat_id", chatKeys, &args)
	}
	b.WriteString(`)`)
	b.WriteString(` AND t.memory_sync_status = 'ready'`)

	if sessionsOnly {
		b.WriteString(` GROUP BY t.session_id, t.chat_id ORDER BY latest_turn_unix DESC, t.session_id ASC LIMIT ?`)
	} else {
		b.WriteString(` ORDER BY t.ts_unix DESC, t.id DESC LIMIT ?`)
	}
	args = append(args, limit)
	return b.String(), args
}

func appendInClause(b *strings.Builder, column string, values []string, args *[]any) {
	b.WriteString(column)
	b.WriteString(` IN (`)
	for i, value := range values {
		if i > 0 {
			b.WriteString(`,`)
		}
		b.WriteString(`?`)
		*args = append(*args, value)
	}
	b.WriteString(`)`)
}
