package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

var _ tools.Tool = (*DelegateTool)(nil)

type DelegateTool struct {
	mgr *Manager
}

func NewDelegateTool(mgr *Manager) *DelegateTool {
	return &DelegateTool{mgr: mgr}
}

func (*DelegateTool) Name() string { return "delegate_task" }

func (*DelegateTool) Description() string {
	return "Delegate a bounded child task to a subagent and return its run result."
}

func (*DelegateTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"goal":{"type":"string","description":"task for the child subagent"},"context":{"type":"string","description":"scoped context for the child"},"model":{"type":"string","description":"optional model override"},"max_iterations":{"type":"integer","minimum":1,"description":"maximum child iterations"},"timeout_seconds":{"type":"integer","minimum":1,"description":"child timeout in seconds"},"allowed_tools":{"type":"array","items":{"type":"string"},"description":"tool names the child may use"}},"required":["goal"],"additionalProperties":false}`)
}

func (*DelegateTool) Timeout() time.Duration { return 2 * time.Minute }

func (t *DelegateTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t == nil || t.mgr == nil {
		return nil, fmt.Errorf("subagent: nil delegate manager")
	}

	var in struct {
		Goal           string   `json:"goal"`
		Context        string   `json:"context"`
		Model          string   `json:"model"`
		MaxIterations  int      `json:"max_iterations"`
		TimeoutSeconds int      `json:"timeout_seconds"`
		AllowedTools   []string `json:"allowed_tools"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("subagent: invalid delegate args: %w", err)
	}

	spec := Spec{
		Goal:          in.Goal,
		Context:       in.Context,
		Model:         in.Model,
		AllowedTools:  append([]string(nil), in.AllowedTools...),
		MaxIterations: in.MaxIterations,
		Timeout:       time.Duration(in.TimeoutSeconds) * time.Second,
	}

	handle, err := t.mgr.Start(ctx, spec)
	if err != nil {
		return nil, err
	}

	result, waitErr := handle.Wait(ctx)
	if waitErr != nil {
		if result.Status == "" {
			switch ctx.Err() {
			case context.Canceled:
				result.Status = StatusCancelled
			case context.DeadlineExceeded:
				result.Status = StatusTimedOut
			default:
				result.Status = StatusFailed
			}
		}
		if result.Error == "" {
			result.Error = waitErr.Error()
		}
	}

	out := struct {
		RunID   string       `json:"run_id"`
		Status  ResultStatus `json:"status"`
		Summary string       `json:"summary,omitempty"`
		Error   string       `json:"error,omitempty"`
	}{
		RunID:   result.RunID,
		Status:  result.Status,
		Summary: result.Summary,
		Error:   result.Error,
	}
	if out.Status == "" {
		out.Status = StatusFailed
	}

	raw, marshalErr := json.Marshal(out)
	if marshalErr != nil {
		return nil, marshalErr
	}
	if waitErr != nil {
		return raw, fmt.Errorf("subagent: wait child: %w", waitErr)
	}
	return raw, nil
}
