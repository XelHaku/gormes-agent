package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

type recordingStreamSink struct {
	calls []streamDeliveryCall
	errs  map[string]error
}

type streamDeliveryCall struct {
	target DeliveryTarget
	frame  kernel.RenderFrame
}

func (s *recordingStreamSink) DeliverFrame(_ context.Context, target DeliveryTarget, frame kernel.RenderFrame) error {
	s.calls = append(s.calls, streamDeliveryCall{target: target, frame: frame})
	if s.errs != nil {
		return s.errs[target.String()]
	}
	return nil
}

func TestGatewayStreamConsumer_FanOutsToMultipleTargets(t *testing.T) {
	sink := &recordingStreamSink{}
	consumer := NewGatewayStreamConsumer(sink)
	frame := kernel.RenderFrame{Phase: kernel.PhaseStreaming, DraftText: "partial", SessionID: "sess-1"}
	targets := []DeliveryTarget{
		{Platform: "telegram", ChatID: "42", IsExplicit: true},
		{Platform: "discord", ChatID: "99", IsExplicit: true},
	}

	results := consumer.FanOut(context.Background(), frame, targets)

	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	if len(sink.calls) != 2 {
		t.Fatalf("sink calls = %d, want 2", len(sink.calls))
	}
	for i, want := range targets {
		if sink.calls[i].target != want {
			t.Fatalf("call %d target = %+v, want %+v", i, sink.calls[i].target, want)
		}
		if sink.calls[i].frame.SessionID != "sess-1" {
			t.Fatalf("call %d SessionID = %q, want %q", i, sink.calls[i].frame.SessionID, "sess-1")
		}
		if results[i].Target != want || results[i].Err != nil {
			t.Fatalf("result %d = %+v, want target %+v with nil err", i, results[i], want)
		}
	}
}

func TestGatewayStreamConsumer_FanOutContinuesAfterError(t *testing.T) {
	sink := &recordingStreamSink{
		errs: map[string]error{"telegram:42": errors.New("send failed")},
	}
	consumer := NewGatewayStreamConsumer(sink)
	frame := kernel.RenderFrame{Phase: kernel.PhaseIdle, SessionID: "sess-2"}
	targets := []DeliveryTarget{
		{Platform: "telegram", ChatID: "42", IsExplicit: true},
		{Platform: "discord", ChatID: "99", IsExplicit: true},
	}

	results := consumer.FanOut(context.Background(), frame, targets)

	if len(sink.calls) != 2 {
		t.Fatalf("sink calls = %d, want 2", len(sink.calls))
	}
	if results[0].Err == nil {
		t.Fatal("first result error = nil, want non-nil")
	}
	if results[1].Err != nil {
		t.Fatalf("second result error = %v, want nil", results[1].Err)
	}
}
