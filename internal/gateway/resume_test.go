package gateway

import (
	"strings"
	"testing"
	"time"
)

func TestMarkResumePending_RunningAgentProducesFlag(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()
	flag, ok := MarkResumePending("sess-123", DrainReasonRestart, AgentStateRunning, at)
	if !ok {
		t.Fatalf("MarkResumePending(running) ok = false, want true")
	}
	if flag.SessionID != "sess-123" {
		t.Fatalf("flag.SessionID = %q, want %q", flag.SessionID, "sess-123")
	}
	if flag.Reason != DrainReasonRestart {
		t.Fatalf("flag.Reason = %q, want %q", flag.Reason, DrainReasonRestart)
	}
	if flag.AgentState != AgentStateRunning {
		t.Fatalf("flag.AgentState = %q, want %q", flag.AgentState, AgentStateRunning)
	}
	if !flag.MarkedAt.Equal(at) {
		t.Fatalf("flag.MarkedAt = %v, want %v", flag.MarkedAt, at)
	}
}

func TestMarkResumePending_SuspendedSkipsMark(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()
	_, ok := MarkResumePending("sess-123", DrainReasonRestart, AgentStateSuspended, at)
	if ok {
		t.Fatalf("MarkResumePending(suspended) ok = true, want false (hard state must override)")
	}
}

func TestMarkResumePending_StuckLoopSkipsMark(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()
	_, ok := MarkResumePending("sess-123", DrainReasonShutdown, AgentStateStuckLoop, at)
	if ok {
		t.Fatalf("MarkResumePending(stuck_loop) ok = true, want false (stuck loop must override)")
	}
}

func TestMarkResumePending_IdleSkipsMark(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()
	_, ok := MarkResumePending("sess-123", DrainReasonShutdown, AgentStateIdle, at)
	if ok {
		t.Fatalf("MarkResumePending(idle) ok = true, want false (only running agents get marked)")
	}
}

func TestMarkResumePending_EmptySessionIDSkips(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()
	_, ok := MarkResumePending("   ", DrainReasonRestart, AgentStateRunning, at)
	if ok {
		t.Fatalf("MarkResumePending(empty session) ok = true, want false (cannot resume without session_id)")
	}
}

func TestBuildResumeNote_RestartReasonMentionsSessionID(t *testing.T) {
	note := BuildResumeNote(ResumePending{
		SessionID:  "sess-123",
		Reason:     DrainReasonRestart,
		AgentState: AgentStateRunning,
	})
	if note == "" {
		t.Fatalf("BuildResumeNote(running/restart) = empty, want non-empty note")
	}
	if !strings.Contains(note, "sess-123") {
		t.Fatalf("BuildResumeNote() = %q, want preserved session_id %q", note, "sess-123")
	}
	if !strings.Contains(note, "restart") {
		t.Fatalf("BuildResumeNote() = %q, want reason %q", note, "restart")
	}
}

func TestBuildResumeNote_ShutdownReasonMentionsShutdown(t *testing.T) {
	note := BuildResumeNote(ResumePending{
		SessionID:  "sess-abc",
		Reason:     DrainReasonShutdown,
		AgentState: AgentStateRunning,
	})
	if !strings.Contains(note, "shutdown") {
		t.Fatalf("BuildResumeNote() = %q, want reason %q", note, "shutdown")
	}
}

func TestBuildResumeNote_SuspendedReturnsEmpty(t *testing.T) {
	if got := BuildResumeNote(ResumePending{
		SessionID:  "sess-123",
		Reason:     DrainReasonRestart,
		AgentState: AgentStateSuspended,
	}); got != "" {
		t.Fatalf("BuildResumeNote(suspended) = %q, want empty string", got)
	}
}

func TestBuildResumeNote_StuckLoopReturnsEmpty(t *testing.T) {
	if got := BuildResumeNote(ResumePending{
		SessionID:  "sess-123",
		Reason:     DrainReasonShutdown,
		AgentState: AgentStateStuckLoop,
	}); got != "" {
		t.Fatalf("BuildResumeNote(stuck_loop) = %q, want empty string", got)
	}
}

func TestBuildResumeNote_EmptySessionIDReturnsEmpty(t *testing.T) {
	if got := BuildResumeNote(ResumePending{
		SessionID:  "",
		Reason:     DrainReasonRestart,
		AgentState: AgentStateRunning,
	}); got != "" {
		t.Fatalf("BuildResumeNote(empty sid) = %q, want empty string", got)
	}
}

func TestBuildSessionContextPrompt_InjectsResumeNoteForRunningAgent(t *testing.T) {
	got := BuildSessionContextPrompt(SessionContext{
		Source: SessionSource{
			Platform: "telegram",
			ChatID:   "42",
			UserID:   "7",
		},
		SessionKey: "telegram:42",
		SessionID:  "sess-123",
		ResumePending: &ResumePending{
			SessionID:  "sess-123",
			Reason:     DrainReasonRestart,
			AgentState: AgentStateRunning,
		},
	})

	for _, want := range []string{
		"## Resume Pending",
		"restart",
		"sess-123",
		"## Current Session Context",
		"**Source:** telegram chat `42`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q in:\n%s", want, got)
		}
	}

	if idx := strings.Index(got, "## Resume Pending"); idx < 0 {
		t.Fatalf("Resume Pending header missing from prompt:\n%s", got)
	} else if sessionIdx := strings.Index(got, "## Current Session Context"); sessionIdx < idx {
		t.Fatalf("Resume Pending should precede Current Session Context in prompt:\n%s", got)
	}
}

func TestBuildSessionContextPrompt_SkipsResumeNoteForSuspendedAgent(t *testing.T) {
	got := BuildSessionContextPrompt(SessionContext{
		Source: SessionSource{
			Platform: "telegram",
			ChatID:   "42",
		},
		SessionKey: "telegram:42",
		SessionID:  "sess-123",
		ResumePending: &ResumePending{
			SessionID:  "sess-123",
			Reason:     DrainReasonRestart,
			AgentState: AgentStateSuspended,
		},
	})

	if strings.Contains(got, "Resume Pending") {
		t.Fatalf("prompt included resume note for suspended agent (hard state must override):\n%s", got)
	}
	if !strings.Contains(got, "## Current Session Context") {
		t.Fatalf("prompt missing baseline session context:\n%s", got)
	}
}

func TestBuildSessionContextPrompt_NilResumePendingUnchanged(t *testing.T) {
	got := BuildSessionContextPrompt(SessionContext{
		Source: SessionSource{
			Platform: "telegram",
			ChatID:   "42",
		},
		SessionKey: "telegram:42",
		SessionID:  "sess-123",
	})

	if strings.Contains(got, "Resume Pending") {
		t.Fatalf("prompt unexpectedly included resume note when ResumePending is nil:\n%s", got)
	}
}
