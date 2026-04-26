package builderloop

import "time"

type Verdict string

const (
	VerdictHealthy Verdict = "healthy"
	VerdictSlow    Verdict = "slow"
	VerdictDead    Verdict = "dead"
)

type WorkerVitals struct {
	PID          int
	LastCommitAt time.Time
	PIDIsLive    bool
}

func Diagnose(now time.Time, v WorkerVitals, deadAfter, slowAfter time.Duration) Verdict {
	if v.PID == 0 {
		return VerdictDead
	}
	elapsed := now.Sub(v.LastCommitAt)
	if !v.PIDIsLive && elapsed >= deadAfter {
		return VerdictDead
	}
	if v.PIDIsLive && elapsed >= slowAfter {
		return VerdictSlow
	}
	return VerdictHealthy
}
