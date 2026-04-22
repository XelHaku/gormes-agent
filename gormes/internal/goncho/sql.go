package goncho

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type conclusionRow struct {
	WorkspaceID    string
	ObserverPeerID string
	PeerID         string
	SessionKey     string
	Content        string
	Kind           string
	Status         string
	Source         string
	IdempotencyKey string
	EvidenceJSON   string
}

func upsertPeerCard(ctx context.Context, db *sql.DB, workspaceID, peer string, card []string) error {
	raw, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("goncho: marshal peer card: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO goncho_peer_cards(workspace_id, peer_id, card_json, updated_at)
		VALUES(?, ?, ?, ?)
		ON CONFLICT(workspace_id, peer_id)
		DO UPDATE SET card_json = excluded.card_json, updated_at = excluded.updated_at
	`, workspaceID, peer, string(raw), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("goncho: upsert peer card: %w", err)
	}
	return nil
}

func getPeerCard(ctx context.Context, db *sql.DB, workspaceID, peer string) ([]string, error) {
	var raw string
	err := db.QueryRowContext(ctx, `
		SELECT card_json
		FROM goncho_peer_cards
		WHERE workspace_id = ? AND peer_id = ?
	`, workspaceID, peer).Scan(&raw)
	if err == sql.ErrNoRows {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("goncho: get peer card: %w", err)
	}
	var card []string
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		return nil, fmt.Errorf("goncho: decode peer card: %w", err)
	}
	return card, nil
}

func upsertConclusion(ctx context.Context, db *sql.DB, row conclusionRow) (int64, string, error) {
	now := time.Now().Unix()
	_, err := db.ExecContext(ctx, `
		INSERT INTO goncho_conclusions(
			workspace_id, observer_peer_id, peer_id, session_key, content,
			kind, status, source, idempotency_key, evidence_json, created_at, updated_at
		)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, observer_peer_id, peer_id, idempotency_key)
		DO UPDATE SET updated_at = excluded.updated_at
	`,
		row.WorkspaceID,
		row.ObserverPeerID,
		row.PeerID,
		nullIfBlank(row.SessionKey),
		row.Content,
		row.Kind,
		row.Status,
		row.Source,
		row.IdempotencyKey,
		row.EvidenceJSON,
		now,
		now,
	)
	if err != nil {
		return 0, "", fmt.Errorf("goncho: upsert conclusion: %w", err)
	}

	var id int64
	var status string
	err = db.QueryRowContext(ctx, `
		SELECT id, status
		FROM goncho_conclusions
		WHERE workspace_id = ? AND observer_peer_id = ? AND peer_id = ? AND idempotency_key = ?
	`, row.WorkspaceID, row.ObserverPeerID, row.PeerID, row.IdempotencyKey).Scan(&id, &status)
	if err != nil {
		return 0, "", fmt.Errorf("goncho: lookup conclusion after upsert: %w", err)
	}
	return id, status, nil
}

func deleteConclusion(ctx context.Context, db *sql.DB, workspaceID, observer, peer string, id int64) (bool, error) {
	res, err := db.ExecContext(ctx, `
		DELETE FROM goncho_conclusions
		WHERE id = ? AND workspace_id = ? AND observer_peer_id = ? AND peer_id = ?
	`, id, workspaceID, observer, peer)
	if err != nil {
		return false, fmt.Errorf("goncho: delete conclusion: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("goncho: delete conclusion rows affected: %w", err)
	}
	return affected > 0, nil
}

func findConclusions(ctx context.Context, db *sql.DB, workspaceID, observer, peer, query, sessionKey string, limit int) ([]SearchHit, error) {
	base := `
		SELECT id, content, COALESCE(session_key, '')
		FROM goncho_conclusions
		WHERE workspace_id = ? AND observer_peer_id = ? AND peer_id = ?
	`
	args := []any{workspaceID, observer, peer}
	if trimmed := strings.TrimSpace(sessionKey); trimmed != "" {
		base += ` AND (session_key = ? OR session_key IS NULL)`
		args = append(args, trimmed)
	}
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		base += ` AND content LIKE ?`
		args = append(args, "%"+trimmed+"%")
	}
	base += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("goncho: find conclusions: %w", err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var hit SearchHit
		hit.Source = "conclusion"
		if err := rows.Scan(&hit.ID, &hit.Content, &hit.SessionKey); err != nil {
			return nil, fmt.Errorf("goncho: scan conclusion: %w", err)
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("goncho: iterate conclusions: %w", err)
	}
	return hits, nil
}

func findTurns(ctx context.Context, db *sql.DB, query, sessionKey string, limit int) ([]SearchHit, error) {
	if strings.TrimSpace(sessionKey) == "" {
		return nil, nil
	}

	base := `
		SELECT content
		FROM turns
		WHERE (chat_id = ? OR session_id = ?)
	`
	args := []any{sessionKey, sessionKey}
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		base += ` AND content LIKE ?`
		args = append(args, "%"+trimmed+"%")
	}
	base += ` ORDER BY ts_unix DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("goncho: find turns: %w", err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, fmt.Errorf("goncho: scan turn: %w", err)
		}
		hits = append(hits, SearchHit{
			Source:     "turn",
			Content:    content,
			SessionKey: sessionKey,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("goncho: iterate turns: %w", err)
	}
	return hits, nil
}

func recentTurns(ctx context.Context, db *sql.DB, sessionKey string, limit int) ([]MessageSlice, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT role, content
		FROM turns
		WHERE chat_id = ? OR session_id = ?
		ORDER BY ts_unix DESC, id DESC
		LIMIT ?
	`, sessionKey, sessionKey, limit)
	if err != nil {
		return nil, fmt.Errorf("goncho: recent turns: %w", err)
	}
	defer rows.Close()

	var reverse []MessageSlice
	for rows.Next() {
		var msg MessageSlice
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, fmt.Errorf("goncho: scan recent turn: %w", err)
		}
		reverse = append(reverse, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("goncho: iterate recent turns: %w", err)
	}

	out := make([]MessageSlice, 0, len(reverse))
	for i := len(reverse) - 1; i >= 0; i-- {
		out = append(out, reverse[i])
	}
	return out, nil
}

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
