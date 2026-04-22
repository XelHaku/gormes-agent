// gormes/internal/subagent/types.go

// Package subagent implements goroutine-per-subagent execution isolation
// with deterministic context cancellation, bounded batch concurrency, and
// a swappable Runner interface. See gormes/docs/superpowers/specs/2026-04-20-gormes-phase2e-subagent-design.md.
package subagent

import (
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

// SubagentConfig is the per-subagent configuration handed to the Runner.
// Defaults are applied at Spawn time, not at TOML decode time.
type SubagentConfig struct {
	Goal          string
	Context       string
	MaxIterations int           // 0 → DefaultMaxIterations at Spawn time
	EnabledTools  []string      // empty → all parent tools minus BlockedTools
	Model         string        // empty → inherit from parent
	Timeout       time.Duration // 0 → no timeout
	toolExecutor  tools.ToolExecutor
	toolAudit     audit.Recorder
	agentID       string
}

// EventType discriminates SubagentEvent values streamed from runner to parent.
type EventType string

const (
	EventStarted     EventType = "started"
	EventProgress    EventType = "progress"
	EventToolCall    EventType = "tool_call"
	EventOutput      EventType = "output"
	EventCompleted   EventType = "completed"
	EventFailed      EventType = "failed"
	EventInterrupted EventType = "interrupted"
)

// SubagentEvent is a single observation streamed back to the parent during execution.
type SubagentEvent struct {
	Type     EventType
	Message  string
	ToolCall *ToolCallInfo
	Progress *ProgressInfo
}

// ToolCallInfo summarises a tool invocation observed from inside the subagent.
type ToolCallInfo struct {
	Name       string
	ArgsBytes  int
	ResultSize int
	Status     string
}

// ProgressInfo is an iteration tick from a long-running runner.
type ProgressInfo struct {
	Iteration int
	Message   string
}

// ResultStatus is the terminal status of a subagent.
type ResultStatus string

const (
	StatusCompleted   ResultStatus = "completed"
	StatusFailed      ResultStatus = "failed"
	StatusInterrupted ResultStatus = "interrupted"
	StatusError       ResultStatus = "error"
)

// SubagentResult is published exactly once when a subagent finishes.
type SubagentResult struct {
	ID         string
	Status     ResultStatus
	Summary    string
	ExitReason string
	Duration   time.Duration
	Iterations int
	ToolCalls  []ToolCallInfo
	Error      string
}
