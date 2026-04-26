package hermes

import (
	"net/http"
	"testing"
)

func TestClassify429(t *testing.T) {
	t.Run("genuine_quota_1h_reset", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Requests-1h", "0")
		h.Set("X-RateLimit-Reset-Requests-1h", "300")
		h.Set("X-RateLimit-Remaining-Requests", "5")
		h.Set("X-RateLimit-Reset-Requests", "10")
		h.Set("X-RateLimit-Remaining-Tokens", "1000")
		h.Set("X-RateLimit-Reset-Tokens", "30")
		got := Classify429(h)
		if got != RateLimitGenuineQuota {
			t.Fatalf("expected RateLimitGenuineQuota, got %q", got)
		}
	})

	t.Run("short_reset_upstream_capacity", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Requests", "0")
		h.Set("X-RateLimit-Reset-Requests", "30")
		got := Classify429(h)
		if got != RateLimitUpstreamCapacity {
			t.Fatalf("expected RateLimitUpstreamCapacity for sub-60s reset, got %q", got)
		}
	})

	t.Run("healthy_remaining_upstream_capacity", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Requests", "5")
		h.Set("X-RateLimit-Reset-Requests", "10")
		h.Set("X-RateLimit-Remaining-Tokens", "1000")
		h.Set("X-RateLimit-Reset-Tokens", "10")
		got := Classify429(h)
		if got != RateLimitUpstreamCapacity {
			t.Fatalf("expected RateLimitUpstreamCapacity for healthy buckets, got %q", got)
		}
	})

	t.Run("missing_headers_insufficient", func(t *testing.T) {
		h := http.Header{}
		got := Classify429(h)
		if got != RateLimitInsufficientEvidence {
			t.Fatalf("expected RateLimitInsufficientEvidence with no headers, got %q", got)
		}
		if string(got) == "" {
			t.Fatalf("expected non-empty classification string")
		}
	})

	t.Run("unknown_headers_ignored", func(t *testing.T) {
		h := http.Header{}
		h.Set("Retry-After", "120")
		h.Set("X-Custom-Foo", "bar")
		h.Set("X-RateLimit-Remaining-Requests", "5")
		h.Set("X-RateLimit-Reset-Requests", "10")
		got := Classify429(h)
		if got != RateLimitUpstreamCapacity {
			t.Fatalf("expected RateLimitUpstreamCapacity ignoring unknown headers, got %q", got)
		}
	})

	t.Run("malformed_values_ignored", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Tokens", "abc")
		got := Classify429(h)
		if got != RateLimitInsufficientEvidence {
			t.Fatalf("expected RateLimitInsufficientEvidence for malformed values, got %q", got)
		}
	})

	t.Run("three_buckets_with_remaining_one_missing_returns_upstream_capacity", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-RateLimit-Remaining-Requests", "5")
		h.Set("X-RateLimit-Reset-Requests", "10")
		h.Set("X-RateLimit-Remaining-Requests-1h", "100")
		h.Set("X-RateLimit-Reset-Requests-1h", "3600")
		h.Set("X-RateLimit-Remaining-Tokens", "1000")
		h.Set("X-RateLimit-Reset-Tokens", "30")
		got := Classify429(h)
		if got != RateLimitUpstreamCapacity {
			t.Fatalf("expected RateLimitUpstreamCapacity with partial headers and no exhaustion, got %q", got)
		}
	})
}
