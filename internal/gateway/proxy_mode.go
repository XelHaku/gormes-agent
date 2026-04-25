package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

const (
	proxyStateRunning  = "running"
	proxyStateDegraded = "degraded"
)

// ErrProxyBusy is returned when proxy mode is asked to start a second turn
// before the current remote stream has reached a terminal frame.
var ErrProxyBusy = errors.New("gateway proxy: turn already active")

// ProxySubmitterConfig wires gateway proxy mode to an OpenAI-compatible
// Gormes API server.
type ProxySubmitterConfig struct {
	BaseURL       string
	APIKey        string
	Model         string
	History       []hermes.Message
	Client        hermes.Client
	RuntimeStatus RuntimeStatusWriter
}

// ProxySubmitter satisfies the gateway manager's kernel submitter contract by
// forwarding each turn to a remote /v1/chat/completions stream.
type ProxySubmitter struct {
	baseURL string
	apiKey  string
	model   string
	client  hermes.Client
	status  RuntimeStatusWriter
	frames  chan kernel.RenderFrame

	mu         sync.Mutex
	history    []hermes.Message
	active     bool
	generation uint64
}

// NewProxySubmitter constructs a proxy-mode submitter. The default client uses
// the same HTTP+SSE implementation as the native kernel.
func NewProxySubmitter(cfg ProxySubmitterConfig) (*ProxySubmitter, error) {
	baseURL := normalizeProxyBaseURL(cfg.BaseURL)
	if baseURL == "" && cfg.Client == nil {
		return nil, errors.New("gateway proxy: base URL is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gormes-agent"
	}
	client := cfg.Client
	if client == nil {
		client = hermes.NewHTTPClient(baseURL, strings.TrimSpace(cfg.APIKey))
	}
	return &ProxySubmitter{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		model:   model,
		client:  client,
		status:  cfg.RuntimeStatus,
		frames:  make(chan kernel.RenderFrame, 16),
		history: append([]hermes.Message(nil), cfg.History...),
	}, nil
}

func normalizeProxyBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// Submit starts a remote proxy turn. It returns after the stream goroutine is
// admitted, matching kernel.Submit's non-blocking manager contract.
func (p *ProxySubmitter) Submit(ev kernel.PlatformEvent) error {
	if p == nil {
		return nil
	}
	switch ev.Kind {
	case kernel.PlatformEventSubmit:
		p.mu.Lock()
		if p.active {
			p.mu.Unlock()
			return ErrProxyBusy
		}
		p.active = true
		p.generation++
		generation := p.generation
		history := append([]hermes.Message(nil), p.history...)
		p.mu.Unlock()

		go p.runTurn(context.Background(), generation, ev, history)
		return nil
	case kernel.PlatformEventCancel:
		p.mu.Lock()
		p.generation++
		p.mu.Unlock()
		return nil
	default:
		return nil
	}
}

// ResetSession clears local proxy history and invalidates any in-flight remote
// stream so stale output cannot become the current gateway reply.
func (p *ProxySubmitter) ResetSession() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	p.history = nil
	p.generation++
	p.mu.Unlock()
	return nil
}

// Render returns proxy-mode render frames for the manager outbound loop.
func (p *ProxySubmitter) Render() <-chan kernel.RenderFrame {
	if p == nil {
		return nil
	}
	return p.frames
}

func (p *ProxySubmitter) runTurn(ctx context.Context, generation uint64, ev kernel.PlatformEvent, history []hermes.Message) {
	userMessage := hermes.Message{Role: "user", Content: ev.Text}
	safeHistory := safeProxyHistory(history)
	historyWithUser := append(append([]hermes.Message(nil), safeHistory...), userMessage)

	p.emitIfCurrent(generation, kernel.RenderFrame{
		Phase:     kernel.PhaseConnecting,
		History:   historyWithUser,
		SessionID: ev.SessionID,
		Model:     p.model,
	})

	messages := make([]hermes.Message, 0, len(safeHistory)+2)
	if contextPrompt := strings.TrimSpace(ev.SessionContext); contextPrompt != "" {
		messages = append(messages, hermes.Message{Role: "system", Content: ev.SessionContext})
	}
	messages = append(messages, safeHistory...)
	if strings.TrimSpace(ev.Text) != "" {
		messages = append(messages, userMessage)
	}

	stream, err := p.client.OpenStream(ctx, hermes.ChatRequest{
		Model:     p.model,
		SessionID: ev.SessionID,
		Stream:    true,
		Messages:  messages,
	})
	if err != nil {
		p.finishProxyError(generation, ev.SessionID, historyWithUser, err)
		return
	}
	defer stream.Close()

	sessionID := ev.SessionID
	if remoteSessionID := strings.TrimSpace(stream.SessionID()); remoteSessionID != "" {
		sessionID = remoteSessionID
	}

	var draft strings.Builder
	for {
		if !p.isCurrent(generation) {
			p.finishStale(generation, sessionID)
			return
		}
		event, err := stream.Recv(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			p.finishProxyError(generation, sessionID, historyWithUser, err)
			return
		}
		if event.Kind != hermes.EventToken || event.Token == "" {
			continue
		}
		draft.WriteString(event.Token)
		p.emitIfCurrent(generation, kernel.RenderFrame{
			Phase:     kernel.PhaseStreaming,
			DraftText: draft.String(),
			History:   historyWithUser,
			SessionID: sessionID,
			Model:     p.model,
		})
	}

	if !p.isCurrent(generation) {
		p.finishStale(generation, sessionID)
		return
	}
	finalHistory := append(historyWithUser, hermes.Message{Role: "assistant", Content: draft.String()})
	p.mu.Lock()
	p.history = append([]hermes.Message(nil), finalHistory...)
	p.active = false
	p.mu.Unlock()

	p.writeProxyStatus(proxyStateRunning, "")
	p.frames <- kernel.RenderFrame{
		Phase:     kernel.PhaseIdle,
		History:   finalHistory,
		SessionID: sessionID,
		Model:     p.model,
	}
}

func safeProxyHistory(history []hermes.Message) []hermes.Message {
	out := make([]hermes.Message, 0, len(history))
	for _, msg := range history {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "system", "user", "assistant":
		default:
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		out = append(out, hermes.Message{Role: role, Content: msg.Content})
	}
	return out
}

func (p *ProxySubmitter) finishProxyError(generation uint64, sessionID string, history []hermes.Message, err error) {
	message := p.degradedErrorMessage(err)
	p.mu.Lock()
	if p.generation == generation {
		p.history = append([]hermes.Message(nil), history...)
	}
	p.active = false
	p.mu.Unlock()

	p.writeProxyStatus(proxyStateDegraded, message)
	p.frames <- kernel.RenderFrame{
		Phase:     kernel.PhaseFailed,
		History:   history,
		SessionID: sessionID,
		Model:     p.model,
		LastError: message,
	}
}

func (p *ProxySubmitter) degradedErrorMessage(err error) string {
	classification := hermes.ClassifyProviderError(err)
	if classification.Kind == hermes.ProviderErrorAuth && strings.TrimSpace(p.apiKey) == "" {
		return "missing proxy credentials: remote API rejected the proxy request"
	}
	if classification.Status > 0 {
		return fmt.Sprintf("proxy remote error (%d): %s", classification.Status, err.Error())
	}
	return "proxy unreachable: " + err.Error()
}

func (p *ProxySubmitter) finishStale(generation uint64, sessionID string) {
	message := fmt.Sprintf("stale generation: ignored proxy response for generation %d", generation)
	p.mu.Lock()
	p.active = false
	history := append([]hermes.Message(nil), p.history...)
	p.mu.Unlock()

	p.writeProxyStatus(proxyStateDegraded, message)
	p.frames <- kernel.RenderFrame{
		Phase:     kernel.PhaseFailed,
		History:   history,
		SessionID: sessionID,
		Model:     p.model,
		LastError: message,
	}
}

func (p *ProxySubmitter) isCurrent(generation uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generation == generation
}

func (p *ProxySubmitter) emitIfCurrent(generation uint64, frame kernel.RenderFrame) {
	if !p.isCurrent(generation) {
		return
	}
	p.frames <- frame
}

func (p *ProxySubmitter) writeProxyStatus(state, message string) {
	if p.status == nil {
		return
	}
	_ = p.status.UpdateRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		ProxyState:        state,
		ProxyURL:          p.baseURL,
		ProxyErrorMessage: message,
	})
}
