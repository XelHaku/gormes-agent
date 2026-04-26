package tuigateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

// KernelHandle is the dispatch contract the GatewayMux uses to deliver
// PlatformEvents into a kernel.Kernel (or an equivalent test fake).
// Keeping the surface narrow means the gateway transport never touches
// kernel internals — it only forwards events and reads render frames.
type KernelHandle interface {
	Submit(kernel.PlatformEvent) error
	Render() <-chan kernel.RenderFrame
}

// GatewayMux is the native Go HTTP front for the remote TUI: it streams
// kernel.RenderFrames as SSE on GET /events and accepts JSON-encoded
// platform events on the dedicated /submit, /cancel, /resize endpoints
// plus a unified /platform-event dispatcher for clients that prefer one
// envelope. The mux mirrors the upstream tui_gateway/server.py JSON-RPC
// methods (prompt.submit, session.interrupt, terminal.resize) but speaks
// plain HTTP+SSE so the consumer can be any Go process.
type GatewayMux struct {
	*http.ServeMux
	handle KernelHandle

	mu        sync.RWMutex
	lastResize map[string]int
}

// NewGatewayMux returns a multiplexer wired to the supplied kernel
// handle. The returned mux embeds an *http.ServeMux so callers can
// compose additional routes (health, metrics) onto the same listener.
func NewGatewayMux(handle KernelHandle) *GatewayMux {
	g := &GatewayMux{
		ServeMux:   http.NewServeMux(),
		handle:     handle,
		lastResize: make(map[string]int),
	}
	g.HandleFunc("GET /events", g.handleEvents)
	g.HandleFunc("POST /submit", g.handleSubmit)
	g.HandleFunc("POST /cancel", g.handleCancel)
	g.HandleFunc("POST /resize", g.handleResize)
	g.HandleFunc("POST /platform-event", g.handlePlatformEvent)
	return g
}

// LastResizeCols returns the most recent column count POSTed to /resize
// for the given session id. Tests use it to assert the resize handler
// recorded the value; production callers can also surface it for
// diagnostics. Returns 0 when no resize has been observed.
func (g *GatewayMux) LastResizeCols(sessionID string) int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lastResize[sessionID]
}

func (g *GatewayMux) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, -32603, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	frames := g.handle.Render()
	for {
		select {
		case <-r.Context().Done():
			return
		case f, ok := <-frames:
			if !ok {
				return
			}
			payload, err := json.Marshal(f)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: frame\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (g *GatewayMux) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var evt SubmitEvent
	if err := decodeJSONBody(r, &evt); err != nil {
		writeJSONError(w, http.StatusBadRequest, -32700, "parse error")
		return
	}
	if err := g.handle.Submit(kernel.PlatformEvent{
		Kind:      kernel.PlatformEventSubmit,
		SessionID: evt.SessionID,
		Text:      evt.Text,
	}); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, -32000, err.Error())
		return
	}
	writeJSONOK(w, map[string]any{"status": "queued"})
}

func (g *GatewayMux) handleCancel(w http.ResponseWriter, r *http.Request) {
	var evt CancelEvent
	if err := decodeJSONBody(r, &evt); err != nil {
		writeJSONError(w, http.StatusBadRequest, -32700, "parse error")
		return
	}
	if err := g.handle.Submit(kernel.PlatformEvent{
		Kind:      kernel.PlatformEventCancel,
		SessionID: evt.SessionID,
	}); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, -32000, err.Error())
		return
	}
	writeJSONOK(w, map[string]any{"status": "ok"})
}

func (g *GatewayMux) handleResize(w http.ResponseWriter, r *http.Request) {
	var evt ResizeEvent
	if err := decodeJSONBody(r, &evt); err != nil {
		writeJSONError(w, http.StatusBadRequest, -32700, "parse error")
		return
	}
	g.mu.Lock()
	g.lastResize[evt.SessionID] = evt.Cols
	g.mu.Unlock()
	writeJSONOK(w, map[string]any{"cols": evt.Cols})
}

// handlePlatformEvent dispatches the unified envelope. It first decodes
// just the kind discriminator, then re-decodes into the concrete struct
// so each variant goes through the same handler as its dedicated
// endpoint. Unknown kinds get a 400 with a JSON-RPC error envelope.
func (g *GatewayMux) handlePlatformEvent(w http.ResponseWriter, r *http.Request) {
	body, err := readBodyOnce(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, -32700, "parse error")
		return
	}
	var head struct {
		Kind PlatformEventKind `json:"kind"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		writeJSONError(w, http.StatusBadRequest, -32700, "parse error")
		return
	}
	if !ValidPlatformEventKind(head.Kind) {
		writeJSONError(w, http.StatusBadRequest, -32602, fmt.Sprintf("unknown platform event kind: %q", head.Kind))
		return
	}
	switch head.Kind {
	case PlatformEventKindSubmit:
		var evt SubmitEvent
		if err := json.Unmarshal(body, &evt); err != nil {
			writeJSONError(w, http.StatusBadRequest, -32700, "parse error")
			return
		}
		if err := g.handle.Submit(kernel.PlatformEvent{
			Kind:      kernel.PlatformEventSubmit,
			SessionID: evt.SessionID,
			Text:      evt.Text,
		}); err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, -32000, err.Error())
			return
		}
		writeJSONOK(w, map[string]any{"status": "queued"})
	case PlatformEventKindCancel:
		var evt CancelEvent
		if err := json.Unmarshal(body, &evt); err != nil {
			writeJSONError(w, http.StatusBadRequest, -32700, "parse error")
			return
		}
		if err := g.handle.Submit(kernel.PlatformEvent{
			Kind:      kernel.PlatformEventCancel,
			SessionID: evt.SessionID,
		}); err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, -32000, err.Error())
			return
		}
		writeJSONOK(w, map[string]any{"status": "ok"})
	case PlatformEventKindResize:
		var evt ResizeEvent
		if err := json.Unmarshal(body, &evt); err != nil {
			writeJSONError(w, http.StatusBadRequest, -32700, "parse error")
			return
		}
		g.mu.Lock()
		g.lastResize[evt.SessionID] = evt.Cols
		g.mu.Unlock()
		writeJSONOK(w, map[string]any{"cols": evt.Cols})
	default:
		// Progress and image_metadata are server→client emit-only events;
		// the mux accepts them as valid kinds but does not dispatch them
		// onto the kernel. Acknowledging keeps the wire compatible with
		// future relay use cases without leaking through to the kernel.
		writeJSONOK(w, map[string]any{"status": "ack"})
	}
}

func decodeJSONBody(r *http.Request, dst any) error {
	body, err := readBodyOnce(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func readBodyOnce(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	const maxBody = 1 << 20 // 1 MiB ceiling — generous for a control-plane envelope.
	limited := http.MaxBytesReader(nil, r.Body, maxBody)
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := limited.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			break
		}
	}
	if len(buf) == 0 {
		return nil, fmt.Errorf("empty body")
	}
	return buf, nil
}

func writeJSONOK(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    code,
		"message": msg,
	})
}
