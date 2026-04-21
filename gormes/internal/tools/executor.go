package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolExecutor executes tools on behalf of an agent.
type ToolExecutor interface {
	Execute(ctx context.Context, req ToolRequest) (<-chan ToolEvent, error)
}

// ToolRequest is a single tool invocation request submitted to a ToolExecutor.
type ToolRequest struct {
	AgentID  string
	ToolName string
	Input    json.RawMessage
	Metadata map[string]string
}

// ToolEvent is one observation from a tool invocation.
type ToolEvent struct {
	Type   string
	Output json.RawMessage
	Err    error
}

// InProcessToolExecutor runs tools directly against a Registry in the current
// process, honoring each tool's declared timeout.
type InProcessToolExecutor struct {
	registry *Registry
}

// NewInProcessToolExecutor returns an in-process executor backed by reg.
func NewInProcessToolExecutor(reg *Registry) *InProcessToolExecutor {
	return &InProcessToolExecutor{registry: reg}
}

// Execute looks up the requested tool and streams started→output→completed, or
// started→failed when the tool returns an error.
func (e *InProcessToolExecutor) Execute(ctx context.Context, req ToolRequest) (<-chan ToolEvent, error) {
	tool, ok := e.registry.Get(req.ToolName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTool, req.ToolName)
	}

	ch := make(chan ToolEvent, 4)
	go func() {
		defer close(ch)

		ch <- ToolEvent{Type: "started"}

		execCtx := ctx
		if timeout := tool.Timeout(); timeout > 0 {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		out, err := tool.Execute(execCtx, req.Input)
		if err != nil {
			ch <- ToolEvent{Type: "failed", Err: err}
			return
		}

		ch <- ToolEvent{Type: "output", Output: out}
		ch <- ToolEvent{Type: "completed"}
	}()

	return ch, nil
}
