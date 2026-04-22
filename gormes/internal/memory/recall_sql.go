package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// weightExpr returns the SQL expression that substitutes for r.weight
// in WHERE / ORDER BY clauses. When horizonDays <= 0, decay is
// disabled and the raw column reference is returned. Otherwise a
// linear-decay expression with one bound parameter (horizonSec) is
// returned; callers are responsible for binding horizonSec in the
// correct argument position.
//
// The expression: MAX(0, r.weight * (1 - age_seconds / horizon_seconds))
//   - CAST forces float division (integer division snaps to 0/1 at
//     the day boundary).
//   - MAX(0, ...) clamps rows older than horizon to exactly zero;
//     no wrap-around in ORDER BY.
//   - Uses r.updated_at (existing column); no schema change.
func weightExpr(horizonDays int) string {
	if horizonDays <= 0 {
		return "r.weight"
	}
	return "MAX(0, r.weight * (1 - CAST(strftime('%s','now') - r.updated_at AS REAL) / ?))"
}

// seedsExactName returns up to `limit` entity IDs whose name (lower-fold)
// matches any of the provided candidates. Silently drops short candidates
// (<3 chars) before sending to SQL. Empty candidates list returns
// (nil, nil) with no DB round-trip.
func seedsExactName(ctx context.Context, db *sql.DB, candidates []string, chatKeys []string, limit int) ([]int64, error) {
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
	args := append([]any{}, clean...)
	var q string
	if len(chatKeys) == 0 {
		args = append(args, any(limit))
		q = fmt.Sprintf(
			`SELECT id FROM entities
			 WHERE lower(name) IN (%s)
			   AND length(name) >= 3
			 ORDER BY updated_at DESC
			 LIMIT ?`, placeholders)
	} else {
		chatPlaceholders := strings.Repeat("?,", len(chatKeys))
		chatPlaceholders = chatPlaceholders[:len(chatPlaceholders)-1]
		for _, chatKey := range chatKeys {
			args = append(args, chatKey)
		}
		args = append(args, any(limit))
		q = fmt.Sprintf(
			`SELECT DISTINCT e.id
			 FROM entities e
			 JOIN turns t ON lower(t.content) LIKE '%%' || lower(e.name) || '%%'
			 WHERE lower(e.name) IN (%s)
			   AND length(e.name) >= 3
			   AND t.chat_id IN (%s)
			 ORDER BY e.updated_at DESC
			 LIMIT ?`, placeholders, chatPlaceholders)
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("seedsExactName: %w", err)
	}
	defer rows.Close()
	return scanIDs(rows)
}

func seedsFTS5Scoped(ctx context.Context, db *sql.DB, userMessage string, chatKeys []string, limit int) ([]int64, error) {
	if len(chatKeys) == 0 {
		return seedsFTS5(ctx, db, userMessage, "", limit)
	}

	seen := make(map[int64]struct{}, limit)
	out := make([]int64, 0, limit)
	for _, chatKey := range chatKeys {
		ids, err := seedsFTS5(ctx, db, userMessage, chatKey, limit)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func filterEntityIDsByChatScope(ctx context.Context, db *sql.DB, ids []int64, chatKeys []string) ([]int64, error) {
	if len(ids) == 0 || len(chatKeys) == 0 {
		return ids, nil
	}

	idPlaceholders := strings.Repeat("?,", len(ids))
	idPlaceholders = idPlaceholders[:len(idPlaceholders)-1]
	chatPlaceholders := strings.Repeat("?,", len(chatKeys))
	chatPlaceholders = chatPlaceholders[:len(chatPlaceholders)-1]

	args := make([]any, 0, len(ids)+len(chatKeys))
	for _, id := range ids {
		args = append(args, id)
	}
	for _, chatKey := range chatKeys {
		args = append(args, chatKey)
	}

	q := fmt.Sprintf(
		`SELECT DISTINCT e.id
		 FROM entities e
		 JOIN turns t ON lower(t.content) LIKE '%%' || lower(e.name) || '%%'
		 WHERE e.id IN (%s)
		   AND t.chat_id IN (%s)`, idPlaceholders, chatPlaceholders)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("filterEntityIDsByChatScope: %w", err)
	}
	defer rows.Close()

	allowed := make(map[int64]struct{}, len(ids))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		allowed[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := allowed[id]; ok {
			out = append(out, id)
		}
	}
	return out, nil
}

// seedsFTS5 is the Layer 2 fallback: FTS5 MATCH over turns.content, joined
// back to entities whose names appear in those turns. Per-chat scoped via
// the chat_id filter (empty string = global scope — matches any chat_id).
func seedsFTS5(ctx context.Context, db *sql.DB, userMessage, chatKey string, limit int) ([]int64, error) {
	msg := sanitizeFTS5Pattern(userMessage)
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

// sanitizeFTS5Pattern strips characters that FTS5 treats as operators
// ("?", "*", "(", ")", "+", "-", double quotes, etc.) so a user message
// with normal punctuation ("how does Acme work?") becomes a valid
// FTS5 MATCH pattern. Without this, any message containing "?" or "*"
// produces "fts5: syntax error near ..." on every lookup.
//
// We preserve alphanumerics + spaces + underscores + hyphens (hyphens
// are safe inside tokens). Everything else collapses to space, then
// runs of spaces collapse to one.
func sanitizeFTS5Pattern(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == ' ', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	// Collapse runs of spaces.
	out := b.String()
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return strings.TrimSpace(out)
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

// traverseNeighborhood runs the Recursive CTE that expands a set of seed
// entity IDs into a depth-bounded neighborhood, filtered by relationship
// weight (decay-aware) >= threshold, sorted by depth ASC then updated_at
// DESC, capped at maxFacts.
//
// horizonDays controls Phase 3.E.6 decay. <= 0 disables decay and uses
// the raw weight column as the filter.
//
// Depth 0 = seeds themselves.
// Depth N = reachable via N hops along edges with effective weight >= threshold.
func traverseNeighborhood(
	ctx context.Context,
	db *sql.DB,
	seedIDs []int64,
	depth int,
	threshold float64,
	maxFacts int,
	horizonDays int,
) ([]recalledEntity, error) {
	if len(seedIDs) == 0 {
		return nil, nil
	}

	// Build the seeds VALUES() clause: (?), (?), ...
	seedValues := strings.Repeat("(?),", len(seedIDs))
	seedValues = seedValues[:len(seedValues)-1]

	// Args layout depends on whether decay is active:
	//   disabled: [seed IDs], threshold, depth, maxFacts
	//   enabled:  [seed IDs], horizonSec, threshold, depth, maxFacts
	args := make([]any, 0, len(seedIDs)+4)
	for _, id := range seedIDs {
		args = append(args, id)
	}
	if horizonDays > 0 {
		args = append(args, int64(horizonDays)*86400)
	}
	args = append(args, threshold, depth, maxFacts)

	q := fmt.Sprintf(`
		WITH RECURSIVE
			seeds(entity_id) AS (VALUES %s),
			neighborhood(entity_id, depth) AS (
				SELECT entity_id, 0 FROM seeds
				UNION
				SELECT
					CASE WHEN r.source_id = n.entity_id THEN r.target_id
					     ELSE r.source_id END,
					n.depth + 1
				FROM neighborhood n
				JOIN relationships r
					ON (r.source_id = n.entity_id OR r.target_id = n.entity_id)
				   AND %s >= ?
				WHERE n.depth < ?
			),
			dedup_neighborhood AS (
				SELECT entity_id, MIN(depth) AS depth
				FROM neighborhood
				GROUP BY entity_id
			)
		SELECT e.name, e.type, COALESCE(e.description, '')
		FROM dedup_neighborhood dn
		JOIN entities e ON e.id = dn.entity_id
		ORDER BY dn.depth ASC, e.updated_at DESC
		LIMIT ?`, seedValues, weightExpr(horizonDays))

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("traverseNeighborhood: %w", err)
	}
	defer rows.Close()

	var out []recalledEntity
	for rows.Next() {
		var e recalledEntity
		if err := rows.Scan(&e.Name, &e.Type, &e.Description); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// enumerateRelationships fetches all relationships where BOTH source_id
// and target_id are inside the given entity ID set, filtered by effective
// weight (decay-aware) >= threshold, sorted by effective weight DESC
// then source-name ASC then target-name ASC, capped at limit. Returns
// joined rows (source name, predicate, target name, RAW weight) ready
// for formatting into the fenced block.
//
// horizonDays controls Phase 3.E.6 decay (<= 0 disables). The SELECT
// returns r.weight (raw) so the fence shows honest pre-decay numbers
// to the operator; decay only affects ranking + filtering.
//
// AND (not OR) on the IN clauses: we want relationships WITHIN the
// neighborhood, not ones that merely touch it.
func enumerateRelationships(
	ctx context.Context,
	db *sql.DB,
	neighborhoodIDs []int64,
	threshold float64,
	limit int,
	horizonDays int,
) ([]recalledRel, error) {
	if len(neighborhoodIDs) == 0 {
		return nil, nil
	}

	placeholders := strings.Repeat("?,", len(neighborhoodIDs))
	placeholders = placeholders[:len(placeholders)-1]

	// Args layout:
	//   disabled: [source IN], [target IN], threshold, limit
	//   enabled:  [source IN], [target IN], horizonSec_WHERE, threshold,
	//             horizonSec_ORDER, limit
	// Two horizon binds because SQLite positional placeholders don't share
	// across clauses.
	args := make([]any, 0, 2*len(neighborhoodIDs)+4)
	for _, id := range neighborhoodIDs {
		args = append(args, id)
	}
	for _, id := range neighborhoodIDs {
		args = append(args, id)
	}
	horizonSec := int64(horizonDays) * 86400
	if horizonDays > 0 {
		args = append(args, horizonSec)
	}
	args = append(args, threshold)
	if horizonDays > 0 {
		args = append(args, horizonSec)
	}
	args = append(args, limit)

	expr := weightExpr(horizonDays)
	q := fmt.Sprintf(`
		SELECT e1.name, r.predicate, e2.name, r.weight
		FROM relationships r
		JOIN entities e1 ON r.source_id = e1.id
		JOIN entities e2 ON r.target_id = e2.id
		WHERE r.source_id IN (%s)
		  AND r.target_id IN (%s)
		  AND %s >= ?
		ORDER BY %s DESC, e1.name ASC, e2.name ASC
		LIMIT ?`, placeholders, placeholders, expr, expr)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("enumerateRelationships: %w", err)
	}
	defer rows.Close()

	var out []recalledRel
	for rows.Next() {
		var r recalledRel
		if err := rows.Scan(&r.Source, &r.Predicate, &r.Target, &r.Weight); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
