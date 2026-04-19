package kernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
)

func newKernelWithRegistry(t *testing.T, reg *tools.Registry) *Kernel {
	t.Helper()
	return New(Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         Admission{MaxBytes: 200_000, MaxLines: 10_000},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
	}, hermes.NewMockClient(), store.NewNoop(), telemetry.New(), nil)
}

func TestExecuteToolCalls_UnknownToolReturnsErrorResult(t *testing.T) {
	reg := tools.NewRegistry()
	k := newKernelWithRegistry(t, reg)
	res := k.executeToolCalls(context.Background(), []hermes.ToolCall{
		{ID: "c1", Name: "not_registered", Arguments: json.RawMessage(`{}`)},
	})
	if len(res) != 1 {
		t.Fatalf("len = %d, want 1", len(res))
	}
	if !strings.Contains(res[0].Content, "unknown tool") {
		t.Errorf("content = %q, want to contain 'unknown tool'", res[0].Content)
	}
}

func TestExecuteToolCalls_PanicRecovered(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.MockTool{
		NameStr: "boom",
		ExecuteFn: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			panic("synthetic panic")
		},
	})
	k := newKernelWithRegistry(t, reg)
	res := k.executeToolCalls(context.Background(), []hermes.ToolCall{
		{ID: "c1", Name: "boom", Arguments: json.RawMessage(`{}`)},
	})
	if !strings.Contains(res[0].Content, "panicked") {
		t.Errorf("content = %q, want to contain 'panicked'", res[0].Content)
	}
}

func TestExecuteToolCalls_TimeoutHonoured(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.MockTool{
		NameStr:  "slow",
		TimeoutD: 20 * time.Millisecond,
		ExecuteFn: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(1 * time.Second):
				return json.RawMessage(`{"ok":true}`), nil
			}
		},
	})
	k := newKernelWithRegistry(t, reg)
	start := time.Now()
	res := k.executeToolCalls(context.Background(), []hermes.ToolCall{
		{ID: "c1", Name: "slow", Arguments: json.RawMessage(`{}`)},
	})
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("elapsed = %v, want ~20ms (the tool's timeout)", elapsed)
	}
	if !strings.Contains(res[0].Content, "deadline exceeded") && !strings.Contains(res[0].Content, "context") {
		t.Errorf("content = %q, want a context-deadline error", res[0].Content)
	}
}

func TestExecuteToolCalls_CancelBetweenCalls(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.MockTool{NameStr: "a"})
	reg.MustRegister(&tools.MockTool{NameStr: "b"})
	k := newKernelWithRegistry(t, reg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := k.executeToolCalls(ctx, []hermes.ToolCall{
		{ID: "1", Name: "a", Arguments: json.RawMessage(`{}`)},
		{ID: "2", Name: "b", Arguments: json.RawMessage(`{}`)},
	})
	for i := range res {
		if !strings.Contains(res[i].Content, "cancelled") {
			t.Errorf("res[%d] content = %q, want to mention cancelled", i, res[i].Content)
		}
	}
}
