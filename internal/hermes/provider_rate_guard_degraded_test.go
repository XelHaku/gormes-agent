package hermes

import (
	"testing"
	"time"
)

func TestApplyClassification(t *testing.T) {
	t0 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Minute)

	t.Run("preserves_last_known_when_insufficient", func(t *testing.T) {
		got := ApplyClassification(
			GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0, Unavailable: false},
			t1,
			RateLimitInsufficientEvidence,
		)
		want := GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0, Unavailable: true}
		if got.LastKnownClass != want.LastKnownClass {
			t.Fatalf("LastKnownClass: got %q, want %q", got.LastKnownClass, want.LastKnownClass)
		}
		if !got.LastKnownAt.Equal(want.LastKnownAt) {
			t.Fatalf("LastKnownAt: got %v, want %v", got.LastKnownAt, want.LastKnownAt)
		}
		if got.Unavailable != want.Unavailable {
			t.Fatalf("Unavailable: got %v, want %v", got.Unavailable, want.Unavailable)
		}
	})

	t.Run("treats_empty_class_as_insufficient", func(t *testing.T) {
		got := ApplyClassification(
			GuardState{LastKnownClass: RateLimitUpstreamCapacity, LastKnownAt: t0, Unavailable: false},
			t1,
			RateLimitClass(""),
		)
		want := GuardState{LastKnownClass: RateLimitUpstreamCapacity, LastKnownAt: t0, Unavailable: true}
		if got.LastKnownClass != want.LastKnownClass {
			t.Fatalf("LastKnownClass: got %q, want %q", got.LastKnownClass, want.LastKnownClass)
		}
		if !got.LastKnownAt.Equal(want.LastKnownAt) {
			t.Fatalf("LastKnownAt: got %v, want %v", got.LastKnownAt, want.LastKnownAt)
		}
		if got.Unavailable != want.Unavailable {
			t.Fatalf("Unavailable: got %v, want %v", got.Unavailable, want.Unavailable)
		}
	})

	t.Run("fresh_genuine_quota_clears_unavailable", func(t *testing.T) {
		got := ApplyClassification(GuardState{}, t1, RateLimitGenuineQuota)
		want := GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t1, Unavailable: false}
		if got.LastKnownClass != want.LastKnownClass {
			t.Fatalf("LastKnownClass: got %q, want %q", got.LastKnownClass, want.LastKnownClass)
		}
		if !got.LastKnownAt.Equal(want.LastKnownAt) {
			t.Fatalf("LastKnownAt: got %v, want %v", got.LastKnownAt, want.LastKnownAt)
		}
		if got.Unavailable != want.Unavailable {
			t.Fatalf("Unavailable: got %v, want %v", got.Unavailable, want.Unavailable)
		}
	})

	t.Run("fresh_upstream_capacity_clears_unavailable", func(t *testing.T) {
		got := ApplyClassification(GuardState{}, t1, RateLimitUpstreamCapacity)
		want := GuardState{LastKnownClass: RateLimitUpstreamCapacity, LastKnownAt: t1, Unavailable: false}
		if got.LastKnownClass != want.LastKnownClass {
			t.Fatalf("LastKnownClass: got %q, want %q", got.LastKnownClass, want.LastKnownClass)
		}
		if !got.LastKnownAt.Equal(want.LastKnownAt) {
			t.Fatalf("LastKnownAt: got %v, want %v", got.LastKnownAt, want.LastKnownAt)
		}
		if got.Unavailable != want.Unavailable {
			t.Fatalf("Unavailable: got %v, want %v", got.Unavailable, want.Unavailable)
		}
	})

	t.Run("input_immutability_via_struct_value", func(t *testing.T) {
		original := GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0, Unavailable: true}
		_ = ApplyClassification(original, t1, RateLimitUpstreamCapacity)
		if original.LastKnownClass != RateLimitGenuineQuota {
			t.Fatalf("original.LastKnownClass mutated: got %q, want %q", original.LastKnownClass, RateLimitGenuineQuota)
		}
		if !original.LastKnownAt.Equal(t0) {
			t.Fatalf("original.LastKnownAt mutated: got %v, want %v", original.LastKnownAt, t0)
		}
		if original.Unavailable != true {
			t.Fatalf("original.Unavailable mutated: got %v, want %v", original.Unavailable, true)
		}
	})

	t.Run("transitions_back_to_available_on_fresh_evidence", func(t *testing.T) {
		got := ApplyClassification(
			GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0, Unavailable: true},
			t1,
			RateLimitUpstreamCapacity,
		)
		want := GuardState{LastKnownClass: RateLimitUpstreamCapacity, LastKnownAt: t1, Unavailable: false}
		if got.LastKnownClass != want.LastKnownClass {
			t.Fatalf("LastKnownClass: got %q, want %q", got.LastKnownClass, want.LastKnownClass)
		}
		if !got.LastKnownAt.Equal(want.LastKnownAt) {
			t.Fatalf("LastKnownAt: got %v, want %v", got.LastKnownAt, want.LastKnownAt)
		}
		if got.Unavailable != want.Unavailable {
			t.Fatalf("Unavailable: got %v, want %v", got.Unavailable, want.Unavailable)
		}
	})
}
