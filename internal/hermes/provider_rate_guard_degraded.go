package hermes

import "time"

type GuardState struct {
	LastKnownClass RateLimitClass
	LastKnownAt    time.Time
	Unavailable    bool
}

func ApplyClassification(state GuardState, now time.Time, class RateLimitClass) GuardState {
	if class == RateLimitInsufficientEvidence || class == RateLimitClass("") {
		return GuardState{
			LastKnownClass: state.LastKnownClass,
			LastKnownAt:    state.LastKnownAt,
			Unavailable:    true,
		}
	}
	return GuardState{
		LastKnownClass: class,
		LastKnownAt:    now,
		Unavailable:    false,
	}
}
