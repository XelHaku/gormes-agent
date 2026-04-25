package apiserver

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

type kernelSubmitter interface {
	Submit(kernel.PlatformEvent) error
	Render() <-chan kernel.RenderFrame
}

// KernelTurnLoop adapts the native single-owner kernel loop to the HTTP
// chat-completions surface. It serializes API turns because the current kernel
// owns exactly one active turn at a time.
type KernelTurnLoop struct {
	mu             sync.Mutex
	kernel         kernelSubmitter
	frames         <-chan kernel.RenderFrame
	lastSeq        uint64
	lastHistoryLen int
}

// NewKernelTurnLoop returns a TurnLoop backed by a running kernel.Kernel. The
// caller is responsible for starting k.Run(ctx).
func NewKernelTurnLoop(k kernelSubmitter) *KernelTurnLoop {
	var frames <-chan kernel.RenderFrame
	if k != nil {
		frames = k.Render()
	}
	return &KernelTurnLoop{kernel: k, frames: frames}
}

func (l *KernelTurnLoop) RunTurn(ctx context.Context, req TurnRequest) (TurnResult, error) {
	return l.run(ctx, req, nil)
}

func (l *KernelTurnLoop) StreamTurn(ctx context.Context, req TurnRequest, cb StreamCallbacks) (TurnResult, error) {
	return l.run(ctx, req, cb.OnToken)
}

func (l *KernelTurnLoop) run(ctx context.Context, req TurnRequest, onToken func(string) error) (TurnResult, error) {
	if l == nil || l.kernel == nil || l.frames == nil {
		return TurnResult{}, errors.New("kernel turn loop is not configured")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.drainPendingFrames()
	startSeq := l.lastSeq
	startHistoryLen := l.lastHistoryLen
	if err := l.kernel.Submit(kernel.PlatformEvent{
		Kind:           kernel.PlatformEventSubmit,
		Text:           req.UserMessage,
		SessionID:      req.SessionID,
		SessionContext: buildKernelSessionContext(req),
	}); err != nil {
		return TurnResult{}, err
	}

	streamedDraft := ""
	for {
		select {
		case <-ctx.Done():
			return TurnResult{}, ctx.Err()
		case f, ok := <-l.frames:
			if !ok {
				return TurnResult{}, errors.New("kernel render stream closed")
			}
			l.rememberFrame(f)
			if f.Seq <= startSeq {
				continue
			}

			if onToken != nil && (f.Phase == kernel.PhaseStreaming || f.Phase == kernel.PhaseFinalizing || f.Phase == kernel.PhaseReconnecting) {
				if delta := draftDelta(streamedDraft, f.DraftText); delta != "" {
					if err := onToken(delta); err != nil {
						return TurnResult{}, err
					}
					streamedDraft = f.DraftText
				}
			}

			if f.Phase == kernel.PhaseFailed || f.Phase == kernel.PhaseCancelling {
				if f.LastError != "" {
					return TurnResult{}, errors.New(f.LastError)
				}
				return TurnResult{}, errors.New(strings.ToLower(f.Phase.String()))
			}
			if f.LastError != "" && f.Phase == kernel.PhaseIdle {
				return TurnResult{}, errors.New(f.LastError)
			}
			if f.Phase == kernel.PhaseIdle && len(f.History) > startHistoryLen {
				return resultFromFrame(f, req, startHistoryLen), nil
			}
		}
	}
}

func (l *KernelTurnLoop) drainPendingFrames() {
	for {
		select {
		case f, ok := <-l.frames:
			if !ok {
				return
			}
			l.rememberFrame(f)
		default:
			return
		}
	}
}

func (l *KernelTurnLoop) rememberFrame(f kernel.RenderFrame) {
	if f.Seq > l.lastSeq {
		l.lastSeq = f.Seq
	}
	l.lastHistoryLen = len(f.History)
}

func resultFromFrame(f kernel.RenderFrame, req TurnRequest, startHistoryLen int) TurnResult {
	content := f.DraftText
	for i := len(f.History) - 1; i >= 0; i-- {
		if f.History[i].Role == "assistant" {
			content = f.History[i].Content
			break
		}
	}
	sessionID := f.SessionID
	if sessionID == "" {
		sessionID = req.SessionID
	}
	prompt := f.Telemetry.TokensInTotal
	completion := f.Telemetry.TokensOutTotal
	return TurnResult{
		Content:      content,
		SessionID:    sessionID,
		FinishReason: "stop",
		Messages:     newChatMessagesFromFrame(f, req, startHistoryLen),
		Usage: Usage{
			PromptTokens:     prompt,
			CompletionTokens: completion,
			TotalTokens:      prompt + completion,
		},
	}
}

func newChatMessagesFromFrame(f kernel.RenderFrame, req TurnRequest, startHistoryLen int) []ChatMessage {
	if startHistoryLen < 0 || startHistoryLen > len(f.History) {
		startHistoryLen = 0
	}
	messages := make([]ChatMessage, 0, len(f.History)-startHistoryLen)
	for _, msg := range f.History[startHistoryLen:] {
		converted := ChatMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			Name:       msg.Name,
		}
		for _, call := range msg.ToolCalls {
			converted.ToolCalls = append(converted.ToolCalls, ToolCall{
				ID:        call.ID,
				Name:      call.Name,
				Arguments: string(call.Arguments),
			})
		}
		messages = append(messages, converted)
	}
	if len(messages) > 0 && messages[0].Role == "user" && messages[0].Content == req.UserMessage {
		messages = messages[1:]
	}
	return messages
}

func draftDelta(previous, next string) string {
	if next == "" || next == previous {
		return ""
	}
	if strings.HasPrefix(next, previous) {
		return strings.TrimPrefix(next, previous)
	}
	return next
}

func buildKernelSessionContext(req TurnRequest) string {
	var blocks []string
	if strings.TrimSpace(req.SystemPrompt) != "" {
		blocks = append(blocks, req.SystemPrompt)
	}
	if len(req.History) > 0 {
		lines := []string{"## Client Conversation History"}
		for _, msg := range req.History {
			role := strings.TrimSpace(msg.Role)
			content := strings.TrimSpace(msg.Content)
			if role == "" || (content == "" && len(msg.ToolCalls) == 0 && msg.ToolCallID == "") {
				continue
			}
			line := role + ": " + content
			for _, call := range msg.ToolCalls {
				line += "\n" + "assistant tool_call " + call.ID + " " + call.Name + ": " + call.Arguments
			}
			if msg.ToolCallID != "" {
				line += "\n" + "tool_result " + msg.ToolCallID + " " + msg.Name + ": " + content
			}
			lines = append(lines, line)
		}
		if len(lines) > 1 {
			blocks = append(blocks, strings.Join(lines, "\n"))
		}
	}
	return strings.Join(blocks, "\n\n")
}
