package kernel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
)

func TestPerTurnReasoningOverrideIsTurnScoped(t *testing.T) {
	const residentModel = "resident-model"
	override := hermes.ReasoningEffortHigh

	mock := hermes.NewMockClient()
	mock.SetProviderStatus(chatCompletionsProviderStatus())
	releaseFirstStream := make(chan struct{})
	client := &gatedMockClient{
		MockClient:         mock,
		releaseFirstStream: releaseFirstStream,
	}
	mock.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "reasoned response"},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-reasoning-override")
	mock.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-provider-default")

	k := New(Config{
		Model:     residentModel,
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)

	initial := <-k.Render()
	if initial.ReasoningEffort.State != hermes.ReasoningEffortStateDefault {
		t.Fatalf("initial ReasoningEffort.State = %q, want %q", initial.ReasoningEffort.State, hermes.ReasoningEffortStateDefault)
	}

	if err := k.Submit(PlatformEvent{
		Kind:            PlatformEventSubmit,
		Text:            "use high reasoning",
		ReasoningEffort: string(override),
	}); err != nil {
		t.Fatal(err)
	}

	inFlight := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return (f.Phase == PhaseConnecting || f.Phase == PhaseStreaming) &&
			f.ReasoningEffort.State == hermes.ReasoningEffortStateOverride
	}, time.Second)
	if inFlight.ReasoningEffort.Effort != override {
		t.Fatalf("in-flight Effort = %q, want %q", inFlight.ReasoningEffort.Effort, override)
	}
	if !inFlight.ReasoningEffort.Forwarded {
		t.Fatalf("in-flight ReasoningEffort.Forwarded = false, want true: %+v", inFlight.ReasoningEffort)
	}

	close(releaseFirstStream)
	firstFinal := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-reasoning-override"
	}, 2*time.Second)
	if firstFinal.ReasoningEffort.State != hermes.ReasoningEffortStateDefault {
		t.Fatalf("first final ReasoningEffort.State = %q, want provider default", firstFinal.ReasoningEffort.State)
	}

	if err := k.Submit(PlatformEvent{
		Kind: PlatformEventSubmit,
		Text: "use provider default",
	}); err != nil {
		t.Fatal(err)
	}
	secondFinal := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-provider-default"
	}, 2*time.Second)
	if secondFinal.ReasoningEffort.State != hermes.ReasoningEffortStateDefault {
		t.Fatalf("second final ReasoningEffort.State = %q, want provider default", secondFinal.ReasoningEffort.State)
	}

	reqs := waitForRequestCount(t, mock, 2, time.Second)
	assertReasoningEffort(t, reqs[0], override)
	if reqs[1].ReasoningEffort != nil {
		t.Fatalf("second ChatRequest.ReasoningEffort = %q, want nil provider default", *reqs[1].ReasoningEffort)
	}
	if k.cfg.ReasoningEffort != "" {
		t.Fatalf("resident config ReasoningEffort mutated to %q, want empty", k.cfg.ReasoningEffort)
	}
}

func TestPerTurnReasoningFallsBackToConfiguredDefault(t *testing.T) {
	const residentModel = "resident-model"
	configDefault := hermes.ReasoningEffortLow
	override := hermes.ReasoningEffortXHigh

	mock := hermes.NewMockClient()
	mock.SetProviderStatus(chatCompletionsProviderStatus())
	mock.Script([]hermes.Event{{Kind: hermes.EventDone, FinishReason: "stop"}}, "sess-xhigh")
	mock.Script([]hermes.Event{{Kind: hermes.EventDone, FinishReason: "stop"}}, "sess-low")

	k := New(Config{
		Model:           residentModel,
		Endpoint:        "http://mock",
		Admission:       Admission{MaxBytes: 200_000, MaxLines: 10_000},
		ReasoningEffort: string(configDefault),
	}, mock, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(PlatformEvent{
		Kind:            PlatformEventSubmit,
		Text:            "use xhigh",
		ReasoningEffort: string(override),
	}); err != nil {
		t.Fatal(err)
	}
	firstFinal := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-xhigh"
	}, 2*time.Second)
	if firstFinal.ReasoningEffort.Source != hermes.ReasoningEffortSourceConfigDefault {
		t.Fatalf("first final ReasoningEffort.Source = %q, want config default", firstFinal.ReasoningEffort.Source)
	}
	if firstFinal.ReasoningEffort.Effort != configDefault {
		t.Fatalf("first final Effort = %q, want %q", firstFinal.ReasoningEffort.Effort, configDefault)
	}

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "use configured default"}); err != nil {
		t.Fatal(err)
	}
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-low"
	}, 2*time.Second)

	reqs := waitForRequestCount(t, mock, 2, time.Second)
	assertReasoningEffort(t, reqs[0], override)
	assertReasoningEffort(t, reqs[1], configDefault)
	if k.cfg.ReasoningEffort != string(configDefault) {
		t.Fatalf("resident config ReasoningEffort mutated to %q, want %q", k.cfg.ReasoningEffort, configDefault)
	}
}

func TestPerTurnReasoningUnsupportedProviderReportsEvidenceAndOmitsRequest(t *testing.T) {
	mock := hermes.NewMockClient()
	mock.SetProviderStatus(hermes.ProviderStatus{Provider: "anthropic", Runtime: "anthropic_messages"})
	releaseFirstStream := make(chan struct{})
	client := &gatedMockClient{
		MockClient:         mock,
		releaseFirstStream: releaseFirstStream,
	}
	mock.Script([]hermes.Event{{Kind: hermes.EventDone, FinishReason: "stop"}}, "sess-unsupported")

	k := New(Config{
		Model:     "resident-model",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(PlatformEvent{
		Kind:            PlatformEventSubmit,
		Text:            "unsupported reasoning",
		ReasoningEffort: string(hermes.ReasoningEffortHigh),
	}); err != nil {
		t.Fatal(err)
	}

	inFlight := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return (f.Phase == PhaseConnecting || f.Phase == PhaseStreaming) &&
			f.ReasoningEffort.State == hermes.ReasoningEffortStateUnsupported
	}, time.Second)
	if inFlight.ReasoningEffort.Forwarded {
		t.Fatalf("Forwarded = true, want false for unsupported provider: %+v", inFlight.ReasoningEffort)
	}
	if !strings.Contains(inFlight.ReasoningEffort.Reason, "anthropic_messages") {
		t.Fatalf("Reason = %q, want provider runtime evidence", inFlight.ReasoningEffort.Reason)
	}

	close(releaseFirstStream)
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-unsupported"
	}, 2*time.Second)
	reqs := waitForRequestCount(t, mock, 1, time.Second)
	if reqs[0].ReasoningEffort != nil {
		t.Fatalf("ChatRequest.ReasoningEffort = %q, want nil for unsupported provider", *reqs[0].ReasoningEffort)
	}
}

func TestPerTurnReasoningInvalidOverrideReportsEvidenceAndOmitsRequest(t *testing.T) {
	mock := hermes.NewMockClient()
	mock.SetProviderStatus(chatCompletionsProviderStatus())
	releaseFirstStream := make(chan struct{})
	client := &gatedMockClient{
		MockClient:         mock,
		releaseFirstStream: releaseFirstStream,
	}
	mock.Script([]hermes.Event{{Kind: hermes.EventDone, FinishReason: "stop"}}, "sess-invalid")

	k := New(Config{
		Model:     "resident-model",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(PlatformEvent{
		Kind:            PlatformEventSubmit,
		Text:            "invalid reasoning",
		ReasoningEffort: "max",
	}); err != nil {
		t.Fatal(err)
	}

	inFlight := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return (f.Phase == PhaseConnecting || f.Phase == PhaseStreaming) &&
			f.ReasoningEffort.State == hermes.ReasoningEffortStateInvalid
	}, time.Second)
	if inFlight.ReasoningEffort.Forwarded {
		t.Fatalf("Forwarded = true, want false for invalid reasoning effort: %+v", inFlight.ReasoningEffort)
	}
	if !strings.Contains(inFlight.ReasoningEffort.Reason, "invalid reasoning_effort") {
		t.Fatalf("Reason = %q, want invalid reasoning_effort evidence", inFlight.ReasoningEffort.Reason)
	}

	close(releaseFirstStream)
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-invalid"
	}, 2*time.Second)
	reqs := waitForRequestCount(t, mock, 1, time.Second)
	if reqs[0].ReasoningEffort != nil {
		t.Fatalf("ChatRequest.ReasoningEffort = %q, want nil for invalid override", *reqs[0].ReasoningEffort)
	}
}

func assertReasoningEffort(t *testing.T, req hermes.ChatRequest, want hermes.ReasoningEffort) {
	t.Helper()
	if req.ReasoningEffort == nil {
		t.Fatalf("ChatRequest.ReasoningEffort = nil, want %q", want)
	}
	if *req.ReasoningEffort != want {
		t.Fatalf("ChatRequest.ReasoningEffort = %q, want %q", *req.ReasoningEffort, want)
	}
}

func chatCompletionsProviderStatus() hermes.ProviderStatus {
	return hermes.ProviderStatus{Provider: "mock-openai", Runtime: "chat_completions"}
}
