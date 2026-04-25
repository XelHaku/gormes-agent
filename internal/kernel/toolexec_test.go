package kernel

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
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

func TestExecuteToolCalls_ContextCancelCancelsEveryConcurrentWorker(t *testing.T) {
	reg := tools.NewRegistry()
	started := make(chan string, 3)
	cancelled := make(chan string, 3)

	for _, name := range []string{"alpha", "bravo", "charlie"} {
		name := name
		reg.MustRegister(&tools.MockTool{
			NameStr:  name,
			TimeoutD: 5 * time.Second,
			ExecuteFn: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
				started <- name
				<-ctx.Done()
				cancelled <- name
				return nil, ctx.Err()
			},
		})
	}
	k := newKernelWithRegistry(t, reg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan []toolResult, 1)
	go func() {
		done <- k.executeToolCalls(ctx, []hermes.ToolCall{
			{ID: "call_alpha", Name: "alpha", Arguments: json.RawMessage(`{}`)},
			{ID: "call_bravo", Name: "bravo", Arguments: json.RawMessage(`{}`)},
			{ID: "call_charlie", Name: "charlie", Arguments: json.RawMessage(`{}`)},
		})
	}()

	requireNames(t, started, []string{"alpha", "bravo", "charlie"}, 500*time.Millisecond)
	cancel()
	requireNames(t, cancelled, []string{"alpha", "bravo", "charlie"}, time.Second)

	select {
	case results := <-done:
		if len(results) != 3 {
			t.Fatalf("result count = %d, want 3", len(results))
		}
		for i, result := range results {
			if !strings.Contains(result.Content, "tool execution cancelled") {
				t.Fatalf("results[%d].Content = %q, want coherent cancellation envelope", i, result.Content)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("executeToolCalls did not return within 1s after context cancellation")
	}
}

func requireNames(t *testing.T, ch <-chan string, want []string, timeout time.Duration) {
	t.Helper()

	deadline := time.After(timeout)
	remaining := make(map[string]struct{}, len(want))
	for _, name := range want {
		remaining[name] = struct{}{}
	}

	for len(remaining) > 0 {
		select {
		case name := <-ch:
			delete(remaining, name)
		case <-deadline:
			t.Fatalf("timeout waiting for names %v", sortedKeys(remaining))
		}
	}
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
