package builderloop

import "time"

type Decision string

const (
	DecisionFirst Decision = "first"
	DecisionAmend Decision = "amend"
	DecisionNoop  Decision = "noop"
)

type CheckpointState struct {
	LastCheckpointAt time.Time
	LastSubject      string
	WindowID         string
}

type CoalesceConfig struct {
	WindowSeconds int
	Dirty         bool
	NextWindowID  func() string
}

func DecideCheckpoint(now time.Time, st CheckpointState, cfg CoalesceConfig) (Decision, CheckpointState) {
	if !cfg.Dirty {
		return DecisionNoop, st
	}

	windowSeconds := cfg.WindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = 600
	}
	window := time.Duration(windowSeconds) * time.Second
	if st.WindowID != "" && now.Sub(st.LastCheckpointAt) < window {
		return DecisionAmend, CheckpointState{
			LastCheckpointAt: now,
			LastSubject:      st.LastSubject,
			WindowID:         st.WindowID,
		}
	}

	return DecisionFirst, CheckpointState{
		LastCheckpointAt: now,
		LastSubject:      st.LastSubject,
		WindowID:         cfg.NextWindowID(),
	}
}
