package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DelegateTool is the Go-native delegate_task tool.
type DelegateTool struct {
	manager SubagentManager
	drafter CandidateDrafter
}

type CandidateDraftRequest struct {
	Slug            string
	Goal            string
	Summary         string
	SourceRunID     string
	ParentSessionID string
	ChildAgentID    string
	ToolNames       []string
}

type CandidateDrafter interface {
	DraftCandidate(ctx context.Context, req CandidateDraftRequest) (string, error)
}

// NewDelegateTool wires a DelegateTool to the supplied SubagentManager.
func NewDelegateTool(m SubagentManager, drafter CandidateDrafter) *DelegateTool {
	return &DelegateTool{manager: m, drafter: drafter}
}

func (*DelegateTool) Name() string { return "delegate_task" }

func (*DelegateTool) Description() string {
	return "Delegate a task to a subagent for parallel execution. The subagent runs with its own context, returns a structured JSON result."
}

// Timeout returns 0 so the executor does not impose a deadline; per-subagent
// timeouts are governed via SubagentConfig.Timeout.
func (*DelegateTool) Timeout() time.Duration { return 0 }

func (*DelegateTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"goal":           {"type": "string", "description": "Task goal for the subagent"},
			"context":        {"type": "string", "description": "Optional additional context"},
			"max_iterations": {"type": "integer", "description": "Max LLM turns for the subagent"},
			"toolsets":       {"type": "string", "description": "Comma-separated tool names to allowlist for the child run"},
			"draft_candidate_slug": {"type": "string", "description": "Optional inactive skill slug to draft from a successful delegated run"},
			"allow_no_tool_draft": {"type": "boolean", "description": "Explicit override allowing candidate drafting even when the child emitted no tool calls"}
		},
		"required": ["goal"]
	}`)
}

type delegateArgs struct {
	Goal          string `json:"goal"`
	Context       string `json:"context"`
	MaxIterations int    `json:"max_iterations"`
	Toolsets      string `json:"toolsets"`
	DraftSlug     string `json:"draft_candidate_slug"`
	AllowNoTool   bool   `json:"allow_no_tool_draft"`
}

func (t *DelegateTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in delegateArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("delegate_task: invalid args: %w", err)
	}
	if strings.TrimSpace(in.Goal) == "" {
		return nil, errors.New("delegate_task: goal is required")
	}
	if t.manager == nil {
		return nil, errors.New("delegate_task: manager is required")
	}

	var enabled []string
	if in.Toolsets != "" {
		for _, s := range strings.Split(in.Toolsets, ",") {
			if s = strings.TrimSpace(s); s != "" {
				enabled = append(enabled, s)
			}
		}
	}

	sa, err := t.manager.Spawn(ctx, SubagentConfig{
		Goal:          strings.TrimSpace(in.Goal),
		Context:       strings.TrimSpace(in.Context),
		MaxIterations: in.MaxIterations,
		EnabledTools:  enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("delegate_task: spawn: %w", err)
	}

	result, err := sa.WaitForResult(ctx)
	if err != nil {
		_ = t.manager.Interrupt(sa, "parent ctx cancelled")
		return nil, err
	}

	var candidateID string
	var candidateErr error
	if t.drafter != nil && strings.TrimSpace(in.DraftSlug) != "" && result.Status == StatusCompleted {
		if len(result.ToolCalls) > 0 || in.AllowNoTool {
			candidateID, candidateErr = t.drafter.DraftCandidate(ctx, CandidateDraftRequest{
				Slug:         strings.TrimSpace(in.DraftSlug),
				Goal:         strings.TrimSpace(in.Goal),
				Summary:      result.Summary,
				SourceRunID:  result.ID,
				ChildAgentID: result.ID,
				ToolNames:    toolCallNames(result.ToolCalls),
			})
		}
	}

	out := map[string]any{
		"id":          result.ID,
		"status":      string(result.Status),
		"summary":     result.Summary,
		"exit_reason": result.ExitReason,
		"duration_ms": result.Duration.Milliseconds(),
		"iterations":  result.Iterations,
		"error":       result.Error,
	}
	if len(result.ToolCalls) > 0 {
		out["tool_calls"] = result.ToolCalls
	}
	if candidateID != "" {
		out["candidate_id"] = candidateID
	}
	if candidateErr != nil {
		out["candidate_error"] = candidateErr.Error()
	}
	return json.Marshal(out)
}

func toolCallNames(calls []ToolCallInfo) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		if name := strings.TrimSpace(call.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}
