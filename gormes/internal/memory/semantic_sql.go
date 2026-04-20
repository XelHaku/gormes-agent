package memory

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
)

// semanticCache is an in-memory cache of (entity_id, L2-normalized vector,
// model) rows from entity_embeddings. Loaded lazily on first semanticSeeds
// call; rebuilt when the graph-version counter changes.
//
// Invalidation protocol: every write to entities or entity_embeddings must
// call cache.bump() afterward. The next ensureLoaded detects the version
// drift (loadedAt != graphVersion) and rebuilds from SQLite. This is a
// "generation counter" pattern — simple, no distributed coordination needed.
type semanticCache struct {
	// graphVersion is incremented on every write to entities or
	// entity_embeddings. Cached entries are tagged with the version
	// they were loaded at; a mismatch triggers a rebuild.
	graphVersion atomic.Uint64

	mu       sync.Mutex
	loadedAt uint64 // graphVersion at last load
	entries  []cacheEntry
	byModel  string // the model whose vectors this cache currently holds
}

// cacheEntry holds a single row from entity_embeddings: the entity ID and
// its L2-normalized float32 vector. Vectors are immutable once written into
// the cache (the cache is rebuilt wholesale on bump, never mutated in place),
// so they can be iterated without the lock after snapshotting the slice header.
type cacheEntry struct {
	entityID int64
	vec      []float32
}

// newSemanticCache allocates an empty cache. graphVersion starts at 0;
// loadedAt starts at 0 too — but entries is nil, which forces the first
// ensureLoaded to query SQLite even when the version numbers happen to match.
func newSemanticCache() *semanticCache { return &semanticCache{} }

// bump invalidates the cache. Called by writers (Embedder on insert/update;
// the extractor's writeGraphBatch indirectly if it populates entity_embeddings).
// A single atomic add is sufficient: ensureLoaded will detect the drift on its
// next call regardless of which goroutine calls bump.
func (c *semanticCache) bump() { c.graphVersion.Add(1) }

// ensureLoaded rebuilds the in-memory cache if it is stale (wrong graphVersion
// or wrong model). Takes the mutex for the duration of the SQL scan so that
// concurrent callers block rather than issue duplicate queries.
//
// Defense-in-depth: corrupt BLOB rows (decodeFloat32LE error OR len(vec) != dim)
// are silently skipped. The embedder will re-populate them on the next run.
// This means recall may miss a small number of entities but never crashes.
func (c *semanticCache) ensureLoaded(ctx context.Context, db *sql.DB, model string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	currentVersion := c.graphVersion.Load()
	if c.loadedAt == currentVersion && c.byModel == model && c.entries != nil {
		return nil // fresh + same model; nothing to do
	}

	rows, err := db.QueryContext(ctx,
		`SELECT entity_id, dim, vec FROM entity_embeddings WHERE model = ?`, model)
	if err != nil {
		return err
	}
	defer rows.Close()

	var entries []cacheEntry
	for rows.Next() {
		var id int64
		var dim int
		var blob []byte
		if err := rows.Scan(&id, &dim, &blob); err != nil {
			return err
		}
		vec, err := decodeFloat32LE(blob)
		if err != nil || len(vec) != dim {
			// Corrupt row: either BLOB length is not a multiple of 4, or
			// the decoded length doesn't match the stored dim column. Skip
			// silently — bad data should not prevent good data from loading.
			continue
		}
		entries = append(entries, cacheEntry{entityID: id, vec: vec})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	c.loadedAt = currentVersion
	c.byModel = model
	c.entries = entries
	return nil
}

// semanticSeeds runs the Top-K cosine-similarity scan over the cached
// vectors for the given model. queryVec must already be L2-normalized by
// the caller (matching the invariant for stored vectors). Returns entity IDs
// whose similarity to queryVec meets minSimilarity, sorted DESC by score,
// at most topKCount results.
//
// Snapshot pattern: the mutex is held only long enough to copy the slice
// header (a three-word value: ptr + len + cap). The slice elements themselves
// are immutable once a cache generation is loaded — the cache is replaced
// wholesale on bump, never mutated in place — so iterating the snapshot
// without the lock is safe and avoids holding the lock during the O(n) scan.
//
// Empty DB returns (nil, nil) — not an error. Dim mismatch between queryVec
// and a cached entry skips that entry (model-switch race survives gracefully).
func semanticSeeds(
	ctx context.Context,
	db *sql.DB,
	cache *semanticCache,
	model string,
	queryVec []float32,
	topKCount int,
	minSimilarity float64,
) ([]int64, error) {
	if len(queryVec) == 0 || topKCount <= 0 {
		return nil, nil
	}

	if err := cache.ensureLoaded(ctx, db, model); err != nil {
		return nil, err
	}

	// Snapshot the slice header under the lock, then release immediately.
	// The entries themselves are immutable until the next bump+rebuild, so
	// reading them concurrently without the lock is safe (no data race).
	cache.mu.Lock()
	snapshot := cache.entries
	cache.mu.Unlock()

	if len(snapshot) == 0 {
		return nil, nil
	}

	scored := make([]scoredID, 0, len(snapshot))
	for _, e := range snapshot {
		if len(e.vec) != len(queryVec) {
			// Dim mismatch: the model may have changed between the cache
			// load and this query, or a row had an unexpected dimension.
			// Skip rather than returning a misleading score.
			continue
		}
		sim := dotProduct(queryVec, e.vec)
		if float64(sim) < minSimilarity {
			continue
		}
		scored = append(scored, scoredID{ID: e.entityID, Score: sim})
	}

	top := topK(scored, topKCount)
	ids := make([]int64, len(top))
	for i, s := range top {
		ids[i] = s.ID
	}
	return ids, nil
}
