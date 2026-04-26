package gateway

import (
	"errors"
	"log/slog"
	"testing"
)

const (
	reasoningSourceUnset   = "unset"
	reasoningSourceSession = "session"
	reasoningSourceGlobal  = "global"
)

func TestApplyReasoningCommand_ShowReturnsCurrentScope(t *testing.T) {
	state := SessionReasoningState{Effort: ReasoningEffortHigh, Source: reasoningSourceGlobal}
	calls := 0
	persist := func(ReasoningEffort) error { calls++; return nil }

	newState, reply := ApplyReasoningCommand(
		state,
		ReasoningCommand{Action: ReasoningActionShow},
		persist,
	)

	if newState != state {
		t.Fatalf("Show mutated state: got %+v, want %+v", newState, state)
	}
	if calls != 0 {
		t.Fatalf("Show called persistGlobal %d times, want 0", calls)
	}
	if reply.Effort != ReasoningEffortHigh {
		t.Fatalf("reply.Effort = %q, want %q", reply.Effort, ReasoningEffortHigh)
	}
	if reply.Scope != reasoningSourceGlobal {
		t.Fatalf("reply.Scope = %q, want %q", reply.Scope, reasoningSourceGlobal)
	}
	if reply.PersistFailed {
		t.Fatalf("reply.PersistFailed = true, want false")
	}
}

func TestApplyReasoningCommand_SetSessionMutatesOnly(t *testing.T) {
	state := SessionReasoningState{Source: reasoningSourceUnset}
	calls := 0
	persist := func(ReasoningEffort) error { calls++; return nil }

	newState, reply := ApplyReasoningCommand(
		state,
		ReasoningCommand{Action: ReasoningActionSet, Effort: ReasoningEffortLow, Global: false},
		persist,
	)

	if calls != 0 {
		t.Fatalf("session set called persistGlobal %d times, want 0", calls)
	}
	wantState := SessionReasoningState{Effort: ReasoningEffortLow, Source: reasoningSourceSession}
	if newState != wantState {
		t.Fatalf("newState = %+v, want %+v", newState, wantState)
	}
	if reply.Effort != ReasoningEffortLow || reply.Scope != reasoningSourceSession {
		t.Fatalf("reply = %+v, want effort=%q scope=%q", reply, ReasoningEffortLow, reasoningSourceSession)
	}
	if reply.PersistFailed {
		t.Fatalf("reply.PersistFailed = true, want false")
	}
}

func TestApplyReasoningCommand_SetGlobalCallsPersistGlobal(t *testing.T) {
	state := SessionReasoningState{Effort: ReasoningEffortMedium, Source: reasoningSourceSession}
	var saved ReasoningEffort
	calls := 0
	persist := func(e ReasoningEffort) error {
		calls++
		saved = e
		return nil
	}

	newState, reply := ApplyReasoningCommand(
		state,
		ReasoningCommand{Action: ReasoningActionSet, Effort: ReasoningEffortHigh, Global: true},
		persist,
	)

	if calls != 1 {
		t.Fatalf("persistGlobal called %d times, want 1", calls)
	}
	if saved != ReasoningEffortHigh {
		t.Fatalf("persistGlobal effort = %q, want %q", saved, ReasoningEffortHigh)
	}
	wantState := SessionReasoningState{Effort: ReasoningEffortHigh, Source: reasoningSourceGlobal}
	if newState != wantState {
		t.Fatalf("newState = %+v, want %+v", newState, wantState)
	}
	if reply.Effort != ReasoningEffortHigh || reply.Scope != reasoningSourceGlobal {
		t.Fatalf("reply = %+v, want effort=%q scope=%q", reply, ReasoningEffortHigh, reasoningSourceGlobal)
	}
	if reply.PersistFailed {
		t.Fatalf("reply.PersistFailed = true, want false")
	}
}

func TestApplyReasoningCommand_GlobalPersistFallback(t *testing.T) {
	state := SessionReasoningState{Source: reasoningSourceUnset}
	persistErr := errors.New("disk full")
	calls := 0
	persist := func(ReasoningEffort) error {
		calls++
		return persistErr
	}

	newState, reply := ApplyReasoningCommand(
		state,
		ReasoningCommand{Action: ReasoningActionSet, Effort: ReasoningEffortLow, Global: true},
		persist,
	)

	if calls != 1 {
		t.Fatalf("persistGlobal called %d times, want 1", calls)
	}
	wantState := SessionReasoningState{Effort: ReasoningEffortLow, Source: reasoningSourceSession}
	if newState != wantState {
		t.Fatalf("newState = %+v, want %+v", newState, wantState)
	}
	if !reply.PersistFailed {
		t.Fatalf("reply.PersistFailed = false, want true")
	}
	if reply.Effort != ReasoningEffortLow || reply.Scope != reasoningSourceSession {
		t.Fatalf("reply = %+v, want effort=%q scope=%q", reply, ReasoningEffortLow, reasoningSourceSession)
	}
}

func TestApplyReasoningCommand_ResetClearsSessionState(t *testing.T) {
	state := SessionReasoningState{Effort: ReasoningEffortHigh, Source: reasoningSourceSession}
	calls := 0
	persist := func(ReasoningEffort) error { calls++; return nil }

	newState, reply := ApplyReasoningCommand(
		state,
		ReasoningCommand{Action: ReasoningActionReset},
		persist,
	)

	if calls != 0 {
		t.Fatalf("reset called persistGlobal %d times, want 0", calls)
	}
	wantState := SessionReasoningState{Effort: ReasoningEffort(""), Source: reasoningSourceUnset}
	if newState != wantState {
		t.Fatalf("newState = %+v, want %+v", newState, wantState)
	}
	if reply.Effort != ReasoningEffort("") || reply.Scope != reasoningSourceUnset {
		t.Fatalf("reply = %+v, want effort=\"\" scope=%q", reply, reasoningSourceUnset)
	}
	if reply.PersistFailed {
		t.Fatalf("reply.PersistFailed = true, want false")
	}
}

func TestParseReasoningCommand_ShowFormReturnsActionShow(t *testing.T) {
	cmd, err := ParseReasoningCommand(nil)
	if err != nil {
		t.Fatalf("ParseReasoningCommand(nil) err = %v, want nil", err)
	}
	if cmd.Action != ReasoningActionShow {
		t.Fatalf("Action = %v, want ReasoningActionShow", cmd.Action)
	}
	if cmd.Effort != ReasoningEffort("") {
		t.Fatalf("Effort = %q, want empty", cmd.Effort)
	}
	if cmd.Global {
		t.Fatalf("Global = true, want false")
	}

	cmd, err = ParseReasoningCommand([]string{})
	if err != nil {
		t.Fatalf("ParseReasoningCommand([]) err = %v, want nil", err)
	}
	if cmd.Action != ReasoningActionShow {
		t.Fatalf("Action = %v, want ReasoningActionShow", cmd.Action)
	}
}

func TestParseReasoningCommand_SetSessionScoped(t *testing.T) {
	for _, effort := range []string{"high", "low", "medium"} {
		t.Run(effort, func(t *testing.T) {
			cmd, err := ParseReasoningCommand([]string{effort})
			if err != nil {
				t.Fatalf("ParseReasoningCommand([%q]) err = %v, want nil", effort, err)
			}
			if cmd.Action != ReasoningActionSet {
				t.Fatalf("Action = %v, want ReasoningActionSet", cmd.Action)
			}
			if cmd.Effort != ReasoningEffort(effort) {
				t.Fatalf("Effort = %q, want %q", cmd.Effort, effort)
			}
			if cmd.Global {
				t.Fatalf("Global = true, want false")
			}
		})
	}
}

func TestParseReasoningCommand_SetGlobal(t *testing.T) {
	cmd, err := ParseReasoningCommand([]string{"low", "--global"})
	if err != nil {
		t.Fatalf("ParseReasoningCommand([low --global]) err = %v, want nil", err)
	}
	if cmd.Action != ReasoningActionSet {
		t.Fatalf("Action = %v, want ReasoningActionSet", cmd.Action)
	}
	if cmd.Effort != ReasoningEffort("low") {
		t.Fatalf("Effort = %q, want low", cmd.Effort)
	}
	if !cmd.Global {
		t.Fatalf("Global = false, want true")
	}
}

func TestParseReasoningCommand_ResetSession(t *testing.T) {
	cmd, err := ParseReasoningCommand([]string{"reset"})
	if err != nil {
		t.Fatalf("ParseReasoningCommand([reset]) err = %v, want nil", err)
	}
	if cmd.Action != ReasoningActionReset {
		t.Fatalf("Action = %v, want ReasoningActionReset", cmd.Action)
	}
	if cmd.Global {
		t.Fatalf("Global = true, want false")
	}
	if cmd.Effort != ReasoningEffort("") {
		t.Fatalf("Effort = %q, want empty", cmd.Effort)
	}
}

func TestParseReasoningCommand_RejectGlobalReset(t *testing.T) {
	_, err := ParseReasoningCommand([]string{"reset", "--global"})
	if err == nil {
		t.Fatalf("ParseReasoningCommand([reset --global]) err = nil, want ErrResetGlobalUnsupported")
	}
	if !errors.Is(err, ErrResetGlobalUnsupported) {
		t.Fatalf("err = %v, want ErrResetGlobalUnsupported", err)
	}
}

func TestParseReasoningCommand_RejectInvalidEffort(t *testing.T) {
	_, err := ParseReasoningCommand([]string{"bogus"})
	if err == nil {
		t.Fatalf("ParseReasoningCommand([bogus]) err = nil, want ErrInvalidEffort")
	}
	if !errors.Is(err, ErrInvalidEffort) {
		t.Fatalf("err = %v, want ErrInvalidEffort", err)
	}
}

func TestGatewayManagerDispatchesReasoning(t *testing.T) {
	var globalCalls []ReasoningEffort
	persist := func(e ReasoningEffort) error {
		globalCalls = append(globalCalls, e)
		return nil
	}

	m := NewManager(ManagerConfig{
		PersistReasoningGlobal: persist,
	}, nil, slog.Default())

	// Set session-only override on chat A.
	replyA, err := m.DispatchReasoning("telegram:42", []string{"high"})
	if err != nil {
		t.Fatalf("DispatchReasoning(A high) err = %v", err)
	}
	if replyA.Effort != ReasoningEffortHigh || replyA.Scope != ReasoningSourceSession {
		t.Fatalf("chat A set reply = %+v, want effort=high scope=session", replyA)
	}
	if len(globalCalls) != 0 {
		t.Fatalf("session-only set persisted globally: calls=%v", globalCalls)
	}

	// Show on chat A reflects the override.
	showA, err := m.DispatchReasoning("telegram:42", nil)
	if err != nil {
		t.Fatalf("DispatchReasoning(A show) err = %v", err)
	}
	if showA.Effort != ReasoningEffortHigh || showA.Scope != ReasoningSourceSession {
		t.Fatalf("chat A show = %+v, want effort=high scope=session", showA)
	}

	// Chat B is isolated — never touched, still unset.
	showB, err := m.DispatchReasoning("telegram:99", nil)
	if err != nil {
		t.Fatalf("DispatchReasoning(B show) err = %v", err)
	}
	if showB.Effort != ReasoningEffort("") || showB.Scope != ReasoningSourceUnset {
		t.Fatalf("chat B show = %+v, want effort=\"\" scope=unset (chat A leak)", showB)
	}

	// Global set on chat B calls the persist callback once with the requested effort.
	replyB, err := m.DispatchReasoning("telegram:99", []string{"low", "--global"})
	if err != nil {
		t.Fatalf("DispatchReasoning(B low --global) err = %v", err)
	}
	if replyB.Effort != ReasoningEffortLow || replyB.Scope != ReasoningSourceGlobal {
		t.Fatalf("chat B global reply = %+v, want effort=low scope=global", replyB)
	}
	if len(globalCalls) != 1 || globalCalls[0] != ReasoningEffortLow {
		t.Fatalf("persistGlobal calls = %v, want [low]", globalCalls)
	}

	// Chat A unaffected by chat B's global set — still session-scoped high.
	showAAfter, err := m.DispatchReasoning("telegram:42", nil)
	if err != nil {
		t.Fatalf("DispatchReasoning(A show after) err = %v", err)
	}
	if showAAfter.Effort != ReasoningEffortHigh || showAAfter.Scope != ReasoningSourceSession {
		t.Fatalf("chat A show after = %+v, want effort=high scope=session", showAAfter)
	}

	// Reset on chat A clears the session override.
	resetA, err := m.DispatchReasoning("telegram:42", []string{"reset"})
	if err != nil {
		t.Fatalf("DispatchReasoning(A reset) err = %v", err)
	}
	if resetA.Effort != ReasoningEffort("") || resetA.Scope != ReasoningSourceUnset {
		t.Fatalf("chat A reset = %+v, want effort=\"\" scope=unset", resetA)
	}

	// Invalid arg returns the parser error verbatim.
	if _, err := m.DispatchReasoning("telegram:42", []string{"bogus"}); !errors.Is(err, ErrInvalidEffort) {
		t.Fatalf("invalid effort err = %v, want ErrInvalidEffort", err)
	}
}

func TestGatewayManagerDispatchReasoningGlobalPersistFallback(t *testing.T) {
	persistErr := errors.New("config write failed")
	persist := func(ReasoningEffort) error { return persistErr }

	m := NewManager(ManagerConfig{
		PersistReasoningGlobal: persist,
	}, nil, slog.Default())

	reply, err := m.DispatchReasoning("telegram:42", []string{"medium", "--global"})
	if err != nil {
		t.Fatalf("DispatchReasoning err = %v, want nil (fallback should not propagate)", err)
	}
	if !reply.PersistFailed {
		t.Fatalf("reply.PersistFailed = false, want true on persistGlobal error")
	}
	if reply.Effort != ReasoningEffortMedium || reply.Scope != ReasoningSourceSession {
		t.Fatalf("fallback reply = %+v, want effort=medium scope=session", reply)
	}

	// Subsequent show reflects session fallback.
	show, err := m.DispatchReasoning("telegram:42", nil)
	if err != nil {
		t.Fatalf("DispatchReasoning(show) err = %v", err)
	}
	if show.Effort != ReasoningEffortMedium || show.Scope != ReasoningSourceSession {
		t.Fatalf("post-fallback show = %+v, want effort=medium scope=session", show)
	}
}
