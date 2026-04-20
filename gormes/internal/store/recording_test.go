package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRecordingStore_CapturesCommands(t *testing.T) {
	r := NewRecording()

	ctx := context.Background()
	_, err := r.Exec(ctx, Command{Kind: AppendUserTurn, Payload: json.RawMessage(`{"x":1}`)})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	_, err = r.Exec(ctx, Command{Kind: FinalizeAssistantTurn, Payload: json.RawMessage(`{"x":2}`)})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	got := r.Commands()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Kind != AppendUserTurn {
		t.Errorf("got[0].Kind = %v, want AppendUserTurn", got[0].Kind)
	}
	if got[1].Kind != FinalizeAssistantTurn {
		t.Errorf("got[1].Kind = %v, want FinalizeAssistantTurn", got[1].Kind)
	}
}

func TestRecordingStore_ConcurrentSafe(t *testing.T) {
	r := NewRecording()
	ctx := context.Background()
	done := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = r.Exec(ctx, Command{Kind: AppendUserTurn})
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
	if got := len(r.Commands()); got != 50 {
		t.Errorf("len(Commands) = %d, want 50", got)
	}
}

func TestRecordingStore_CtxCancelHonored(t *testing.T) {
	r := NewRecording()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Exec(ctx, Command{Kind: AppendUserTurn}); err == nil {
		t.Error("Exec on canceled ctx should return ctx.Err(), got nil")
	}
}
