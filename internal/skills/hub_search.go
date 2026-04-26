package skills

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// HubSearchResult is a single entry returned by a registry provider while
// browsing the skills hub. Field names are stable across the gateway/RPC
// boundary so that downstream slices (Search, gateway dispatch) can rely on a
// wire-compatible shape.
type HubSearchResult struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Source      string  `json:"source"`
	InstallID   string  `json:"install_id"`
	Score       float64 `json:"score"`
}

// HubRegistryProvider yields a deterministic read-only snapshot of search
// results from a single registry source. Implementations must not mutate the
// active or inactive skill stores: the snapshot is a read-model used by the
// upcoming Search() function over multiple providers.
type HubRegistryProvider interface {
	Snapshot(ctx context.Context) ([]HubSearchResult, error)
}

// Sentinel errors so downstream slices can table-test degraded evidence
// without depending on string matching or live network behaviour. The text
// matches the wire codes used by the future HubSearchResponse.Evidence field.
var (
	ErrRegistryUnavailable = errors.New("registry_unavailable")
	ErrRegistryRateLimited = errors.New("registry_rate_limited")
)

// InMemoryRegistryProvider is a deterministic test double that returns a
// preconfigured slice of HubSearchResult entries (sorted by Name ascending)
// or a preconfigured error. It is the only provider implementation in this
// slice; live registries land in later rows.
type InMemoryRegistryProvider struct {
	results []HubSearchResult
	err     error
}

// NewInMemoryRegistryProvider returns a provider that yields a defensive copy
// of the given results sorted by Name ascending. If err is non-nil, Snapshot
// returns it unchanged and the results slice is ignored on the read path.
func NewInMemoryRegistryProvider(results []HubSearchResult, err error) *InMemoryRegistryProvider {
	sorted := make([]HubSearchResult, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	return &InMemoryRegistryProvider{results: sorted, err: err}
}

// Snapshot returns the configured error when set, otherwise a fresh copy of
// the deterministic results slice. The copy prevents callers from mutating
// the provider's view between calls.
func (p *InMemoryRegistryProvider) Snapshot(_ context.Context) ([]HubSearchResult, error) {
	if p == nil {
		return nil, nil
	}
	if p.err != nil {
		return nil, p.err
	}
	out := make([]HubSearchResult, len(p.results))
	copy(out, p.results)
	return out, nil
}

// HubSearchEvidence is a typed enum reported by Search to describe degraded
// outcomes (empty query, unavailable registry, rate limited registry, no
// matches). Callers must inspect Evidence rather than assume an empty Results
// slice means failure.
type HubSearchEvidence string

const (
	HubSearchEvidenceEmptyQuery          HubSearchEvidence = "empty_query"
	HubSearchEvidenceRegistryUnavailable HubSearchEvidence = "registry_unavailable"
	HubSearchEvidenceRateLimited         HubSearchEvidence = "registry_rate_limited"
	HubSearchEvidenceNoResults           HubSearchEvidence = "no_results"
)

// HubSearchOptions are the read-side options accepted by Search. Reserved for
// downstream slices that need source filters or limits; the current row has
// no required behaviour for any field.
type HubSearchOptions struct {
	// Limit caps the merged result list after dedupe and sort. Zero means no cap.
	Limit int
}

// HubSearchResponse pairs the sorted, deduped result list with a typed
// evidence value describing degraded conditions. The shape is wire-stable so
// gateway and TUI slices can serialise it directly.
type HubSearchResponse struct {
	Results  []HubSearchResult `json:"results"`
	Evidence HubSearchEvidence `json:"evidence,omitempty"`
}

// Search merges the read-only snapshots from each provider, filters them by
// substring match on Name+Description, dedupes by InstallID, and sorts by
// Score descending then Name ascending. It never touches the active or
// inactive skill stores and never opens a network connection — providers are
// the only seam to live data.
func Search(ctx context.Context, query string, providers []HubRegistryProvider, opts HubSearchOptions) (HubSearchResponse, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return HubSearchResponse{Evidence: HubSearchEvidenceEmptyQuery}, nil
	}

	needle := strings.ToLower(trimmed)

	var (
		merged       []HubSearchResult
		unavailable  bool
		rateLimited  bool
	)

	for _, p := range providers {
		if p == nil {
			continue
		}
		snap, err := p.Snapshot(ctx)
		if err != nil {
			switch {
			case errors.Is(err, ErrRegistryUnavailable):
				unavailable = true
			case errors.Is(err, ErrRegistryRateLimited):
				rateLimited = true
			default:
				return HubSearchResponse{}, err
			}
			continue
		}
		for _, r := range snap {
			haystack := strings.ToLower(r.Name + " " + r.Description)
			if strings.Contains(haystack, needle) {
				merged = append(merged, r)
			}
		}
	}

	deduped := make([]HubSearchResult, 0, len(merged))
	indexByInstallID := make(map[string]int, len(merged))
	for _, r := range merged {
		if r.InstallID == "" {
			deduped = append(deduped, r)
			continue
		}
		if i, ok := indexByInstallID[r.InstallID]; ok {
			if r.Score > deduped[i].Score {
				deduped[i] = r
			}
			continue
		}
		indexByInstallID[r.InstallID] = len(deduped)
		deduped = append(deduped, r)
	}

	sort.SliceStable(deduped, func(i, j int) bool {
		if deduped[i].Score != deduped[j].Score {
			return deduped[i].Score > deduped[j].Score
		}
		return deduped[i].Name < deduped[j].Name
	})

	if opts.Limit > 0 && len(deduped) > opts.Limit {
		deduped = deduped[:opts.Limit]
	}

	var evidence HubSearchEvidence
	switch {
	case unavailable:
		evidence = HubSearchEvidenceRegistryUnavailable
	case rateLimited:
		evidence = HubSearchEvidenceRateLimited
	case len(deduped) == 0:
		evidence = HubSearchEvidenceNoResults
	}

	return HubSearchResponse{Results: deduped, Evidence: evidence}, nil
}
