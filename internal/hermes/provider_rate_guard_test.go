package hermes

import (
	"net/http"
	"testing"
	"time"
)

func TestProviderRateGuard(t *testing.T) {
	t0 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	t.Run("genuine_quota_records_redacted_reset_evidence", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Requests-1h", "0")
		h.Set("X-RateLimit-Reset-Requests-1h", "1800")
		h.Set("X-RateLimit-Remaining-Requests", "5")
		h.Set("X-RateLimit-Reset-Requests", "10")
		h.Set("Retry-After", "120")

		got := DecideRateGuard(h, GuardState{})
		if got.Class != RateLimitGenuineQuota {
			t.Fatalf("Class: got %q, want %q", got.Class, RateLimitGenuineQuota)
		}
		if got.Source != "headers" {
			t.Fatalf("Source: got %q, want %q", got.Source, "headers")
		}
		if got.ResetSeconds != 1800 {
			t.Fatalf("ResetSeconds: got %d, want 1800 (redacted reset window)", got.ResetSeconds)
		}
		if got.ResetBucket != "requests-1h" {
			t.Fatalf("ResetBucket: got %q, want %q", got.ResetBucket, "requests-1h")
		}
		if !hasTag(got.EvidenceTags, StatusNousRateLimited) {
			t.Fatalf("evidence missing %q: %v", StatusNousRateLimited, got.EvidenceTags)
		}
		if hasTag(got.EvidenceTags, StatusBudgetHeaderMissing) {
			t.Fatalf("evidence must not include %q when buckets parsed: %v", StatusBudgetHeaderMissing, got.EvidenceTags)
		}
		if got.Slept {
			t.Fatal("Slept must be false: rate guard never sleeps in unit tests")
		}
		if got.Budget.HeaderMissing {
			t.Fatal("Budget.HeaderMissing must be false when buckets parsed")
		}
		if !hasString(got.Budget.Exhausted, "requests-1h") {
			t.Fatalf("Budget.Exhausted must include requests-1h: got %v", got.Budget.Exhausted)
		}
		if hasString(got.Budget.Exhausted, "requests") {
			t.Fatalf("Budget.Exhausted must not include short-reset bucket requests: got %v", got.Budget.Exhausted)
		}
	})

	t.Run("upstream_capacity_does_not_trip_breaker", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Requests", "198")
		h.Set("X-RateLimit-Reset-Requests", "40")
		h.Set("X-RateLimit-Remaining-Requests-1h", "750")
		h.Set("X-RateLimit-Reset-Requests-1h", "3100")
		h.Set("X-RateLimit-Remaining-Tokens", "790000")
		h.Set("X-RateLimit-Reset-Tokens", "40")

		// Even when last-known says GenuineQuota, fresh healthy headers must
		// override and classify as upstream_capacity.
		got := DecideRateGuard(h, GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0})
		if got.Class != RateLimitUpstreamCapacity {
			t.Fatalf("Class: got %q, want %q (fresh healthy headers must override stale last_known)", got.Class, RateLimitUpstreamCapacity)
		}
		if got.Source != "headers" {
			t.Fatalf("Source: got %q, want %q", got.Source, "headers")
		}
		if got.ResetSeconds != 0 {
			t.Fatalf("ResetSeconds: got %d, want 0 for upstream_capacity", got.ResetSeconds)
		}
		if got.ResetBucket != "" {
			t.Fatalf("ResetBucket: got %q, want empty for upstream_capacity", got.ResetBucket)
		}
		if !hasTag(got.EvidenceTags, StatusNousUpstreamCapacity) {
			t.Fatalf("evidence missing %q: %v", StatusNousUpstreamCapacity, got.EvidenceTags)
		}
		if hasTag(got.EvidenceTags, StatusNousRateLimited) {
			t.Fatalf("evidence must not include %q for healthy buckets: %v", StatusNousRateLimited, got.EvidenceTags)
		}
		if hasTag(got.EvidenceTags, StatusBudgetHeaderMissing) {
			t.Fatalf("evidence must not include %q when buckets parsed: %v", StatusBudgetHeaderMissing, got.EvidenceTags)
		}
		if got.Slept {
			t.Fatal("Slept must be false: upstream_capacity must not sleep or block other models")
		}
	})

	t.Run("bare_429_with_last_known_genuine_classifies_genuine", func(t *testing.T) {
		got := DecideRateGuard(http.Header{}, GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0})
		if got.Class != RateLimitGenuineQuota {
			t.Fatalf("Class: got %q, want %q (bare 429 falls back to last_known genuine)", got.Class, RateLimitGenuineQuota)
		}
		if got.Source != "last_known" {
			t.Fatalf("Source: got %q, want %q", got.Source, "last_known")
		}
		if !got.LastKnownPresent {
			t.Fatal("LastKnownPresent must be true when last.LastKnownClass is set")
		}
		if !hasTag(got.EvidenceTags, StatusNousRateLimited) {
			t.Fatalf("evidence missing %q: %v", StatusNousRateLimited, got.EvidenceTags)
		}
		if !hasTag(got.EvidenceTags, StatusBudgetHeaderMissing) {
			t.Fatalf("evidence missing %q (no budget headers on bare 429): %v", StatusBudgetHeaderMissing, got.EvidenceTags)
		}
		if got.ResetSeconds != 0 {
			t.Fatalf("ResetSeconds: got %d, want 0 (no fresh header evidence)", got.ResetSeconds)
		}
		if got.ResetBucket != "" {
			t.Fatalf("ResetBucket: got %q, want empty (no fresh header evidence)", got.ResetBucket)
		}
		if got.Slept {
			t.Fatal("Slept must be false")
		}
	})

	t.Run("bare_429_with_last_known_upstream_classifies_upstream", func(t *testing.T) {
		got := DecideRateGuard(http.Header{}, GuardState{LastKnownClass: RateLimitUpstreamCapacity, LastKnownAt: t0})
		if got.Class != RateLimitUpstreamCapacity {
			t.Fatalf("Class: got %q, want %q (healthy last_known keeps bare 429 as upstream_capacity)", got.Class, RateLimitUpstreamCapacity)
		}
		if got.Source != "last_known" {
			t.Fatalf("Source: got %q, want %q", got.Source, "last_known")
		}
		if !hasTag(got.EvidenceTags, StatusNousUpstreamCapacity) {
			t.Fatalf("evidence missing %q: %v", StatusNousUpstreamCapacity, got.EvidenceTags)
		}
		if !hasTag(got.EvidenceTags, StatusBudgetHeaderMissing) {
			t.Fatalf("evidence missing %q (no budget headers on bare 429): %v", StatusBudgetHeaderMissing, got.EvidenceTags)
		}
		if hasTag(got.EvidenceTags, StatusNousRateLimited) {
			t.Fatalf("evidence must not include %q for healthy last_known: %v", StatusNousRateLimited, got.EvidenceTags)
		}
		if got.Slept {
			t.Fatal("Slept must be false")
		}
	})

	t.Run("missing_headers_no_state_degrades_visibly", func(t *testing.T) {
		got := DecideRateGuard(http.Header{}, GuardState{})
		if got.Class != RateLimitInsufficientEvidence {
			t.Fatalf("Class: got %q, want %q", got.Class, RateLimitInsufficientEvidence)
		}
		if got.Source != "missing" {
			t.Fatalf("Source: got %q, want %q", got.Source, "missing")
		}
		if got.LastKnownPresent {
			t.Fatal("LastKnownPresent must be false on empty GuardState")
		}
		if !hasTag(got.EvidenceTags, StatusRateGuardUnavailable) {
			t.Fatalf("evidence missing %q: %v", StatusRateGuardUnavailable, got.EvidenceTags)
		}
		if !hasTag(got.EvidenceTags, StatusBudgetHeaderMissing) {
			t.Fatalf("evidence missing %q: %v", StatusBudgetHeaderMissing, got.EvidenceTags)
		}
		if got.Slept {
			t.Fatal("Slept must be false: degraded mode never sleeps or amplifies retries")
		}
	})

	t.Run("malformed_headers_degrade_visibly_without_treating_as_zero", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Tokens", "abc")
		h.Set("X-RateLimit-Reset-Tokens", "not-a-number")

		got := DecideRateGuard(h, GuardState{})
		if got.Class != RateLimitInsufficientEvidence {
			t.Fatalf("Class: got %q, want %q (malformed values must not be coerced to zero)", got.Class, RateLimitInsufficientEvidence)
		}
		if got.Source != "missing" {
			t.Fatalf("Source: got %q, want %q", got.Source, "missing")
		}
		if !hasTag(got.EvidenceTags, StatusRateGuardUnavailable) {
			t.Fatalf("evidence missing %q: %v", StatusRateGuardUnavailable, got.EvidenceTags)
		}
		if !hasTag(got.EvidenceTags, StatusBudgetHeaderMissing) {
			t.Fatalf("evidence missing %q: %v", StatusBudgetHeaderMissing, got.EvidenceTags)
		}
		if got.Slept {
			t.Fatal("Slept must be false: malformed headers must not amplify retries")
		}
	})

	t.Run("decision_is_pure_no_global_state", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Requests-1h", "0")
		h.Set("X-RateLimit-Reset-Requests-1h", "1800")

		first := DecideRateGuard(h, GuardState{})
		second := DecideRateGuard(h, GuardState{})

		if first.Class != second.Class {
			t.Fatalf("Class diverged across calls: first=%q second=%q", first.Class, second.Class)
		}
		if first.ResetSeconds != second.ResetSeconds {
			t.Fatalf("ResetSeconds diverged: first=%d second=%d", first.ResetSeconds, second.ResetSeconds)
		}
		if first.ResetBucket != second.ResetBucket {
			t.Fatalf("ResetBucket diverged: first=%q second=%q", first.ResetBucket, second.ResetBucket)
		}
		if first.Slept || second.Slept {
			t.Fatal("Slept must be false on every call")
		}
	})

	t.Run("never_sleeps_under_repeated_classification", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Requests-1h", "0")
		h.Set("X-RateLimit-Reset-Requests-1h", "1800")

		start := time.Now()
		for i := 0; i < 200; i++ {
			d := DecideRateGuard(h, GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0})
			if d.Slept {
				t.Fatalf("iteration %d: Slept must be false", i)
			}
		}
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			t.Fatalf("200 classifications took %v, expected <100ms (no wall-clock sleeps)", elapsed)
		}
	})

	t.Run("parse_budget_returns_per_bucket_evidence", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Requests", "180")
		h.Set("X-RateLimit-Reset-Requests", "30")
		h.Set("X-RateLimit-Remaining-Tokens-1h", "0")
		h.Set("X-RateLimit-Reset-Tokens-1h", "120")

		snap := ParseBudget(h)
		if !snap.AnyParsed {
			t.Fatal("AnyParsed must be true with two parsed buckets")
		}
		if snap.HeaderMissing {
			t.Fatal("HeaderMissing must be false when AnyParsed is true")
		}
		if !hasString(snap.Exhausted, "tokens-1h") {
			t.Fatalf("Exhausted must contain tokens-1h: got %v", snap.Exhausted)
		}
		if hasString(snap.Exhausted, "requests") {
			t.Fatalf("Exhausted must not contain healthy bucket requests: got %v", snap.Exhausted)
		}
		var (
			seenRequests bool
			seenTokens1h bool
		)
		for _, b := range snap.Buckets {
			switch b.Tag {
			case "requests":
				seenRequests = true
				if !b.HasRemaining || b.Remaining != 180 {
					t.Fatalf("requests bucket Remaining: got (%d, has=%v), want (180, true)", b.Remaining, b.HasRemaining)
				}
				if b.Exhausted() {
					t.Fatal("requests bucket must not be Exhausted (healthy)")
				}
			case "tokens-1h":
				seenTokens1h = true
				if !b.HasRemaining || b.Remaining != 0 {
					t.Fatalf("tokens-1h bucket Remaining: got (%d, has=%v), want (0, true)", b.Remaining, b.HasRemaining)
				}
				if !b.Exhausted() {
					t.Fatal("tokens-1h bucket must be Exhausted (remaining=0, reset=120)")
				}
			}
		}
		if !seenRequests || !seenTokens1h {
			t.Fatalf("Buckets missing requests or tokens-1h: %+v", snap.Buckets)
		}
	})

	t.Run("parse_budget_missing_headers_marks_header_missing", func(t *testing.T) {
		snap := ParseBudget(http.Header{})
		if snap.AnyParsed {
			t.Fatal("AnyParsed must be false on empty headers")
		}
		if !snap.HeaderMissing {
			t.Fatal("HeaderMissing must be true on empty headers")
		}
		if len(snap.Buckets) != 0 {
			t.Fatalf("Buckets must be empty on empty headers: got %+v", snap.Buckets)
		}
		if len(snap.Exhausted) != 0 {
			t.Fatalf("Exhausted must be empty on empty headers: got %+v", snap.Exhausted)
		}
	})
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func hasString(items []string, want string) bool {
	return hasTag(items, want)
}
