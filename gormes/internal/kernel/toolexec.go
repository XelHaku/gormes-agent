package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
)

// toolResult is the internal per-call output feeding back into the next
// ChatRequest as a role=tool Message.
type toolResult struct {
	ID      string
	Name    string
	Content string // JSON string — errors are JSON-encoded {"error":"..."}
}

// executeToolCalls runs each tool call sequentially with per-call timeout
// and panic recovery. Honours runCtx cancellation between calls. Returns
// results in the same order as calls.
func (k *Kernel) executeToolCalls(runCtx context.Context, calls []hermes.ToolCall) []toolResult {
	results := make([]toolResult, len(calls))
	for i, call := range calls {
		select {
		case <-runCtx.Done():
			results[i] = toolResult{
				ID: call.ID, Name: call.Name,
				Content: `{"error":"cancelled before execution"}`,
			}
			continue
		default:
		}

		if k.cfg.Tools == nil {
			results[i] = toolResult{
				ID: call.ID, Name: call.Name,
				Content: `{"error":"no tool registry configured"}`,
			}
			continue
		}

		tool, ok := k.cfg.Tools.Get(call.Name)
		if !ok {
			results[i] = toolResult{
				ID: call.ID, Name: call.Name,
				Content: fmt.Sprintf(`{"error":"unknown tool: %q"}`, call.Name),
			}
			k.addSoul("tool unknown: " + call.Name)
			continue
		}

		timeout := tool.Timeout()
		if timeout <= 0 {
			timeout = k.cfg.MaxToolDuration
		}
		if timeout <= 0 {
			timeout = 30 * time.Second
		}

		callCtx, cancel := context.WithTimeout(runCtx, timeout)

		k.addSoul("tool: " + call.Name)
		k.emitFrame("executing tool: " + call.Name)

		payload, err := safeExecute(callCtx, tool, call.Arguments)
		cancel()

		if err != nil {
			results[i] = toolResult{
				ID: call.ID, Name: call.Name,
				Content: fmt.Sprintf(`{"error":%q}`, err.Error()),
			}
			k.addSoul("tool error: " + call.Name + ": " + err.Error())
			continue
		}
		results[i] = toolResult{ID: call.ID, Name: call.Name, Content: string(payload)}
		k.addSoul("tool done: " + call.Name)
	}
	return results
}

// safeExecute wraps Tool.Execute with panic recovery so a misbehaving tool
// cannot crash the kernel goroutine.
func safeExecute(ctx context.Context, t tools.Tool, args json.RawMessage) (result json.RawMessage, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool panicked: %v", r)
			result = nil
		}
	}()
	return t.Execute(ctx, args)
}
