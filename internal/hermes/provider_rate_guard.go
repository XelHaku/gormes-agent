package hermes

import (
	"net/http"
	"strconv"
	"strings"
)

type RateLimitClass string

const (
	RateLimitGenuineQuota         RateLimitClass = "genuine_quota"
	RateLimitUpstreamCapacity     RateLimitClass = "upstream_capacity"
	RateLimitInsufficientEvidence RateLimitClass = "insufficient_evidence"
)

const minResetForBreakerSeconds = 60.0

// Status evidence tags surfaced via DecideRateGuard. They are the only
// strings the kernel and provider status callers should emit when reporting
// rate-guard state, so they remain stable and grep-able.
const (
	StatusRateGuardUnavailable = "rate_guard_unavailable"
	StatusNousRateLimited      = "nous_rate_limited"
	StatusNousUpstreamCapacity = "nous_upstream_capacity"
	StatusBudgetHeaderMissing  = "budget_header_missing"
)

var rateLimitBucketTags = []string{"requests", "requests-1h", "tokens", "tokens-1h"}

func Classify429(headers http.Header) RateLimitClass {
	parsedAny := false
	exhausted := false

	for _, tag := range rateLimitBucketTags {
		remainingRaw := strings.TrimSpace(headers.Get("X-RateLimit-Remaining-" + tag))
		resetRaw := strings.TrimSpace(headers.Get("X-RateLimit-Reset-" + tag))
		if remainingRaw == "" || resetRaw == "" {
			continue
		}
		remaining, err := strconv.Atoi(remainingRaw)
		if err != nil {
			continue
		}
		reset, err := strconv.ParseFloat(resetRaw, 64)
		if err != nil {
			continue
		}
		parsedAny = true
		if remaining <= 0 && reset >= minResetForBreakerSeconds {
			exhausted = true
		}
	}

	switch {
	case exhausted:
		return RateLimitGenuineQuota
	case parsedAny:
		return RateLimitUpstreamCapacity
	default:
		return RateLimitInsufficientEvidence
	}
}

// BudgetBucket is a single x-ratelimit-* bucket extracted from a response
// header set. Missing or malformed values leave HasRemaining/HasReset false
// instead of coercing to zero, so callers can degrade visibly rather than
// silently treating partial data as exhaustion.
type BudgetBucket struct {
	Tag          string
	Remaining    int
	HasRemaining bool
	Reset        float64
	HasReset     bool
}

// Exhausted reports whether the bucket meets the Hermes 192e7eb2 genuine-
// quota rule: remaining<=0 with a reset window >= the 60s threshold.
func (b BudgetBucket) Exhausted() bool {
	return b.HasRemaining && b.HasReset && b.Remaining <= 0 && b.Reset >= minResetForBreakerSeconds
}

// BudgetSnapshot is the parsed budget telemetry for the four Hermes Nous
// bucket tags. AnyParsed is true when at least one bucket has both remaining
// and reset parsed; HeaderMissing is its negation and covers both fully
// missing and entirely malformed responses.
type BudgetSnapshot struct {
	Buckets       []BudgetBucket
	Exhausted     []string
	AnyParsed     bool
	HeaderMissing bool
}

// ParseBudget extracts the four Hermes Nous bucket tags from a response
// header set. The function is pure: it never sleeps, never reads the wall
// clock, and never touches process-global state.
func ParseBudget(headers http.Header) BudgetSnapshot {
	snap := BudgetSnapshot{Buckets: make([]BudgetBucket, 0, len(rateLimitBucketTags))}
	for _, tag := range rateLimitBucketTags {
		remainingRaw := strings.TrimSpace(headers.Get("X-RateLimit-Remaining-" + tag))
		resetRaw := strings.TrimSpace(headers.Get("X-RateLimit-Reset-" + tag))
		if remainingRaw == "" && resetRaw == "" {
			continue
		}
		bucket := BudgetBucket{Tag: tag}
		if v, err := strconv.Atoi(remainingRaw); err == nil {
			bucket.Remaining = v
			bucket.HasRemaining = true
		}
		if v, err := strconv.ParseFloat(resetRaw, 64); err == nil {
			bucket.Reset = v
			bucket.HasReset = true
		}
		snap.Buckets = append(snap.Buckets, bucket)
		if bucket.HasRemaining && bucket.HasReset {
			snap.AnyParsed = true
			if bucket.Exhausted() {
				snap.Exhausted = append(snap.Exhausted, bucket.Tag)
			}
		}
	}
	snap.HeaderMissing = !snap.AnyParsed
	return snap
}

// RateGuardDecision is the visible per-429 outcome that callers attach to
// provider status. It captures the classification, the source of evidence,
// and a redacted reset-window estimate so status frames can show what
// happened without leaking session data or live timestamps.
//
// The decision is intentionally inert: it never blocks, never sleeps, and
// never mutates retry/routing/fallback policy. Slept is exposed so unit
// tests can prove the no-sleep contract and so future status frames can
// surface the invariant alongside other capability evidence.
type RateGuardDecision struct {
	Class            RateLimitClass
	Source           string
	EvidenceTags     []string
	ResetSeconds     int
	ResetBucket      string
	LastKnownPresent bool
	Slept            bool
	Budget           BudgetSnapshot
}

// DecideRateGuard combines a 429's response headers with an optional
// last-known classification snapshot to produce a status-only rate-guard
// decision. The function is pure and never sleeps; it must not be used to
// mutate cross-session breaker state or to block other provider/model
// choices.
//
// Algorithm (matches Hermes 192e7eb2 nous_rate_guard.is_genuine_nous_rate_limit):
//   - If the headers themselves classify as genuine_quota or upstream_capacity,
//     trust them as the freshest evidence.
//   - If the headers are insufficient/malformed, fall back to the last known
//     classification: a prior genuine_quota implies the 429 is the same limit
//     continuing, while a prior upstream_capacity keeps the 429 transient.
//   - If neither signal is present, mark the decision as insufficient_evidence
//     and surface rate_guard_unavailable + budget_header_missing so the
//     provider status can degrade visibly.
func DecideRateGuard(headers http.Header, last GuardState) RateGuardDecision {
	decision := RateGuardDecision{
		Budget:           ParseBudget(headers),
		LastKnownPresent: last.LastKnownClass != "",
	}

	headerClass := Classify429(headers)
	switch headerClass {
	case RateLimitGenuineQuota, RateLimitUpstreamCapacity:
		decision.Class = headerClass
		decision.Source = "headers"
	default:
		switch last.LastKnownClass {
		case RateLimitGenuineQuota, RateLimitUpstreamCapacity:
			decision.Class = last.LastKnownClass
			decision.Source = "last_known"
		default:
			decision.Class = RateLimitInsufficientEvidence
			decision.Source = "missing"
		}
	}

	switch decision.Class {
	case RateLimitGenuineQuota:
		decision.EvidenceTags = append(decision.EvidenceTags, StatusNousRateLimited)
	case RateLimitUpstreamCapacity:
		decision.EvidenceTags = append(decision.EvidenceTags, StatusNousUpstreamCapacity)
	default:
		decision.EvidenceTags = append(decision.EvidenceTags, StatusRateGuardUnavailable)
	}
	if decision.Budget.HeaderMissing {
		decision.EvidenceTags = append(decision.EvidenceTags, StatusBudgetHeaderMissing)
	}

	if decision.Class == RateLimitGenuineQuota && decision.Source == "headers" {
		var (
			bestReset float64
			bestTag   string
		)
		for _, b := range decision.Budget.Buckets {
			if !b.Exhausted() {
				continue
			}
			if b.Reset >= bestReset {
				bestReset = b.Reset
				bestTag = b.Tag
			}
		}
		decision.ResetSeconds = int(bestReset)
		decision.ResetBucket = bestTag
	}

	return decision
}
