package hermes

import (
	"context"
	"io"
	"testing"
)

func TestMockClient_Script(t *testing.T) {
	m := NewMockClient()
	m.Script([]Event{
		{Kind: EventToken, Token: "hel", TokensOut: 1},
		{Kind: EventToken, Token: "lo", TokensOut: 2},
		{Kind: EventDone, FinishReason: "stop", TokensIn: 5, TokensOut: 2},
	}, "sess-abc")

	s, err := m.OpenStream(context.Background(), ChatRequest{Model: "x"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var got []Event
	for {
		e, rerr := s.Recv(context.Background())
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			t.Fatal(rerr)
		}
		got = append(got, e)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[2].FinishReason != "stop" {
		t.Errorf("final finish = %q", got[2].FinishReason)
	}
	if s.SessionID() != "sess-abc" {
		t.Errorf("SessionID = %q, want sess-abc", s.SessionID())
	}
}

func TestMockClient_CancelCutsStream(t *testing.T) {
	m := NewMockClient()
	m.Script([]Event{
		{Kind: EventToken, Token: "a"},
		{Kind: EventToken, Token: "b"},
		{Kind: EventToken, Token: "c"},
		{Kind: EventDone},
	}, "")

	ctx, cancel := context.WithCancel(context.Background())
	s, _ := m.OpenStream(ctx, ChatRequest{})
	defer s.Close()

	// Consume one delta, then cancel.
	if _, err := s.Recv(ctx); err != nil {
		t.Fatalf("first Recv: %v", err)
	}
	cancel()

	// Subsequent Recv must return ctx.Err promptly.
	_, err := s.Recv(ctx)
	if err != context.Canceled {
		t.Errorf("Recv after cancel = %v, want context.Canceled", err)
	}
}

func TestMockClient_HealthError(t *testing.T) {
	m := NewMockClient()
	m.SetHealth(&HTTPError{Status: 503})
	if err := m.Health(context.Background()); err == nil {
		t.Error("expected health error")
	}
}

func TestMockClient_RunEventsScripted(t *testing.T) {
	m := NewMockClient()
	m.ScriptRunEvents([]RunEvent{
		{Type: RunEventToolStarted, ToolName: "terminal"},
		{Type: RunEventToolCompleted, ToolName: "terminal"},
	})
	s, err := m.OpenRunEvents(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var got []RunEvent
	for {
		e, rerr := s.Recv(context.Background())
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			t.Fatal(rerr)
		}
		got = append(got, e)
	}
	if len(got) != 2 {
		t.Fatalf("got %d run events, want 2", len(got))
	}
}

func TestMockClient_RunEventsUnscripted_ReturnsNotSupported(t *testing.T) {
	m := NewMockClient()
	_, err := m.OpenRunEvents(context.Background(), "r1")
	if err != ErrRunEventsNotSupported {
		t.Errorf("err = %v, want ErrRunEventsNotSupported", err)
	}
}
