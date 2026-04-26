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

func Classify429(headers http.Header) RateLimitClass {
	bucketTags := []string{"requests", "requests-1h", "tokens", "tokens-1h"}
	parsedAny := false
	exhausted := false

	for _, tag := range bucketTags {
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
