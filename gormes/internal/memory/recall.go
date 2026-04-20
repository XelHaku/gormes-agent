package memory

import (
	"context"
	"log/slog"
	"time"
)

// RecallConfig controls the seed + CTE parameters.
type RecallConfig struct {
	WeightThreshold float64 // default 1.0 when <= 0
	MaxFacts        int     // default 10 when <= 0
	Depth           int     // default 2 when <= 0
	MaxSeeds        int     // default 5 when <= 0

	// Phase 3.D semantic fusion. All zero / empty = disabled.
	SemanticModel         string        // Ollama embedding model tag; "" disables the layer
	SemanticTopK          int           // default 3 when <= 0 and SemanticModel != ""
	SemanticMinSimilarity float64       // default 0.35 when <= 0 and SemanticModel != ""
	QueryEmbedTimeout     time.Duration // default 60ms when <= 0 and SemanticModel != ""

	// DecayHorizonDays — Phase 3.E.6. An edge's effective weight
	// decays linearly from 1.0×raw at age=0 to 0.0 at
	// age=DecayHorizonDays days. Applied to the recall path's
	// relationship WHERE/ORDER BY; the raw weight column is
	// untouched (decay is reversible by tweaking this knob).
	// Sentinel rules:
	//   0  — unset; withDefaults promotes to 180.
	//   >0 — preserved as the active horizon.
	//   <0 — preserved as the "disabled" signal; recall falls back
	//        to the legacy raw-weight filter.
	DecayHorizonDays int
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
	// Semantic defaults only apply when the feature is opted in.
	if c.SemanticModel != "" {
		if c.SemanticTopK <= 0 {
			c.SemanticTopK = 3
		}
		if c.SemanticMinSimilarity <= 0 {
			c.SemanticMinSimilarity = 0.35
		}
		if c.QueryEmbedTimeout <= 0 {
			c.QueryEmbedTimeout = 60 * time.Millisecond
		}
	}
	// Phase 3.E.6 — only promote zero to the default. Negative
	// values are preserved as the "decay disabled" sentinel.
	if c.DecayHorizonDays == 0 {
		c.DecayHorizonDays = 180
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

// Provider is the recall orchestrator. Use NewRecall to construct; wire the
// optional semantic layer via WithEmbedClient before any GetContext calls.
type Provider struct {
	store *SqliteStore
	cfg   RecallConfig
	log   *slog.Logger
	ec    *embedClient   // nil disables semantic recall
	cache *semanticCache // shared with the Embedder; always non-nil for consistency
}

func NewRecall(s *SqliteStore, cfg RecallConfig, log *slog.Logger) *Provider {
	cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Provider{store: s, cfg: cfg, log: log, cache: newSemanticCache()}
}

// WithEmbedClient attaches the embedding client. Call before Run() or
// any GetContext; not safe for concurrent use with in-flight recalls.
// Pass the same *semanticCache that the Embedder will bump to keep
// both consumers in sync.
func (p *Provider) WithEmbedClient(ec *embedClient, cache *semanticCache) *Provider {
	p.ec = ec
	if cache != nil {
		p.cache = cache
	}
	return p
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

	// 3. Semantic fallback — only if enabled AND we still need more seeds.
	if p.ec != nil && p.cfg.SemanticModel != "" && len(seeds) < p.cfg.MaxSeeds {
		semCtx, semCancel := context.WithTimeout(ctx, p.cfg.QueryEmbedTimeout)
		qvec, err := p.ec.Embed(semCtx, p.cfg.SemanticModel, in.UserMessage)
		semCancel()
		if err != nil {
			p.log.Warn("recall: query embed failed", "err", err)
		} else {
			l2Normalize(qvec)
			semIDs, err := semanticSeeds(ctx, p.store.db, p.cache,
				p.cfg.SemanticModel, qvec, p.cfg.SemanticTopK, p.cfg.SemanticMinSimilarity)
			if err != nil {
				p.log.Warn("recall: semantic scan failed", "err", err)
			} else {
				seen := make(map[int64]struct{}, len(seeds))
				for _, id := range seeds {
					seen[id] = struct{}{}
				}
				for _, id := range semIDs {
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
	}

	if len(seeds) == 0 {
		return ""
	}

	// 4. CTE traversal.
	entities, err := traverseNeighborhood(ctx, p.store.db,
		seeds, p.cfg.Depth, p.cfg.WeightThreshold, p.cfg.MaxFacts,
		p.cfg.DecayHorizonDays)
	if err != nil {
		p.log.Warn("recall: CTE traversal failed", "err", err)
		return ""
	}
	if len(entities) == 0 {
		return ""
	}

	// 5. Relationship enumeration — look up neighborhood IDs by name.
	neighborhoodIDs, err := p.idsForNames(ctx, entities)
	if err != nil {
		p.log.Warn("recall: id-lookup for rels failed", "err", err)
		return ""
	}
	rels, err := enumerateRelationships(ctx, p.store.db,
		neighborhoodIDs, p.cfg.WeightThreshold, p.cfg.MaxFacts,
		p.cfg.DecayHorizonDays)
	if err != nil {
		p.log.Warn("recall: relationship enumeration failed", "err", err)
		return ""
	}

	// 6. Format.
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
