package cron

import (
	"context"
	"errors"
	"testing"
)

func TestFuncSink_Forwards(t *testing.T) {
	var got string
	sink := FuncSink(func(ctx context.Context, text string) error {
		got = text
		return nil
	})
	if err := sink.Deliver(context.Background(), "hello"); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got != "hello" {
		t.Errorf("got = %q, want 'hello'", got)
	}
}

func TestFuncSink_PropagatesError(t *testing.T) {
	stub := errors.New("stub failure")
	sink := FuncSink(func(ctx context.Context, text string) error { return stub })
	err := sink.Deliver(context.Background(), "x")
	if !errors.Is(err, stub) {
		t.Errorf("err = %v, want stub", err)
	}
}
