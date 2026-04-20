package kernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

// TestKernel_FinalizeAssistantTurnReachesStore proves the kernel fires
// both AppendUserTurn (pre-stream) and FinalizeAssistantTurn (post-stream)
// on every successful turn, with matching session_id and content.
func TestKernel_FinalizeAssistantTurnReachesStore(t *testing.T) {
	rec := store.NewRecording()

	mc := hermes.NewMockClient()
	reply := "hello back"
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"})
	mc.Script(events, "sess-finalize-test")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, rec, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID != ""
	}, 3*time.Second)

	cmds := rec.Commands()
	if len(cmds) < 2 {
		t.Fatalf("len(cmds) = %d, want >= 2 (AppendUserTurn + FinalizeAssistantTurn)", len(cmds))
	}

	// First command must be AppendUserTurn with the user's text.
	if cmds[0].Kind != store.AppendUserTurn {
		t.Errorf("cmds[0].Kind = %v, want AppendUserTurn", cmds[0].Kind)
	}
	var p1 struct {
		SessionID string `json:"session_id"`
		Content   string `json:"content"`
		TsUnix    int64  `json:"ts_unix"`
	}
	if err := json.Unmarshal(cmds[0].Payload, &p1); err != nil {
		t.Fatalf("cmds[0] payload: %v", err)
	}
	if p1.Content != "hi" {
		t.Errorf("AppendUserTurn content = %q, want %q", p1.Content, "hi")
	}
	if p1.TsUnix == 0 {
		t.Errorf("AppendUserTurn ts_unix is zero")
	}

	// A later command must be FinalizeAssistantTurn with the assistant's reply.
	var foundFinalize bool
	for _, c := range cmds[1:] {
		if c.Kind != store.FinalizeAssistantTurn {
			continue
		}
		var p2 struct {
			SessionID string `json:"session_id"`
			Content   string `json:"content"`
			TsUnix    int64  `json:"ts_unix"`
		}
		if err := json.Unmarshal(c.Payload, &p2); err != nil {
			t.Fatalf("FinalizeAssistantTurn payload: %v", err)
		}
		if !strings.Contains(p2.Content, "hello back") {
			t.Errorf("FinalizeAssistantTurn content = %q, want contains 'hello back'", p2.Content)
		}
		if p2.SessionID != "sess-finalize-test" {
			t.Errorf("FinalizeAssistantTurn session_id = %q", p2.SessionID)
		}
		foundFinalize = true
		break
	}
	if !foundFinalize {
		t.Errorf("no FinalizeAssistantTurn command captured; got kinds = %v", kindStrings(cmds))
	}
}

func kindStrings(cmds []store.Command) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Kind.String()
	}
	return out
}

// waitForNCommands polls the recording store until it has at least n commands,
// or the deadline passes. Test helper — safe to call from any goroutine.
func waitForNCommands(t *testing.T, st *store.RecordingStore, n int, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if len(st.Commands()) >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("did not see %d commands within %s; got %d", n, d, len(st.Commands()))
}

// scriptOneTurn sets up the mock client to return a single-event stream
// (EventDone only) with the given sessionID returned by the server. This
// keeps the test turn minimal — no tokens emitted, just a clean finish.
func scriptOneTurn(mc *hermes.MockClient, serverSessionID string) {
	mc.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, serverSessionID)
}

// TestKernel_SessionIDOverrideAppliesToTurn verifies that when an event
// carries a non-empty SessionID, the kernel uses it for that turn's
// store payload INSTEAD of k.sessionID. Also verifies CronJobID flows
// through to the cron_job_id field with cron=1 marker.
func TestKernel_SessionIDOverrideAppliesToTurn(t *testing.T) {
	rec := store.NewRecording()
	mc := hermes.NewMockClient()
	scriptOneTurn(mc, "")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, rec, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render() // consume initial idle frame

	if err := k.Submit(PlatformEvent{
		Kind:      PlatformEventSubmit,
		Text:      "hello",
		SessionID: "cron:job-7:1700000000",
		CronJobID: "job-7",
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Wait for AppendUserTurn (at minimum) to land in the recording.
	waitForNCommands(t, rec, 1, 2*time.Second)

	cmds := rec.Commands()
	var found bool
	for _, c := range cmds {
		if c.Kind != store.AppendUserTurn {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			t.Fatalf("unmarshal AppendUserTurn payload: %v", err)
		}
		if p["session_id"] != "cron:job-7:1700000000" {
			t.Errorf("session_id = %v, want cron:job-7:1700000000", p["session_id"])
		}
		if p["cron_job_id"] != "job-7" {
			t.Errorf("cron_job_id = %v, want job-7", p["cron_job_id"])
		}
		if v, _ := p["cron"].(float64); int(v) != 1 {
			t.Errorf("cron = %v, want 1", p["cron"])
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("AppendUserTurn not captured; got kinds = %v", kindStrings(cmds))
	}
}

// TestKernel_SessionIDOverrideDoesNotLeakToNextTurn verifies that the
// override is a per-event swap, not a permanent mutation of k.sessionID.
// After a cron turn runs, subsequent non-cron events use the kernel's
// resident sessionID, NOT the cron override.
func TestKernel_SessionIDOverrideDoesNotLeakToNextTurn(t *testing.T) {
	rec := store.NewRecording()
	mc := hermes.NewMockClient()
	// Turn 1: cron. Turn 2: normal. Both scripted ahead of time.
	scriptOneTurn(mc, "") // server doesn't assign a new session for turn 1
	scriptOneTurn(mc, "") // server doesn't assign a new session for turn 2

	k := New(Config{
		Model:            "hermes-agent",
		Endpoint:         "http://mock",
		Admission:        Admission{MaxBytes: 200_000, MaxLines: 10_000},
		InitialSessionID: "resident-session-xyz",
	}, mc, rec, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render() // consume initial idle frame

	// Turn 1: cron override.
	if err := k.Submit(PlatformEvent{
		Kind:      PlatformEventSubmit,
		Text:      "cron hi",
		SessionID: "cron:job-1:1",
		CronJobID: "job-1",
	}); err != nil {
		t.Fatalf("Submit turn 1: %v", err)
	}

	// Wait for turn 1's AppendUserTurn to land, then wait for the kernel to
	// return to Idle before sending turn 2.
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle
	}, 2*time.Second)

	// Turn 2: no override.
	if err := k.Submit(PlatformEvent{
		Kind: PlatformEventSubmit,
		Text: "normal hi",
	}); err != nil {
		t.Fatalf("Submit turn 2: %v", err)
	}

	// Wait for turn 2's AppendUserTurn to appear (at least 2 AppendUserTurns total).
	waitForNCommands(t, rec, 2, 2*time.Second)

	// Find the second AppendUserTurn (text == "normal hi").
	cmds := rec.Commands()
	var secondPayload map[string]any
	for _, c := range cmds {
		if c.Kind != store.AppendUserTurn {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			continue
		}
		if p["content"] == "normal hi" {
			secondPayload = p
			break
		}
	}
	if secondPayload == nil {
		t.Fatalf("could not find AppendUserTurn for 'normal hi'; got kinds = %v", kindStrings(cmds))
	}

	// session_id must NOT be the cron override.
	if secondPayload["session_id"] == "cron:job-1:1" {
		t.Errorf("cron sessionID leaked into next turn: %v", secondPayload["session_id"])
	}
	// Resident session ID must be restored (server returned "" so k.sessionID stays resident).
	if secondPayload["session_id"] != "resident-session-xyz" {
		t.Errorf("session_id = %v, want resident-session-xyz", secondPayload["session_id"])
	}
	// cron flag must not bleed.
	if v, _ := secondPayload["cron"].(float64); int(v) != 0 {
		t.Errorf("cron=%v leaked into next turn, want 0", secondPayload["cron"])
	}
	if cj, _ := secondPayload["cron_job_id"].(string); cj != "" {
		t.Errorf("cron_job_id=%q leaked into next turn, want empty", cj)
	}
}

// TestKernel_NonCronEventSendsZeroCronFields verifies baseline: no
// SessionID/CronJobID on the event → payload has cron=0 and empty cron_job_id.
func TestKernel_NonCronEventSendsZeroCronFields(t *testing.T) {
	rec := store.NewRecording()
	mc := hermes.NewMockClient()
	scriptOneTurn(mc, "")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, rec, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitForNCommands(t, rec, 1, 2*time.Second)

	cmds := rec.Commands()
	if len(cmds) == 0 {
		t.Fatal("no commands captured")
	}
	var p map[string]any
	if err := json.Unmarshal(cmds[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, _ := p["cron"].(float64); int(v) != 0 {
		t.Errorf("cron = %v, want 0 for non-cron event", p["cron"])
	}
	if cj, _ := p["cron_job_id"].(string); cj != "" {
		t.Errorf("cron_job_id = %q, want empty for non-cron event", cj)
	}
}
