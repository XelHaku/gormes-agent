package transcript

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrSessionNotFound = errors.New("transcript: session not found")

type turn struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	Timestamp time.Time
	ChatID    string
	MetaJSON  string
}

type turnMeta struct {
	ToolCalls []toolCall `json:"tool_calls"`
}

type toolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func ExportMarkdown(ctx context.Context, db *sql.DB, sessionID string) (string, error) {
	turns, err := loadTurns(ctx, db, sessionID)
	if err != nil {
		return "", err
	}
	if len(turns) == 0 {
		return "", ErrSessionNotFound
	}

	var b strings.Builder
	created := turns[0].Timestamp.UTC().Format("2006-01-02 15:04:05 MST")

	fmt.Fprintf(&b, "# Session: %s\n\n", sessionID)
	fmt.Fprintf(&b, "**Session ID:** `%s`  \n", sessionID)
	fmt.Fprintf(&b, "**Platform:** %s  \n", derivePlatform(turns))
	fmt.Fprintf(&b, "**Created:** %s  \n", created)
	fmt.Fprintf(&b, "**Messages:** %d\n", len(turns))

	for i, turn := range turns {
		b.WriteString("\n---\n\n")
		fmt.Fprintf(&b, "## Turn %d - %s\n\n", i+1, turn.Timestamp.UTC().Format("2006-01-02 15:04:05 MST"))
		switch turn.Role {
		case "user":
			fmt.Fprintf(&b, "**User:** %s\n", turn.Content)
		case "assistant":
			fmt.Fprintf(&b, "**Agent:** %s\n", turn.Content)
		default:
			fmt.Fprintf(&b, "**%s:** %s\n", turn.Role, turn.Content)
		}

		meta, err := parseMeta(turn.MetaJSON)
		if err != nil {
			return "", err
		}
		if len(meta.ToolCalls) > 0 {
			b.WriteString("\n**Tool Calls:**\n")
			for _, call := range meta.ToolCalls {
				fmt.Fprintf(&b, "- `%s` `%s`\n", call.Name, compactJSON(call.Arguments))
			}
		}
	}

	return b.String(), nil
}

func loadTurns(ctx context.Context, db *sql.DB, sessionID string) ([]turn, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, session_id, role, content, ts_unix, COALESCE(chat_id, ''), COALESCE(meta_json, '')
		FROM turns
		WHERE session_id = ?
		ORDER BY ts_unix ASC, id ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("transcript: query session %q: %w", sessionID, err)
	}
	defer rows.Close()

	var out []turn
	for rows.Next() {
		var row turn
		var ts int64
		if err := rows.Scan(&row.ID, &row.SessionID, &row.Role, &row.Content, &ts, &row.ChatID, &row.MetaJSON); err != nil {
			return nil, fmt.Errorf("transcript: scan session %q: %w", sessionID, err)
		}
		row.Timestamp = time.Unix(ts, 0).UTC()
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("transcript: iterate session %q: %w", sessionID, err)
	}
	return out, nil
}

func parseMeta(raw string) (turnMeta, error) {
	if raw == "" {
		return turnMeta{}, nil
	}
	var meta turnMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return turnMeta{}, fmt.Errorf("transcript: decode meta_json: %w", err)
	}
	return meta, nil
}

func derivePlatform(turns []turn) string {
	for _, turn := range turns {
		if turn.ChatID == "" {
			continue
		}
		if prefix, _, ok := strings.Cut(turn.ChatID, ":"); ok && prefix != "" {
			return prefix
		}
		return turn.ChatID
	}
	if strings.HasPrefix(turns[0].SessionID, "cron:") {
		return "cron"
	}
	return "unknown"
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err == nil {
		return buf.String()
	}
	return string(raw)
}
