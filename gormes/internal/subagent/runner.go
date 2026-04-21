package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

const (
	childSystemPrompt   = "You are a delegated Gormes subagent. Work only on the scoped goal. Return a concise final answer."
	defaultToolDuration = 30 * time.Second
)

type Runner interface {
	Run(ctx context.Context, spec Spec, emit func(Event)) (Result, error)
}

type ChatRunnerConfig struct {
	Model           string
	MaxToolDuration time.Duration
}

type ChatRunner struct {
	client hermes.Client
	reg    *tools.Registry
	cfg    ChatRunnerConfig
}

func NewChatRunner(client hermes.Client, reg *tools.Registry, cfg ChatRunnerConfig) *ChatRunner {
	if reg == nil {
		reg = tools.NewRegistry()
	}
	return &ChatRunner{client: client, reg: reg, cfg: cfg}
}

func (r *ChatRunner) Run(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	if emit != nil {
		emit(Event{Type: EventStarted, Message: spec.Goal})
	}

	var (
		sessionID string
		result    Result
		messages  = childMessages(spec)
	)

	for iteration := 1; iteration <= spec.MaxIterations; iteration++ {
		stream, err := r.client.OpenStream(ctx, hermes.ChatRequest{
			Model:     chooseModel(spec, r.cfg),
			Messages:  messages,
			SessionID: sessionID,
			Stream:    true,
			Tools:     descriptors(r.reg, spec.AllowedTools),
		})
		if err != nil {
			return r.failResult(ctx, result, fmt.Errorf("subagent: open stream: %w", err), emit)
		}

		sessionID = stream.SessionID()
		result.RunID = sessionID

		var (
			final       hermes.Event
			turnSummary strings.Builder
		)

		for {
			ev, err := stream.Recv(ctx)
			if err != nil {
				_ = stream.Close()
				if err == io.EOF {
					if turnSummary.Len() > 0 {
						result.Status = StatusCompleted
						result.Summary = strings.TrimSpace(turnSummary.String())
						if emit != nil {
							emit(Event{Type: EventCompleted, Message: result.Summary, Iteration: iteration})
						}
						return result, nil
					}
					return r.failResult(ctx, result, fmt.Errorf("subagent: stream ended without final event"), emit)
				}
				return r.failResult(ctx, result, fmt.Errorf("subagent: recv event: %w", err), emit)
			}

			switch ev.Kind {
			case hermes.EventToken:
				turnSummary.WriteString(ev.Token)
				if emit != nil {
					emit(Event{Type: EventProgress, Message: ev.Token, Iteration: iteration})
				}
			case hermes.EventDone:
				final = ev
				if err := stream.Close(); err != nil {
					return r.failResult(ctx, result, fmt.Errorf("subagent: close stream: %w", err), emit)
				}
				goto finished
			}
		}

	finished:
		result.FinishReason = final.FinishReason

		switch final.FinishReason {
		case "stop":
			result.Status = StatusCompleted
			result.Summary = strings.TrimSpace(turnSummary.String())
			if emit != nil {
				emit(Event{Type: EventCompleted, Message: result.Summary, Iteration: iteration})
			}
			return result, nil
		case "tool_calls":
			messages = append(messages, hermes.Message{
				Role:      "assistant",
				Content:   turnSummary.String(),
				ToolCalls: final.ToolCalls,
			})

			for _, call := range final.ToolCalls {
				if !contains(result.ToolCalls, call.Name) {
					result.ToolCalls = append(result.ToolCalls, call.Name)
				}
				if emit != nil {
					emit(Event{Type: EventToolCall, ToolName: call.Name, Iteration: iteration})
				}
				reply := r.execTool(ctx, spec, call)
				messages = append(messages, hermes.Message{
					Role:       "tool",
					Content:    string(reply),
					ToolCallID: call.ID,
					Name:       call.Name,
				})
			}
		default:
			return r.failResult(ctx, result, fmt.Errorf("subagent: unexpected finish reason %q", final.FinishReason), emit)
		}
	}

	return r.failResult(ctx, result, fmt.Errorf("subagent: max iterations exhausted"), emit)
}

func (r *ChatRunner) failResult(ctx context.Context, result Result, err error, emit func(Event)) (Result, error) {
	switch {
	case ctx.Err() == context.Canceled:
		result.Status = StatusCancelled
	case ctx.Err() == context.DeadlineExceeded:
		result.Status = StatusTimedOut
	default:
		result.Status = StatusFailed
	}
	result.Error = err.Error()
	if emit != nil {
		emit(Event{Type: EventFailed, Message: result.Error})
	}
	return result, err
}

func chooseModel(spec Spec, cfg ChatRunnerConfig) string {
	if strings.TrimSpace(spec.Model) != "" {
		return spec.Model
	}
	return cfg.Model
}

func childMessages(spec Spec) []hermes.Message {
	system := childSystemPrompt
	if strings.TrimSpace(spec.Context) != "" {
		system += "\n\nScoped context:\n" + strings.TrimSpace(spec.Context)
	}
	return []hermes.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: strings.TrimSpace(spec.Goal)},
	}
}

func descriptors(reg *tools.Registry, allowed []string) []hermes.ToolDescriptor {
	if reg == nil {
		return nil
	}
	toolDescs := reg.Descriptors()
	out := make([]hermes.ToolDescriptor, 0, len(toolDescs))
	for _, desc := range toolDescs {
		if IsBlockedTool(desc.Name) {
			continue
		}
		if len(allowed) > 0 && !contains(allowed, desc.Name) {
			continue
		}
		out = append(out, hermes.ToolDescriptor{
			Name:        desc.Name,
			Description: desc.Description,
			Schema:      desc.Schema,
		})
	}
	return out
}

func (r *ChatRunner) execTool(ctx context.Context, spec Spec, call hermes.ToolCall) json.RawMessage {
	if IsBlockedTool(call.Name) {
		return jsonError(fmt.Sprintf("tool %q is blocked by policy", call.Name))
	}
	if len(spec.AllowedTools) > 0 && !contains(spec.AllowedTools, call.Name) {
		return jsonError(fmt.Sprintf("tool %q is not in allowed_tools", call.Name))
	}

	tool, ok := r.reg.Get(call.Name)
	if !ok {
		return jsonError(fmt.Sprintf("tool %q is not registered", call.Name))
	}

	timeout := tool.Timeout()
	if timeout <= 0 {
		timeout = r.cfg.MaxToolDuration
	}
	if timeout <= 0 {
		timeout = defaultToolDuration
	}

	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := safeExecute(toolCtx, tool, call.Arguments)
	if err != nil {
		return jsonError(err.Error())
	}
	return out
}

func safeExecute(ctx context.Context, tool tools.Tool, args json.RawMessage) (result json.RawMessage, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool panicked: %v", r)
			result = nil
		}
	}()

	return tool.Execute(ctx, args)
}

func jsonError(message string) json.RawMessage {
	raw, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		return json.RawMessage(`{"error":"tool execution failed"}`)
	}
	return raw
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
