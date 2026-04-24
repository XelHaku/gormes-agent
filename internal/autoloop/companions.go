package autoloop

import "time"

type CompanionState struct {
	LastCycle int
	LastEpoch int64
}

type CompanionOptions struct {
	Name           string
	CurrentCycle   int
	EveryNCycles   int
	EveryDuration  time.Duration
	Now            time.Time
	LoopSleep      time.Duration
	ExternalRecent bool
	Disabled       bool
}

type CompanionDecision struct {
	Run    bool
	Reason string
}

func CompanionDue(opts CompanionOptions, state CompanionState) CompanionDecision {
	if opts.Disabled {
		return CompanionDecision{Reason: "disabled"}
	}
	if opts.ExternalRecent {
		return CompanionDecision{Reason: "external scheduler ran recently"}
	}
	if opts.EveryNCycles > 0 && opts.CurrentCycle-state.LastCycle >= opts.EveryNCycles {
		return CompanionDecision{Run: true, Reason: "cycle cadence reached"}
	}
	if opts.EveryDuration > 0 && opts.Now.Sub(time.Unix(state.LastEpoch, 0)) >= opts.EveryDuration {
		return CompanionDecision{Run: true, Reason: "time cadence reached"}
	}
	return CompanionDecision{Reason: "not due"}
}
