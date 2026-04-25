package kernel

import (
	"context"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
)

func TestPerTurnModelOverrideIsTurnScoped(t *testing.T) {
	const residentModel = "resident-model"
	const overrideModel = "override-model"

	mock := hermes.NewMockClient()
	releaseFirstStream := make(chan struct{})
	client := &gatedMockClient{
		MockClient:         mock,
		releaseFirstStream: releaseFirstStream,
	}
	mock.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "override response"},
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-override")
	mock.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-resident")

	k := New(Config{
		Model:     residentModel,
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)

	initial := <-k.Render()
	if initial.Model != residentModel {
		t.Fatalf("initial frame Model = %q, want %q", initial.Model, residentModel)
	}

	if err := k.Submit(PlatformEvent{
		Kind:  PlatformEventSubmit,
		Text:  "use override",
		Model: overrideModel,
	}); err != nil {
		t.Fatal(err)
	}

	inFlight := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return (f.Phase == PhaseConnecting || f.Phase == PhaseStreaming) && f.Model == overrideModel
	}, time.Second)
	if inFlight.Model != overrideModel {
		t.Fatalf("in-flight frame Model = %q, want %q", inFlight.Model, overrideModel)
	}

	close(releaseFirstStream)
	firstFinal := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-override"
	}, 2*time.Second)
	if firstFinal.Model != residentModel {
		t.Fatalf("first final idle frame Model = %q, want resident %q", firstFinal.Model, residentModel)
	}

	if err := k.Submit(PlatformEvent{
		Kind: PlatformEventSubmit,
		Text: "use resident",
	}); err != nil {
		t.Fatal(err)
	}
	secondFinal := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-resident"
	}, 2*time.Second)
	if secondFinal.Model != residentModel {
		t.Fatalf("second final idle frame Model = %q, want %q", secondFinal.Model, residentModel)
	}

	reqs := waitForRequestCount(t, mock, 2, time.Second)
	if len(reqs) != 2 {
		t.Fatalf("mock request count = %d, want 2", len(reqs))
	}
	if reqs[0].Model != overrideModel {
		t.Fatalf("first ChatRequest.Model = %q, want %q", reqs[0].Model, overrideModel)
	}
	if reqs[1].Model != residentModel {
		t.Fatalf("second ChatRequest.Model = %q, want %q", reqs[1].Model, residentModel)
	}
	if k.cfg.Model != residentModel {
		t.Fatalf("resident config Model mutated to %q, want %q", k.cfg.Model, residentModel)
	}
}

func TestPerTurnModelBlankOverrideFallsBack(t *testing.T) {
	const residentModel = "resident-model"

	mock := hermes.NewMockClient()
	releaseFirstStream := make(chan struct{})
	client := &gatedMockClient{
		MockClient:         mock,
		releaseFirstStream: releaseFirstStream,
	}
	mock.Script([]hermes.Event{
		{Kind: hermes.EventDone, FinishReason: "stop"},
	}, "sess-blank")

	k := New(Config{
		Model:     residentModel,
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(PlatformEvent{
		Kind:  PlatformEventSubmit,
		Text:  "blank override",
		Model: "   ",
	}); err != nil {
		t.Fatal(err)
	}

	inFlight := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return (f.Phase == PhaseConnecting || f.Phase == PhaseStreaming) && f.Model == residentModel
	}, time.Second)
	if inFlight.Model != residentModel {
		t.Fatalf("in-flight frame Model = %q, want %q", inFlight.Model, residentModel)
	}

	close(releaseFirstStream)
	final := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID == "sess-blank"
	}, 2*time.Second)
	if final.Model != residentModel {
		t.Fatalf("final frame Model = %q, want %q", final.Model, residentModel)
	}

	reqs := waitForRequestCount(t, mock, 1, time.Second)
	if len(reqs) != 1 {
		t.Fatalf("mock request count = %d, want 1", len(reqs))
	}
	if reqs[0].Model != residentModel {
		t.Fatalf("ChatRequest.Model = %q, want %q", reqs[0].Model, residentModel)
	}
}

type gatedMockClient struct {
	*hermes.MockClient
	releaseFirstStream <-chan struct{}
	firstStreamHeld    bool
}

func (c *gatedMockClient) OpenStream(ctx context.Context, req hermes.ChatRequest) (hermes.Stream, error) {
	stream, err := c.MockClient.OpenStream(ctx, req)
	if err != nil {
		return nil, err
	}
	if c.firstStreamHeld {
		return stream, nil
	}
	c.firstStreamHeld = true
	return &gatedStream{
		Stream:  stream,
		release: c.releaseFirstStream,
	}, nil
}

type gatedStream struct {
	hermes.Stream
	release <-chan struct{}
}

func (s *gatedStream) Recv(ctx context.Context) (hermes.Event, error) {
	if s.release != nil {
		select {
		case <-s.release:
			s.release = nil
		case <-ctx.Done():
			return hermes.Event{}, ctx.Err()
		}
	}
	return s.Stream.Recv(ctx)
}

func waitForRequestCount(t *testing.T, mock *hermes.MockClient, want int, timeout time.Duration) []hermes.ChatRequest {
	t.Helper()
	deadline := time.After(timeout)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		reqs := mock.Requests()
		if len(reqs) >= want {
			return reqs
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %d mock requests; got %d", want, len(reqs))
		case <-ticker.C:
		}
	}
}
