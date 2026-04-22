package kernel

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

type stubSkillProvider struct {
	block string
	names []string
	err   error
	calls int
	last  string
}

func (s *stubSkillProvider) BuildSkillBlock(_ context.Context, userMessage string) (string, []string, error) {
	s.calls++
	s.last = userMessage
	return s.block, append([]string(nil), s.names...), s.err
}

type stubSkillUsageRecorder struct {
	calls int
	got   [][]string
	err   error
}

func (s *stubSkillUsageRecorder) RecordSkillUsage(_ context.Context, skillNames []string) error {
	s.calls++
	s.got = append(s.got, append([]string(nil), skillNames...))
	return s.err
}

func TestKernel_InjectsSkillBlockAndRecordsUsage(t *testing.T) {
	provider := &stubSkillProvider{
		block: "<skills>\n## careful-review\nReview carefully.\n</skills>",
		names: []string{"careful-review"},
	}
	recorder := &stubSkillUsageRecorder{}
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{{Kind: hermes.EventDone, FinishReason: "stop"}}, "sess-skills")

	k := New(Config{
		Model:      "hermes-agent",
		Endpoint:   "http://mock",
		Admission:  Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Skills:     provider,
		SkillUsage: recorder,
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	_ = k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "please review this patch"})

	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID != ""
	}, 2*time.Second)

	reqs := mc.Requests()
	if len(reqs) == 0 {
		t.Fatal("mock client received zero requests")
	}
	req := reqs[0]
	if len(req.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2 (system + user)", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Fatalf("Messages[0].Role = %q, want system", req.Messages[0].Role)
	}
	if !strings.Contains(req.Messages[0].Content, "careful-review") {
		t.Fatalf("system message = %q, want skill block", req.Messages[0].Content)
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != "please review this patch" {
		t.Fatalf("Messages[1] = %+v, want user submit", req.Messages[1])
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if provider.last != "please review this patch" {
		t.Fatalf("provider last user message = %q", provider.last)
	}
	if recorder.calls != 1 {
		t.Fatalf("recorder calls = %d, want 1", recorder.calls)
	}
	if !reflect.DeepEqual(recorder.got, [][]string{{"careful-review"}}) {
		t.Fatalf("recorder got = %#v, want [[careful-review]]", recorder.got)
	}
}

func TestKernel_SkillProviderErrorFallsBackToUserOnly(t *testing.T) {
	provider := &stubSkillProvider{err: errors.New("boom")}
	recorder := &stubSkillUsageRecorder{}
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{{Kind: hermes.EventDone, FinishReason: "stop"}}, "sess-skills-fallback")

	k := New(Config{
		Model:      "hermes-agent",
		Endpoint:   "http://mock",
		Admission:  Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Skills:     provider,
		SkillUsage: recorder,
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	_ = k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "plain request"})

	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID != ""
	}, 2*time.Second)

	reqs := mc.Requests()
	if len(reqs) == 0 {
		t.Fatal("mock client received zero requests")
	}
	if len(reqs[0].Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1 (user only)", len(reqs[0].Messages))
	}
	if reqs[0].Messages[0].Role != "user" || reqs[0].Messages[0].Content != "plain request" {
		t.Fatalf("Messages[0] = %+v, want user/plain request", reqs[0].Messages[0])
	}
	if recorder.calls != 0 {
		t.Fatalf("recorder calls = %d, want 0", recorder.calls)
	}
}
