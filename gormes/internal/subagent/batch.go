package subagent

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// spawnBatch runs at most maxConcurrent subagents in parallel and preserves
// input order in its returned result slice.
func (m *manager) spawnBatch(ctx context.Context, cfgs []SubagentConfig, maxConcurrent int) ([]*SubagentResult, error) {
	if len(cfgs) == 0 {
		return nil, nil
	}
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrent
	}

	results := make([]*SubagentResult, len(cfgs))
	sem := make(chan struct{}, maxConcurrent)
	g, gctx := errgroup.WithContext(ctx)

	for i := range cfgs {
		i, cfg := i, cfgs[i]
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-gctx.Done():
				results[i] = &SubagentResult{
					Status:     StatusInterrupted,
					ExitReason: "batch_ctx_cancelled",
					Error:      gctx.Err().Error(),
				}
				return nil
			}
			defer func() { <-sem }()

			sa, err := m.Spawn(gctx, cfg)
			if err != nil {
				results[i] = &SubagentResult{
					Status:     StatusError,
					ExitReason: "spawn_failed",
					Error:      err.Error(),
				}
				return nil
			}

			result, err := sa.WaitForResult(gctx)
			if err != nil {
				results[i] = &SubagentResult{
					ID:         sa.ID,
					Status:     StatusInterrupted,
					ExitReason: "batch_ctx_cancelled",
					Error:      err.Error(),
				}
				return nil
			}
			results[i] = result
			return nil
		})
	}

	_ = g.Wait()
	return results, nil
}
