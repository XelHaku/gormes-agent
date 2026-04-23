package memory

import (
	"context"
	"database/sql"
	"fmt"
)

// writeGraphBatch upserts the validated entities + relationships and marks
// the given turnIDs as extracted=1. One transaction for the whole batch
// so the graph is never left in a half-written state.
//
// An empty ValidatedOutput is legal (LLM found nothing); we still mark
// the turns as extracted=1 to avoid infinite retries.
func writeGraphBatch(ctx context.Context, db *sql.DB, v ValidatedOutput, turnIDs []int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Upsert entities, collect name+type -> id map.
	idByKey := make(map[string]int64, len(v.Entities))
	for _, e := range v.Entities {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO entities(name, type, description, updated_at)
			 VALUES(?, ?, ?, strftime('%s','now'))
			 ON CONFLICT(name, type) DO UPDATE SET
			   description = CASE WHEN excluded.description != ''
			                      THEN excluded.description
			                      ELSE entities.description END,
			   updated_at = excluded.updated_at`,
			e.Name, e.Type, e.Description); err != nil {
			return fmt.Errorf("upsert entity %q/%s: %w", e.Name, e.Type, err)
		}
		var id int64
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM entities WHERE name = ? AND type = ?`,
			e.Name, e.Type).Scan(&id); err != nil {
			return fmt.Errorf("resolve entity id %q/%s: %w", e.Name, e.Type, err)
		}
		idByKey[e.Name+"\x00"+e.Type] = id
	}

	// Name -> id map (validator guarantees relationship source/target names
	// exist in entities[]; the orphan check already dropped mismatches).
	idByName := make(map[string]int64, len(v.Entities))
	for _, e := range v.Entities {
		idByName[e.Name] = idByKey[e.Name+"\x00"+e.Type]
	}

	for _, r := range v.Relationships {
		src, srcOK := idByName[r.Source]
		tgt, tgtOK := idByName[r.Target]
		if !srcOK || !tgtOK {
			continue // defensive; validator should have dropped these
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO relationships(source_id, target_id, predicate, weight, updated_at, last_seen)
			 VALUES(?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'))
			 ON CONFLICT(source_id, target_id, predicate) DO UPDATE SET
			   weight = MIN(relationships.weight + excluded.weight, 10.0),
			   last_seen = excluded.last_seen`,
			src, tgt, r.Predicate, r.Weight); err != nil {
			return fmt.Errorf("upsert rel %d-%s->%d: %w", src, r.Predicate, tgt, err)
		}
	}

	// Mark turns extracted=1 and clear any prior error.
	if len(turnIDs) > 0 {
		if err := execInIDs(ctx, tx,
			`UPDATE turns SET extracted = 1, extraction_error = NULL WHERE id IN (%s)`,
			turnIDs); err != nil {
			return fmt.Errorf("mark turns extracted: %w", err)
		}
	}

	return tx.Commit()
}

// incrementAttempts bumps extraction_attempts on the given turn IDs and
// records the last-seen error message. Keeps extracted = 0 so the turns
// stay eligible for retry.
func incrementAttempts(ctx context.Context, db *sql.DB, turnIDs []int64, errMsg string) error {
	if len(turnIDs) == 0 {
		return nil
	}
	return execInIDsDB(ctx, db,
		`UPDATE turns SET extraction_attempts = extraction_attempts + 1,
		                  extraction_error = ?
		 WHERE id IN (%s)`,
		turnIDs, errMsg)
}

// markDeadLetter sets extracted = 2 on the given turn IDs. After this,
// the polling query WHERE extracted = 0 skips them permanently.
func markDeadLetter(ctx context.Context, db *sql.DB, turnIDs []int64, errMsg string) error {
	if len(turnIDs) == 0 {
		return nil
	}
	return execInIDsDB(ctx, db,
		`UPDATE turns SET extracted = 2, extraction_error = ? WHERE id IN (%s)`,
		turnIDs, errMsg)
}

// execInIDs runs a query whose last argument is a variadic IN-list of
// int64 IDs, interpolated into the query string as "?,?,?...". The
// template must contain exactly one "%s" placeholder. Values are ALWAYS
// bound as parameters — only the comma-separated "?" count is
// interpolated.
func execInIDs(ctx context.Context, tx *sql.Tx, tmpl string, ids []int64) error {
	placeholders, args := inListArgs(ids)
	_, err := tx.ExecContext(ctx, fmt.Sprintf(tmpl, placeholders), args...)
	return err
}

func execInIDsDB(ctx context.Context, db *sql.DB, tmpl string, ids []int64, leadingArgs ...any) error {
	placeholders, idArgs := inListArgs(ids)
	args := append(append([]any{}, leadingArgs...), idArgs...)
	_, err := db.ExecContext(ctx, fmt.Sprintf(tmpl, placeholders), args...)
	return err
}

func inListArgs(ids []int64) (string, []any) {
	if len(ids) == 0 {
		return "NULL", nil
	}
	var b []byte
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
		args = append(args, id)
	}
	return string(b), args
}
