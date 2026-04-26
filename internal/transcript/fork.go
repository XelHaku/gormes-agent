package transcript

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ForkTurns copies every persisted turn from parentSessionID into a new row
// keyed by childSessionID. Returns the number of turns copied. The parent
// session's rows are left intact, satisfying the contract that fork children
// own their own transcript history without rewriting their parent's.
//
// The copy preserves role, content, ts_unix, chat_id, and meta_json. New
// rows get fresh AUTOINCREMENT ids; the rest of the schema (extracted,
// extraction_attempts, cron, memory_sync_status, …) is intentionally left at
// schema defaults so the child enters extraction/sync queues as a normal new
// session — never inheriting the parent's processed/extracted state.
//
// Single-statement INSERT…SELECT keeps the operation atomic at the SQLite
// level: callers get the same row count or an error; partial copies are not
// observable from outside the connection.
func ForkTurns(ctx context.Context, db *sql.DB, parentSessionID, childSessionID string) (int, error) {
	parent := strings.TrimSpace(parentSessionID)
	child := strings.TrimSpace(childSessionID)
	if parent == "" {
		return 0, errors.New("transcript: ForkTurns parent_session_id required")
	}
	if child == "" {
		return 0, errors.New("transcript: ForkTurns child_session_id required")
	}
	if parent == child {
		return 0, fmt.Errorf("transcript: ForkTurns parent and child must differ (got %q)", parent)
	}

	res, err := db.ExecContext(ctx, `
		INSERT INTO turns(session_id, role, content, ts_unix, chat_id, meta_json)
		SELECT ?, role, content, ts_unix, chat_id, meta_json
		FROM turns
		WHERE session_id = ?
		ORDER BY ts_unix ASC, id ASC
	`, child, parent)
	if err != nil {
		return 0, fmt.Errorf("transcript: ForkTurns parent=%s child=%s: %w", parent, child, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("transcript: ForkTurns rows affected: %w", err)
	}
	return int(n), nil
}
