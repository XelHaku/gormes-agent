package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// seedsExactName returns up to `limit` entity IDs whose name (lower-fold)
// matches any of the provided candidates. Silently drops short candidates
// (<3 chars) before sending to SQL. Empty candidates list returns
// (nil, nil) with no DB round-trip.
func seedsExactName(ctx context.Context, db *sql.DB, candidates []string, limit int) ([]int64, error) {
	// Pre-filter: drop empties and shorts, lower-fold for the IN-list.
	clean := make([]any, 0, len(candidates))
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if len(c) < 3 {
			continue
		}
		clean = append(clean, strings.ToLower(c))
	}
	if len(clean) == 0 {
		return nil, nil
	}

	placeholders := strings.Repeat("?,", len(clean))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
	args := append(clean, any(limit))
	q := fmt.Sprintf(
		`SELECT id FROM entities
		 WHERE lower(name) IN (%s)
		   AND length(name) >= 3
		 LIMIT ?`, placeholders)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("seedsExactName: %w", err)
	}
	defer rows.Close()
	return scanIDs(rows)
}

// seedsFTS5 is the Layer 2 fallback: FTS5 MATCH over turns.content, joined
// back to entities whose names appear in those turns. Per-chat scoped via
// the chat_id filter (empty string = global scope — matches any chat_id).
func seedsFTS5(ctx context.Context, db *sql.DB, userMessage, chatKey string, limit int) ([]int64, error) {
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return nil, nil
	}

	q := `
		SELECT DISTINCT e.id
		FROM turns_fts fts
		JOIN turns t ON t.id = fts.rowid
		JOIN entities e ON lower(t.content) LIKE '%' || lower(e.name) || '%'
		WHERE turns_fts MATCH ?
		  AND (t.chat_id = ? OR ? = '')
		  AND length(e.name) >= 3
		LIMIT ?
	`
	rows, err := db.QueryContext(ctx, q, msg, chatKey, chatKey, limit)
	if err != nil {
		return nil, fmt.Errorf("seedsFTS5: %w", err)
	}
	defer rows.Close()
	return scanIDs(rows)
}

// scanIDs drains `rows` into a []int64 of ID columns.
func scanIDs(rows *sql.Rows) ([]int64, error) {
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
