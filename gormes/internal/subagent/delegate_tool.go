package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

var _ tools.Tool = (*DelegateTool)(nil)

const (
	delegateToolKernelTimeout       = 2 * time.Minute
	delegateToolTimeoutSafetyBuffer = 10 * time.Second
	delegateToolMaxChildTimeout     = delegateToolKernelTimeout - delegateToolTimeoutSafetyBuffer
)

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
	return json.RawMessage(fmt.Sprintf(`{"type":"object","properties":{"goal":{"type":"string","description":"task for the child subagent"},"context":{"type":"string","description":"scoped context for the child"},"model":{"type":"string","description":"optional model override"},"max_iterations":{"type":"integer","minimum":1,"description":"maximum child iterations"},"timeout_seconds":{"type":"integer","minimum":1,"maximum":%d,"description":"child timeout in seconds; must stay within the delegate_task tool budget"},"allowed_tools":{"type":"array","items":{"type":"string"},"description":"tool names the child may use"}},"required":["goal"],"additionalProperties":false}`,
		delegateToolMaxChildTimeout/time.Second))
}

func (*DelegateTool) Timeout() time.Duration { return delegateToolKernelTimeout }

func (t *DelegateTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t == nil || t.mgr == nil {
		return nil, fmt.Errorf("subagent: nil delegate manager")
	}

	var in struct {
		Goal           string   `json:"goal"`
		Context        string   `json:"context"`
		Model          string   `json:"model"`
		MaxIterations  int      `json:"max_iterations"`
		TimeoutSeconds *int     `json:"timeout_seconds"`
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
		Depth:         1,
	}
	timeoutSource := "delegation default timeout"
	if in.TimeoutSeconds != nil {
		if *in.TimeoutSeconds <= 0 {
			return nil, fmt.Errorf("subagent: timeout_seconds must be positive")
		}
		spec.Timeout = time.Duration(*in.TimeoutSeconds) * time.Second
		timeoutSource = "timeout_seconds"
	} else {
		if t == nil || t.mgr == nil {
			return nil, fmt.Errorf("subagent: nil delegate manager")
		}
		spec.Timeout = t.mgr.cfg.DefaultTimeout
	}
	if err := ValidateDelegateTimeout(spec.Timeout, timeoutSource); err != nil {
		return nil, err
	}

	handle, err := t.mgr.Start(ctx, spec)
	if err != nil {
		return nil, err
	}

	result, waitErr := handle.Wait(ctx)
	if waitErr != nil {
		return nil, fmt.Errorf("subagent: wait child: %w", waitErr)
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
	return raw, nil
}

func ValidateDelegateTimeout(timeout time.Duration, source string) error {
	if timeout <= 0 {
		return fmt.Errorf("subagent: %s must be positive", source)
	}
	if timeout > delegateToolMaxChildTimeout {
		return fmt.Errorf("subagent: %s %s exceeds delegate_task budget of %s", source, timeout, delegateToolMaxChildTimeout)
	}
	return nil
}

func ValidateDelegationConfig(cfg config.DelegationCfg) error {
	if cfg.DefaultMaxIterations <= 0 {
		return fmt.Errorf("subagent: delegation default max_iterations must be positive")
	}
	if cfg.MaxChildDepth < 1 {
		return fmt.Errorf("subagent: delegation max_child_depth must be at least 1")
	}
	return ValidateDelegateTimeout(cfg.DefaultTimeout, "delegation default timeout")
}
