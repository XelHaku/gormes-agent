package gateway

import (
	"strings"
	"time"
)

// DrainReason identifies why a session was left resume-pending during a
// drain. Gormes currently distinguishes between graceful restart drains and
// full process shutdown drains so the next turn's resume note can explain
// what happened.
type DrainReason string

const (
	// DrainReasonRestart indicates a graceful /restart drain timed out while
	// an agent turn was still running.
	DrainReasonRestart DrainReason = "restart"
	// DrainReasonShutdown indicates a process-shutdown drain timed out while
	// an agent turn was still running.
	DrainReasonShutdown DrainReason = "shutdown"
)

// AgentRunState captures the agent's lifecycle status at the moment the
// drain deadline fires. Only agents that were still running at that moment
// get a resume_pending flag; hard states (suspended, stuck-loop) override
// the flag so the operator's pause or the system's loop-break decision is
// never silently rehydrated on the next turn.
type AgentRunState string

const (
	// AgentStateRunning means the agent turn was actively executing when
	// the drain deadline fired.
	AgentStateRunning AgentRunState = "running"
	// AgentStateIdle means the session existed but had no in-flight turn.
	// These sessions do not need a resume note.
	AgentStateIdle AgentRunState = "idle"
	// AgentStateSuspended means the session was intentionally paused.
	// Hard-suspended sessions must not auto-resume on the next turn.
	AgentStateSuspended AgentRunState = "suspended"
	// AgentStateStuckLoop means the watchdog tripped a loop break for the
	// session. These sessions must never auto-resume.
	AgentStateStuckLoop AgentRunState = "stuck_loop"
)

// ResumePending is the in-process read-model flag written when a drain
// deadline fires with an agent still running. It preserves the session_id
// so the next turn can rehydrate the upstream Python session, and carries
// the reason + agent state so the renderer can emit a reason-aware resume
// note.
type ResumePending struct {
	SessionID  string
	Reason     DrainReason
	AgentState AgentRunState
	MarkedAt   time.Time
}

// MarkResumePending is the writer seam for drain-timeout resume_pending
// recovery. It returns (flag, true) only when the agent was still running
// at the drain deadline and we have a non-empty session_id to preserve.
// Hard-suspended and stuck-loop states override the mark — those sessions
// must not auto-resume — and idle sessions do not need a mark at all.
func MarkResumePending(sessionID string, reason DrainReason, state AgentRunState, at time.Time) (ResumePending, bool) {
	flag := ResumePending{
		SessionID:  strings.TrimSpace(sessionID),
		Reason:     reason,
		AgentState: state,
		MarkedAt:   at,
	}
	if !flag.ShouldResume() {
		return ResumePending{}, false
	}
	return flag, true
}

// ShouldResume is the single predicate that decides whether a flag may
// drive a resume on the next turn. It holds exactly when the session_id is
// preserved and the agent was still running at the drain deadline; hard
// suspended/stuck-loop states (and any future non-running state) short
// circuit here. Both the writer (MarkResumePending) and the renderer
// (BuildResumeNote) consult this predicate so a corrupted or manually
// constructed flag can never inject a note the writer would have refused.
func (p ResumePending) ShouldResume() bool {
	if strings.TrimSpace(p.SessionID) == "" {
		return false
	}
	return p.AgentState == AgentStateRunning
}

// BuildResumeNote renders a deterministic, reason-aware system note for
// the turn immediately following a drain timeout. The note mentions the
// preserved session_id so downstream callers know which Python transcript
// the resume targets. Returns the empty string for any flag that must not
// inject a note (empty session_id, non-running agent state, unknown
// reason handled as generic).
func BuildResumeNote(p ResumePending) string {
	if !p.ShouldResume() {
		return ""
	}
	var reasonText string
	switch p.Reason {
	case DrainReasonRestart:
		reasonText = "a graceful restart drain timeout"
	case DrainReasonShutdown:
		reasonText = "a shutdown drain timeout"
	default:
		reasonText = "a drain timeout"
	}
	lines := []string{
		"## Resume Pending",
		"",
		"The previous turn was interrupted by " + reasonText + "; the prior session (`" + p.SessionID + "`) is preserved. Continue where the turn left off.",
	}
	return strings.Join(lines, "\n")
}
