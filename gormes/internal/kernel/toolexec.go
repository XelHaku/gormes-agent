package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
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
		start := time.Now()
		recordAudit := func(status string, result json.RawMessage, err error) {
			if k.cfg.ToolAudit == nil {
				return
			}
			rec := audit.Record{
				Timestamp:       time.Now().UTC(),
				Source:          "kernel",
				SessionID:       k.sessionID,
				Tool:            call.Name,
				Args:            append(json.RawMessage(nil), call.Arguments...),
				DurationMs:      time.Since(start).Milliseconds(),
				Status:          status,
				ResultSizeBytes: len(result),
			}
			if err != nil {
				rec.Error = err.Error()
			}
			if auditErr := k.cfg.ToolAudit.Record(rec); auditErr != nil && k.log != nil {
				k.log.Warn("kernel: append tool audit failed", "tool", call.Name, "err", auditErr)
			}
		}

		select {
		case <-runCtx.Done():
			results[i] = toolResult{
				ID: call.ID, Name: call.Name,
				Content: `{"error":"cancelled before execution"}`,
			}
			recordAudit("cancelled", nil, errors.New("cancelled before execution"))
			continue
		default:
		}

		if k.cfg.Tools == nil {
			results[i] = toolResult{
				ID: call.ID, Name: call.Name,
				Content: `{"error":"no tool registry configured"}`,
			}
			recordAudit("failed", nil, errors.New("no tool registry configured"))
			continue
		}

		tool, ok := k.cfg.Tools.Get(call.Name)
		if !ok {
			results[i] = toolResult{
				ID: call.ID, Name: call.Name,
				Content: fmt.Sprintf(`{"error":"unknown tool: %q"}`, call.Name),
			}
			k.addSoul("tool unknown: " + call.Name)
			recordAudit("failed", nil, fmt.Errorf("unknown tool: %q", call.Name))
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
			recordAudit("failed", nil, err)
			continue
		}
		results[i] = toolResult{ID: call.ID, Name: call.Name, Content: string(payload)}
		k.addSoul("tool done: " + call.Name)
		recordAudit("completed", payload, nil)
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
