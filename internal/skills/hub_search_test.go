package skills

import (
	"context"
	"testing"
)

// TestHubSearchEmptyQueryReturnsEmptyEvidence asserts that an empty or
// whitespace-only query short-circuits before touching providers and reports
// HubSearchEvidenceEmptyQuery instead of forwarding any results.
func TestHubSearchEmptyQueryReturnsEmptyEvidence(t *testing.T) {
	provider := NewInMemoryRegistryProvider([]HubSearchResult{
		{Name: "alpha", Description: "alpha skill", Source: "fixture", InstallID: "fixture/alpha", Score: 0.50},
	}, nil)

	for _, query := range []string{"", "   ", "\t\n  "} {
		resp, err := Search(context.Background(), query, []HubRegistryProvider{provider}, HubSearchOptions{})
		if err != nil {
			t.Fatalf("Search(query=%q) returned unexpected error: %v", query, err)
		}
		if resp.Evidence != HubSearchEvidenceEmptyQuery {
			t.Errorf("Search(query=%q) Evidence = %q, want %q", query, resp.Evidence, HubSearchEvidenceEmptyQuery)
		}
		if len(resp.Results) != 0 {
			t.Errorf("Search(query=%q) Results = %v, want empty", query, resp.Results)
		}
	}
}

// TestHubSearchSortsAndDedupes asserts that results from multiple providers
// are merged, deduped by InstallID, and sorted by Score descending then Name
// ascending.
func TestHubSearchSortsAndDedupes(t *testing.T) {
	p1 := NewInMemoryRegistryProvider([]HubSearchResult{
		{Name: "alpha", Description: "alpha skill", Source: "p1", InstallID: "p/alpha", Score: 0.30},
		{Name: "duplicate", Description: "shared skill", Source: "p1", InstallID: "shared/dup", Score: 0.50},
		{Name: "zeta", Description: "zeta skill", Source: "p1", InstallID: "p/zeta", Score: 0.80},
	}, nil)
	p2 := NewInMemoryRegistryProvider([]HubSearchResult{
		{Name: "alpha2", Description: "alpha2 skill", Source: "p2", InstallID: "p/alpha2", Score: 0.30},
		{Name: "duplicate", Description: "shared skill", Source: "p2", InstallID: "shared/dup", Score: 0.50},
		{Name: "mu", Description: "mu skill", Source: "p2", InstallID: "p/mu", Score: 0.50},
	}, nil)

	resp, err := Search(context.Background(), "skill", []HubRegistryProvider{p1, p2}, HubSearchOptions{})
	if err != nil {
		t.Fatalf("Search returned unexpected error: %v", err)
	}
	if resp.Evidence == HubSearchEvidenceEmptyQuery ||
		resp.Evidence == HubSearchEvidenceRegistryUnavailable ||
		resp.Evidence == HubSearchEvidenceRateLimited ||
		resp.Evidence == HubSearchEvidenceNoResults {
		t.Errorf("Evidence = %q, want a non-degraded value", resp.Evidence)
	}

	wantOrder := []string{"zeta", "duplicate", "mu", "alpha", "alpha2"}
	gotOrder := make([]string, 0, len(resp.Results))
	for _, r := range resp.Results {
		gotOrder = append(gotOrder, r.Name)
	}
	if len(gotOrder) != len(wantOrder) {
		t.Fatalf("Results length = %d (%v), want %d (%v)", len(gotOrder), gotOrder, len(wantOrder), wantOrder)
	}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Errorf("Results[%d].Name = %q, want %q (full=%v)", i, gotOrder[i], wantOrder[i], gotOrder)
		}
	}

	dupCount := 0
	for _, r := range resp.Results {
		if r.InstallID == "shared/dup" {
			dupCount++
		}
	}
	if dupCount != 1 {
		t.Errorf("shared/dup appeared %d times, want 1", dupCount)
	}
}

// TestHubSearchRegistryUnavailable asserts that Search keeps results from
// healthy providers when one provider returns ErrRegistryUnavailable, while
// still surfacing the degraded condition via Evidence.
func TestHubSearchRegistryUnavailable(t *testing.T) {
	failed := NewInMemoryRegistryProvider([]HubSearchResult{
		{Name: "should-not-appear", InstallID: "broken/x", Score: 1.0},
	}, ErrRegistryUnavailable)
	healthy := NewInMemoryRegistryProvider([]HubSearchResult{
		{Name: "alpha", Description: "alpha skill", Source: "ok", InstallID: "ok/alpha", Score: 0.50},
	}, nil)

	resp, err := Search(context.Background(), "alpha", []HubRegistryProvider{failed, healthy}, HubSearchOptions{})
	if err != nil {
		t.Fatalf("Search returned unexpected error: %v", err)
	}
	if resp.Evidence != HubSearchEvidenceRegistryUnavailable {
		t.Errorf("Evidence = %q, want %q", resp.Evidence, HubSearchEvidenceRegistryUnavailable)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("Results count = %d (%v), want 1", len(resp.Results), resp.Results)
	}
	if resp.Results[0].InstallID != "ok/alpha" {
		t.Errorf("Results[0].InstallID = %q, want %q", resp.Results[0].InstallID, "ok/alpha")
	}
}

// TestHubSearchNoResults asserts that a non-empty query with no provider
// matches reports HubSearchEvidenceNoResults instead of leaving callers to
// guess from an empty Results slice.
func TestHubSearchNoResults(t *testing.T) {
	provider := NewInMemoryRegistryProvider([]HubSearchResult{
		{Name: "alpha", Description: "alpha skill", Source: "fixture", InstallID: "fixture/alpha", Score: 0.50},
		{Name: "beta", Description: "beta skill", Source: "fixture", InstallID: "fixture/beta", Score: 0.40},
	}, nil)

	resp, err := Search(context.Background(), "no-such-skill-anywhere", []HubRegistryProvider{provider}, HubSearchOptions{})
	if err != nil {
		t.Fatalf("Search returned unexpected error: %v", err)
	}
	if resp.Evidence != HubSearchEvidenceNoResults {
		t.Errorf("Evidence = %q, want %q", resp.Evidence, HubSearchEvidenceNoResults)
	}
	if len(resp.Results) != 0 {
		t.Errorf("Results = %v, want empty", resp.Results)
	}
}
