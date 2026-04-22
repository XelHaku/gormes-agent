// gormes/internal/subagent/runner.go
package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

// Runner is the swappable inner loop of a subagent. Phase 2.E closeout ships
// StubRunner; future slices may replace it with an LLM-backed runner.
//
// Contracts (binding on every implementation):
//
//  1. Run MUST return promptly after ctx.Done() fires. "Promptly" means within
//     a small bounded time, not blocked forever.
//  2. Run MUST NOT close the events channel. The manager owns the channel
//     lifecycle.
//  3. Run MAY emit zero or more events.
//  4. Run MUST return a non-nil *SubagentResult.
type Runner interface {
	Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult
}

// StubRunner is the intentionally shipped runtime seam for Phase 2.E closeout.
// It proves lifecycle, cancellation, and tool-surface wiring without yet
// adding a nested LLM loop.
type StubRunner struct{}

func (StubRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	start := time.Now()
	if blocked := blockedToolRequest(cfg.EnabledTools); blocked != "" {
		msg := "blocked tool requested for child run: " + blocked
		select {
		case events <- SubagentEvent{Type: EventFailed, Message: msg}:
		case <-ctx.Done():
			return &SubagentResult{
				Status:     StatusInterrupted,
				ExitReason: "ctx_cancelled_before_start",
				Duration:   time.Since(start),
				Error:      ctx.Err().Error(),
			}
		}
		return &SubagentResult{
			Status:     StatusFailed,
			ExitReason: "blocked_tool_request",
			Duration:   time.Since(start),
			Error:      msg,
		}
	}

	select {
	case events <- SubagentEvent{Type: EventStarted, Message: cfg.Goal}:
	case <-ctx.Done():
		return &SubagentResult{
			Status:     StatusInterrupted,
			ExitReason: "ctx_cancelled_before_start",
			Duration:   time.Since(start),
			Error:      ctx.Err().Error(),
		}
	}

	select {
	case events <- SubagentEvent{Type: EventCompleted, Message: "stub"}:
	case <-ctx.Done():
		return &SubagentResult{
			Status:     StatusInterrupted,
			ExitReason: "ctx_cancelled_during_stub",
			Duration:   time.Since(start),
			Error:      ctx.Err().Error(),
		}
	}

	return &SubagentResult{
		Status:     StatusCompleted,
		Summary:    cfg.Goal,
		ExitReason: "stub_runner_no_llm_yet",
		Duration:   time.Since(start),
		Iterations: 0,
	}
}

// HermesRunner executes a real child Hermes streaming loop with in-process
// tool execution. It keeps the same cancellation and allowlist contracts as
// StubRunner while replacing the "stub" execution seam.
type HermesRunner struct {
	client       hermes.Client
	defaultModel string
	descriptors  map[string]hermes.ToolDescriptor
}

func NewHermesRunner(client hermes.Client, defaultModel string, descriptors []hermes.ToolDescriptor) HermesRunner {
	index := make(map[string]hermes.ToolDescriptor, len(descriptors))
	for _, d := range descriptors {
		if name := strings.TrimSpace(d.Name); name != "" {
			index[name] = d
		}
	}
	return HermesRunner{client: client, defaultModel: strings.TrimSpace(defaultModel), descriptors: index}
}

func (r HermesRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	start := time.Now()
	if blocked := blockedToolRequest(cfg.EnabledTools); blocked != "" {
		msg := "blocked tool requested for child run: " + blocked
		emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
		return &SubagentResult{Status: StatusFailed, ExitReason: "blocked_tool_request", Duration: time.Since(start), Error: msg}
	}
	if r.client == nil {
		msg := "no child hermes client configured"
		emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
		return &SubagentResult{Status: StatusFailed, ExitReason: "child_client_not_configured", Duration: time.Since(start), Error: msg}
	}
	if !emitSubagentEvent(ctx, events, SubagentEvent{Type: EventStarted, Message: cfg.Goal}) {
		return &SubagentResult{Status: StatusInterrupted, ExitReason: "ctx_cancelled_before_start", Duration: time.Since(start), Error: ctx.Err().Error()}
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = r.defaultModel
	}
	if model == "" {
		model = "gpt-5.3-codex"
	}

	request := hermes.ChatRequest{
		Model:    model,
		Stream:   true,
		Messages: []hermes.Message{{Role: "user", Content: buildChildPrompt(cfg)}},
		Tools:    r.allowlistedDescriptors(cfg.EnabledTools),
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 1
	}

	var (
		iteration int
		draft     strings.Builder
		audit     []ToolCallInfo
	)

	for {
		iteration++
		if iteration > maxIterations {
			msg := fmt.Sprintf("child run max iterations exceeded (%d)", maxIterations)
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
			return &SubagentResult{Status: StatusFailed, ExitReason: "max_iterations_exceeded", Duration: time.Since(start), Iterations: iteration - 1, ToolCalls: audit, Error: msg}
		}
		draft.Reset()

		stream, err := r.client.OpenStream(ctx, request)
		if err != nil {
			if ctx.Err() != nil {
				return &SubagentResult{Status: StatusInterrupted, ExitReason: "ctx_cancelled_during_stream", Duration: time.Since(start), Iterations: iteration, ToolCalls: audit, Error: ctx.Err().Error()}
			}
			msg := "child stream open failed: " + err.Error()
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
			return &SubagentResult{Status: StatusFailed, ExitReason: "child_stream_open_failed", Duration: time.Since(start), Iterations: iteration, ToolCalls: audit, Error: msg}
		}

		var final hermes.Event
		gotDone := false
		for {
			ev, recvErr := stream.Recv(ctx)
			if recvErr != nil {
				if errors.Is(recvErr, io.EOF) {
					break
				}
				_ = stream.Close()
				if ctx.Err() != nil {
					return &SubagentResult{Status: StatusInterrupted, ExitReason: "ctx_cancelled_during_stream", Duration: time.Since(start), Iterations: iteration, ToolCalls: audit, Error: ctx.Err().Error()}
				}
				msg := "child stream recv failed: " + recvErr.Error()
				emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
				return &SubagentResult{Status: StatusFailed, ExitReason: "child_stream_recv_failed", Duration: time.Since(start), Iterations: iteration, ToolCalls: audit, Error: msg}
			}
			switch ev.Kind {
			case hermes.EventToken:
				if ev.Token != "" {
					draft.WriteString(ev.Token)
					emitSubagentEvent(ctx, events, SubagentEvent{Type: EventOutput, Message: ev.Token})
				}
			case hermes.EventDone:
				gotDone = true
				final = ev
			}
		}
		_ = stream.Close()
		if sid := stream.SessionID(); sid != "" {
			request.SessionID = sid
		}

		if !gotDone {
			msg := "child stream closed without finish_reason"
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
			return &SubagentResult{Status: StatusFailed, ExitReason: "child_stream_missing_finish_reason", Duration: time.Since(start), Iterations: iteration, ToolCalls: audit, Error: msg}
		}

		if final.FinishReason != "tool_calls" {
			summary := strings.TrimSpace(draft.String())
			if summary == "" {
				summary = cfg.Goal
			}
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventCompleted, Message: summary})
			return &SubagentResult{Status: StatusCompleted, Summary: summary, ExitReason: final.FinishReason, Duration: time.Since(start), Iterations: iteration, ToolCalls: audit}
		}

		if len(final.ToolCalls) == 0 {
			msg := "child stream reported tool_calls with empty payload"
			emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
			return &SubagentResult{Status: StatusFailed, ExitReason: "child_tool_calls_missing", Duration: time.Since(start), Iterations: iteration, ToolCalls: audit, Error: msg}
		}

		request.Messages = append(request.Messages, hermes.Message{Role: "assistant", Content: draft.String(), ToolCalls: final.ToolCalls})
		for _, tc := range final.ToolCalls {
			out, info, toolErr := executeChildTool(ctx, cfg, events, tools.ToolRequest{ToolName: tc.Name, Input: tc.Arguments})
			audit = append(audit, info)
			if toolErr != nil {
				msg := "child tool execution failed: " + toolErr.Error()
				emitSubagentEvent(ctx, events, SubagentEvent{Type: EventFailed, Message: msg})
				return &SubagentResult{Status: StatusFailed, ExitReason: "child_tool_execution_failed", Duration: time.Since(start), Iterations: iteration, ToolCalls: audit, Error: msg}
			}
			request.Messages = append(request.Messages, hermes.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Name, Content: string(out)})
		}
	}
}

func (r HermesRunner) allowlistedDescriptors(enabled []string) []hermes.ToolDescriptor {
	if len(r.descriptors) == 0 {
		return nil
	}
	if len(enabled) == 0 {
		out := make([]hermes.ToolDescriptor, 0, len(r.descriptors))
		for name, d := range r.descriptors {
			if BlockedTools[name] {
				continue
			}
			out = append(out, d)
		}
		return out
	}
	out := make([]hermes.ToolDescriptor, 0, len(enabled))
	for _, name := range enabled {
		if BlockedTools[name] {
			continue
		}
		if d, ok := r.descriptors[name]; ok {
			out = append(out, d)
		}
	}
	return out
}

func buildChildPrompt(cfg SubagentConfig) string {
	goal := strings.TrimSpace(cfg.Goal)
	ctx := strings.TrimSpace(cfg.Context)
	if goal == "" {
		goal = "Complete the delegated task and return a concise result."
	}
	if ctx == "" {
		return goal
	}
	return goal + "\n\nContext:\n" + ctx
}

func emitSubagentEvent(ctx context.Context, events chan<- SubagentEvent, ev SubagentEvent) bool {
	if events == nil {
		return true
	}
	select {
	case events <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

func executeChildTool(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent, req tools.ToolRequest) (json.RawMessage, ToolCallInfo, error) {
	start := time.Now()
	info := ToolCallInfo{
		Name:      req.ToolName,
		ArgsBytes: len(req.Input),
	}
	recordAudit := func(status string, result json.RawMessage, err error) {
		if cfg.toolAudit == nil {
			return
		}
		rec := audit.Record{
			Timestamp:       time.Now().UTC(),
			Source:          "delegate_task",
			AgentID:         cfg.agentID,
			Tool:            req.ToolName,
			Args:            append(json.RawMessage(nil), req.Input...),
			DurationMs:      time.Since(start).Milliseconds(),
			Status:          status,
			ResultSizeBytes: len(result),
		}
		if err != nil {
			rec.Error = err.Error()
		}
		_ = cfg.toolAudit.Record(rec)
	}

	switch {
	case req.ToolName == "":
		info.Status = "failed"
		err := fmt.Errorf("child run tool name is required")
		recordAudit(info.Status, nil, err)
		emitChildToolEvent(ctx, events, info, err.Error())
		return nil, info, err
	case BlockedTools[req.ToolName]:
		info.Status = "blocked"
		err := fmt.Errorf("blocked tool for child run: %s", req.ToolName)
		recordAudit(info.Status, nil, err)
		emitChildToolEvent(ctx, events, info, err.Error())
		return nil, info, err
	case !toolAllowlisted(cfg.EnabledTools, req.ToolName):
		info.Status = "blocked"
		err := fmt.Errorf("tool not allowlisted for child run: %s", req.ToolName)
		recordAudit(info.Status, nil, err)
		emitChildToolEvent(ctx, events, info, err.Error())
		return nil, info, err
	case cfg.toolExecutor == nil:
		info.Status = "failed"
		err := fmt.Errorf("no tool executor configured for child run")
		recordAudit(info.Status, nil, err)
		emitChildToolEvent(ctx, events, info, err.Error())
		return nil, info, err
	}

	ch, err := cfg.toolExecutor.Execute(ctx, req)
	if err != nil {
		info.Status = "failed"
		recordAudit(info.Status, nil, err)
		emitChildToolEvent(ctx, events, info, err.Error())
		return nil, info, err
	}

	var output json.RawMessage
	for ev := range ch {
		switch ev.Type {
		case "output":
			output = append(json.RawMessage(nil), ev.Output...)
		case "failed":
			info.Status = "failed"
			msg := ""
			if ev.Err != nil {
				msg = ev.Err.Error()
				err = ev.Err
			} else {
				msg = "child tool execution failed"
				err = errors.New(msg)
			}
			recordAudit(info.Status, nil, err)
			emitChildToolEvent(ctx, events, info, msg)
			return nil, info, err
		}
	}

	info.Status = "completed"
	info.ResultSize = len(output)
	recordAudit(info.Status, output, nil)
	emitChildToolEvent(ctx, events, info, "")
	return output, info, nil
}

func emitChildToolEvent(ctx context.Context, events chan<- SubagentEvent, info ToolCallInfo, message string) {
	if events == nil {
		return
	}
	infoCopy := info
	select {
	case events <- SubagentEvent{Type: EventToolCall, Message: message, ToolCall: &infoCopy}:
	case <-ctx.Done():
	}
}
