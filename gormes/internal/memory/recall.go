package memory

import (
	"context"
	"log/slog"
)

// RecallConfig controls the seed + CTE parameters.
type RecallConfig struct {
	WeightThreshold float64 // default 1.0 when <= 0
	MaxFacts        int     // default 10 when <= 0
	Depth           int     // default 2 when <= 0
	MaxSeeds        int     // default 5 when <= 0
}

func (c *RecallConfig) withDefaults() {
	if c.WeightThreshold <= 0 {
		c.WeightThreshold = 1.0
	}
	if c.MaxFacts <= 0 {
		c.MaxFacts = 10
	}
	if c.Depth <= 0 {
		c.Depth = 2
	}
	if c.MaxSeeds <= 0 {
		c.MaxSeeds = 5
	}
}

// RecallInput is the data the kernel passes to GetContext. This type
// is the memory-package-local counterpart to kernel.RecallParams.
// Keeping it here (rather than importing kernel) preserves the
// dependency arrow: kernel declares the interface it consumes; memory
// provides the impl that implements the interface. cmd/gormes/telegram.go
// adapts between them at the wire point.
type RecallInput struct {
	UserMessage string
	ChatKey     string
	SessionID   string
}

// Provider is the Phase-3.C recall orchestrator.
type Provider struct {
	store *SqliteStore
	cfg   RecallConfig
	log   *slog.Logger
}

func NewRecall(s *SqliteStore, cfg RecallConfig, log *slog.Logger) *Provider {
	cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Provider{store: s, cfg: cfg, log: log}
}

// GetContext is the single public entry point. Best-effort: any internal
// error results in "" (no context injected) with a WARN log. Caller
// bounds us via ctx (typically 100ms).
func (p *Provider) GetContext(ctx context.Context, in RecallInput) string {
	if err := ctx.Err(); err != nil {
		return ""
	}

	// 1. Layer-1 seed selection — exact name match.
	candidates := extractCandidates(in.UserMessage)
	seeds, err := seedsExactName(ctx, p.store.db, candidates, p.cfg.MaxSeeds)
	if err != nil {
		p.log.Warn("recall: Layer-1 seed query failed", "err", err)
		return ""
	}

	// 2. Layer-2 fallback if Layer-1 didn't get enough.
	if len(seeds) < 2 {
		fts, err := seedsFTS5(ctx, p.store.db, in.UserMessage, in.ChatKey, p.cfg.MaxSeeds)
		if err != nil {
			p.log.Warn("recall: Layer-2 FTS5 query failed", "err", err)
			// Continue with whatever Layer-1 gave us.
		} else {
			seen := make(map[int64]struct{}, len(seeds))
			for _, id := range seeds {
				seen[id] = struct{}{}
			}
			for _, id := range fts {
				if _, dup := seen[id]; !dup {
					seeds = append(seeds, id)
					seen[id] = struct{}{}
				}
				if len(seeds) >= p.cfg.MaxSeeds {
					break
				}
			}
		}
	}

	if len(seeds) == 0 {
		return ""
	}

	// 3. CTE traversal.
	entities, err := traverseNeighborhood(ctx, p.store.db,
		seeds, p.cfg.Depth, p.cfg.WeightThreshold, p.cfg.MaxFacts)
	if err != nil {
		p.log.Warn("recall: CTE traversal failed", "err", err)
		return ""
	}
	if len(entities) == 0 {
		return ""
	}

	// 4. Relationship enumeration — look up neighborhood IDs by name.
	neighborhoodIDs, err := p.idsForNames(ctx, entities)
	if err != nil {
		p.log.Warn("recall: id-lookup for rels failed", "err", err)
		return ""
	}
	rels, err := enumerateRelationships(ctx, p.store.db,
		neighborhoodIDs, p.cfg.WeightThreshold, p.cfg.MaxFacts)
	if err != nil {
		p.log.Warn("recall: relationship enumeration failed", "err", err)
		return ""
	}

	// 5. Format.
	return formatContextBlock(entities, rels)
}

// idsForNames resolves entity IDs for a set of recalledEntities.
// Uses (name, type) as the natural key — matches the UNIQUE constraint.
func (p *Provider) idsForNames(ctx context.Context, ents []recalledEntity) ([]int64, error) {
	if len(ents) == 0 {
		return nil, nil
	}
	const limitQ = 100 // defensive cap
	args := make([]any, 0, 2*len(ents)+1)
	parts := make([]string, 0, len(ents))
	for _, e := range ents {
		args = append(args, e.Name, e.Type)
		parts = append(parts, "(name = ? AND type = ?)")
	}
	args = append(args, limitQ)
	q := "SELECT id FROM entities WHERE " +
		joinWithOr(parts) +
		" LIMIT ?"
	rows, err := p.store.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}

func joinWithOr(parts []string) string {
	if len(parts) == 0 {
		return "0"
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += " OR " + parts[i]
	}
	return out
}
