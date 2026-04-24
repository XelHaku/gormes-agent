// Package tuigateway is the Phase 5.Q seed for the remote TUI gateway: an
// HTTP server-sent-events surface that streams kernel.RenderFrame snapshots
// to a Bubble Tea client running on a different host. The wire format is
// intentionally narrow — `event: frame` for each frame, JSON-encoded body,
// terminated by `event: end` when the kernel's render channel closes —
// so future iterations (auth, multi-session multiplexing, slash-command
// upstream) can layer on without renegotiating the streaming spine.
package tuigateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

// SSE event names. Kept as exported constants so wire-protocol changes are
// audit-friendly and downstream tests can reference them by symbol.
const (
	EventFrame = "frame"
	EventEnd   = "end"
)

// NewSSEHandler returns an http.Handler that pumps kernel render frames
// to the client as Server-Sent Events. The handler owns neither the input
// channel nor the kernel — closing `frames` is the kernel's job, and the
// handler simply terminates the stream when either side hangs up.
//
// Lifecycle:
//   - Each accepted request blocks on `frames` until the channel closes
//     or the request context is cancelled (client disconnect).
//   - On `frames` close the handler emits a final `event: end` so the
//     consumer can distinguish a clean kernel shutdown from a TCP reset.
//   - On context cancel the handler returns immediately; the response is
//     simply truncated (the consumer treats EOF as stream end).
//
// The handler is safe to install behind any router; it does NOT mux on
// session_id yet — that is a future iteration once the gateway grows
// per-chat fanout. For now one HTTP endpoint = one render stream.
func NewSSEHandler(frames <-chan kernel.RenderFrame) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// Disable buffering for proxies that honour the hint (nginx in
		// particular). Browsers and the Go consumer ignore it.
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		if flusher != nil {
			flusher.Flush()
		}

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case f, ok := <-frames:
				if !ok {
					writeEvent(w, EventEnd, []byte("{}"))
					if flusher != nil {
						flusher.Flush()
					}
					return
				}
				body, err := json.Marshal(f)
				if err != nil {
					// Encoding a RenderFrame should be infallible (no
					// channels / funcs in the type) — but fail loudly
					// rather than silently dropping a frame.
					http.Error(w, "encode frame: "+err.Error(), http.StatusInternalServerError)
					return
				}
				writeEvent(w, EventFrame, body)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	})
}

// writeEvent emits a single SSE event. Errors are intentionally ignored:
// the next write/flush will surface a broken pipe via the Flusher, and
// the request context will be cancelled by net/http as soon as the
// client disconnects, which terminates the handler loop above.
func writeEvent(w io.Writer, event string, data []byte) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

// DialSSE opens a long-lived GET to url and returns a channel of decoded
// render frames. The channel closes when the server emits `event: end`,
// when the request body returns EOF, or when ctx is cancelled — whichever
// fires first. Network errors during the initial dial are returned
// synchronously; mid-stream errors close the channel without surfacing.
func DialSSE(ctx context.Context, url string) (<-chan kernel.RenderFrame, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("tuigateway: SSE handshake returned %s", resp.Status)
	}
	out := make(chan kernel.RenderFrame, 1)
	go func() {
		defer resp.Body.Close()
		consumeInto(ctx, resp.Body, out)
	}()
	return out, nil
}

// ConsumeSSE decodes an SSE stream from r into a channel of render frames.
// It is the parser primitive used by both DialSSE and the test suite —
// callers feeding hand-rolled streams (or alternative transports such as
// stdio bridges) can use it directly.
func ConsumeSSE(ctx context.Context, r io.Reader) <-chan kernel.RenderFrame {
	out := make(chan kernel.RenderFrame, 1)
	go consumeInto(ctx, r, out)
	return out
}

func consumeInto(ctx context.Context, r io.Reader, out chan<- kernel.RenderFrame) {
	defer close(out)

	sc := bufio.NewScanner(r)
	// Bound the per-line buffer at 1 MiB. RenderFrame.History grows over
	// the life of a session, so single frames can run several KB; 1 MiB
	// is generous without being a DoS vector.
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	var (
		event string
		data  strings.Builder
	)
	dispatch := func() bool {
		defer func() {
			event = ""
			data.Reset()
		}()
		switch event {
		case EventFrame:
			var f kernel.RenderFrame
			if err := json.Unmarshal([]byte(data.String()), &f); err != nil {
				return true // skip malformed frame, keep stream open
			}
			select {
			case <-ctx.Done():
				return false
			case out <- f:
			}
		case EventEnd:
			return false
		}
		return true
	}

	for sc.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := sc.Text()
		if line == "" {
			if event != "" || data.Len() > 0 {
				if !dispatch() {
					return
				}
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // SSE comment / keepalive
		}
		if rest, ok := strings.CutPrefix(line, "event: "); ok {
			event = rest
			continue
		}
		if rest, ok := strings.CutPrefix(line, "data: "); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(rest)
			continue
		}
	}
	// Flush trailing event (server may close without a blank line).
	if event != "" || data.Len() > 0 {
		dispatch()
	}
}
