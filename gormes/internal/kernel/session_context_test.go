package kernel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

func TestKernel_InjectsPerEventSessionContextBeforeRecallAndUser(t *testing.T) {
	rec := &mockRecall{returnContent: "<memory-context>MEMORY BLOCK</memory-context>"}
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-session-context")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Recall:    rec,
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	_ = k.Submit(PlatformEvent{
		Kind:           PlatformEventSubmit,
		Text:           "hello",
		SessionContext: "## Current Session Context\n**Source:** telegram chat `42`",
	})

	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID != ""
	}, 2*time.Second)

	reqs := mc.Requests()
	if len(reqs) == 0 {
		t.Fatal("mock client received zero requests")
	}
	req := reqs[0]
	if len(req.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3 (session context + recall + user)", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || !strings.Contains(req.Messages[0].Content, "Current Session Context") {
		t.Fatalf("Messages[0] = %+v, want session-context system message", req.Messages[0])
	}
	if req.Messages[1].Role != "system" || !strings.Contains(req.Messages[1].Content, "MEMORY BLOCK") {
		t.Fatalf("Messages[1] = %+v, want recall system message", req.Messages[1])
	}
	if req.Messages[2].Role != "user" || req.Messages[2].Content != "hello" {
		t.Fatalf("Messages[2] = %+v, want user/hello", req.Messages[2])
	}
}
