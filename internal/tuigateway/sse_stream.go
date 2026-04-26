package tuigateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

// Default backoff envelope for SSE reconnects. The remote TUI reconnects
// fast on transient drops (the Python tui_gateway does the same in ws.py
// via best-effort retry) but caps the wait so flapping gateways do not
// keep the consumer awake.
const (
	defaultReconnectInitial = 100 * time.Millisecond
	defaultReconnectMax     = 5 * time.Second
)

// RemoteClient consumes SSE-streamed kernel.RenderFrames from a remote
// Gormes gateway and posts platform events back over plain HTTP. It is
// the "transport client" half of the remote TUI surface: callers wire its
// Frames() channel into the Bubble Tea model and its Submit/Cancel/Resize
// helpers into the model's Submitter/Canceller closures.
//
// The struct is safe to share across goroutines — the SSE consumer runs
// on a dedicated worker started by DialSSE; HTTP POST helpers (Submit,
// Cancel, Resize, PostPlatformEvent) are independently safe to call
// concurrently because each one constructs its own request.
type RemoteClient struct {
	httpClient *http.Client
	baseURL    string
	sessionID  string

	frames chan kernel.RenderFrame
	errors chan error

	reconnectInitial time.Duration
	reconnectMax     time.Duration

	reconnectCount atomic.Uint64
	closed         atomic.Bool
	cancel         context.CancelFunc

	mu sync.Mutex // guards lifecycle ops
}

// DialOption tunes a RemoteClient before DialSSE opens the stream.
// Options are evaluated in order; later options override earlier ones for
// the same field. The package keeps the option set deliberately small so
// upstream parity discussions stay focused on the wire contract.
type DialOption func(*RemoteClient)

// WithHTTPClient swaps the http.Client used for the SSE GET and the
// outbound POST helpers. Tests pass a client backed by an httptest
// server; production wiring uses http.DefaultClient.
func WithHTTPClient(c *http.Client) DialOption {
	return func(rc *RemoteClient) {
		if c != nil {
			rc.httpClient = c
		}
	}
}

// WithSessionID sets the session id RemoteClient embeds in every outbound
// platform event. The remote gateway uses it to route the dispatch back
// to the correct kernel turn. Empty leaves the field blank, matching the
// "no resident session" invariant.
func WithSessionID(sid string) DialOption {
	return func(rc *RemoteClient) { rc.sessionID = sid }
}

// WithReconnectBackoff overrides the exponential reconnect envelope.
// initial is the first wait after a transport drop; the wait doubles up
// to max. Tests pin both to small values so a forced reconnect lands
// inside the test deadline.
func WithReconnectBackoff(initial, max time.Duration) DialOption {
	return func(rc *RemoteClient) {
		if initial > 0 {
			rc.reconnectInitial = initial
		}
		if max > 0 && max >= initial {
			rc.reconnectMax = max
		}
	}
}

// NewRemoteClient constructs a RemoteClient bound to baseURL with no
// in-flight SSE connection. Callers either call DialSSE to open the
// events stream (the typical path) or use the POST helpers directly when
// they only need to send a one-shot platform event without consuming
// frames. The returned client is otherwise quiescent.
func NewRemoteClient(baseURL string, opts ...DialOption) *RemoteClient {
	rc := &RemoteClient{
		httpClient:       http.DefaultClient,
		baseURL:          strings.TrimRight(baseURL, "/"),
		frames:           make(chan kernel.RenderFrame, 8),
		errors:           make(chan error, 4),
		reconnectInitial: defaultReconnectInitial,
		reconnectMax:     defaultReconnectMax,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(rc)
		}
	}
	return rc
}

// DialSSE opens the GET /events SSE stream and returns a RemoteClient
// pumping kernel.RenderFrames onto Frames(). The first connection must
// succeed: a 4xx/5xx or transport error surfaces immediately so callers
// can surface "remote streaming unavailable" without spinning. Later
// reconnects happen on a worker goroutine and never block the caller.
//
// The returned client owns a derived context cancelled by Close. The
// Frames channel closes when the run loop exits (either via Close or
// when ctx is cancelled).
func DialSSE(ctx context.Context, baseURL string, opts ...DialOption) (*RemoteClient, error) {
	rc := NewRemoteClient(baseURL, opts...)
	runCtx, cancel := context.WithCancel(ctx)
	rc.cancel = cancel

	resp, err := rc.openStream(runCtx)
	if err != nil {
		cancel()
		return nil, err
	}

	go rc.run(runCtx, resp)
	return rc, nil
}

// Frames returns the receive-side of the kernel render-frame stream. The
// channel is closed when the run loop exits.
func (rc *RemoteClient) Frames() <-chan kernel.RenderFrame { return rc.frames }

// Errors returns transport-error events the consumer can drain for
// observability. The channel is buffered; older errors are dropped if
// the buffer fills, since the Frames stream is the authoritative signal.
func (rc *RemoteClient) Errors() <-chan error { return rc.errors }

// Reconnects reports the number of successful reconnect attempts since
// DialSSE. Useful for tests asserting the reconnect path actually fired.
func (rc *RemoteClient) Reconnects() uint64 { return rc.reconnectCount.Load() }

// Close signals the run loop to stop and releases the embedded context.
// Frames closes shortly after. Idempotent: safe to call multiple times.
func (rc *RemoteClient) Close() {
	if !rc.closed.CompareAndSwap(false, true) {
		return
	}
	rc.mu.Lock()
	cancel := rc.cancel
	rc.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// PostPlatformEvent serialises evt as JSON and POSTs it to the gateway's
// generic /platform-event endpoint. The gateway dispatches on the kind
// discriminator, mirroring the upstream JSON-RPC method tag without the
// envelope. Returns the underlying transport error, if any, and any
// non-2xx response promoted to a typed error.
func (rc *RemoteClient) PostPlatformEvent(ctx context.Context, evt platformEvent) error {
	return rc.postJSON(ctx, "/platform-event", evt)
}

// Submit posts a SubmitEvent to /submit using the resident session id.
// Equivalent to PostPlatformEvent on a SubmitEvent, but routed through
// the dedicated endpoint so the GatewayMux can dispatch with simple
// path matchers instead of a kind switch on every request.
func (rc *RemoteClient) Submit(ctx context.Context, text string) error {
	return rc.postJSON(ctx, "/submit", SubmitEvent{
		Kind:      PlatformEventKindSubmit,
		SessionID: rc.sessionID,
		Text:      text,
	})
}

// Cancel posts a CancelEvent to /cancel using the resident session id.
func (rc *RemoteClient) Cancel(ctx context.Context) error {
	return rc.postJSON(ctx, "/cancel", CancelEvent{
		Kind:      PlatformEventKindCancel,
		SessionID: rc.sessionID,
	})
}

// Resize posts a ResizeEvent to /resize using the resident session id
// and the supplied terminal column count.
func (rc *RemoteClient) Resize(ctx context.Context, cols int) error {
	return rc.postJSON(ctx, "/resize", ResizeEvent{
		Kind:      PlatformEventKindResize,
		SessionID: rc.sessionID,
		Cols:      cols,
	})
}

func (rc *RemoteClient) postJSON(ctx context.Context, path string, body any) error {
	endpoint, err := url.JoinPath(rc.baseURL, path)
	if err != nil {
		return fmt.Errorf("tuigateway: build %s URL: %w", path, err)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("tuigateway: marshal %s body: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("tuigateway: build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("tuigateway: POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("tuigateway: POST %s: HTTP %d", path, resp.StatusCode)
	}
	return nil
}

func (rc *RemoteClient) openStream(ctx context.Context) (*http.Response, error) {
	endpoint, err := url.JoinPath(rc.baseURL, "/events")
	if err != nil {
		return nil, fmt.Errorf("tuigateway: build /events URL: %w", err)
	}
	if rc.sessionID != "" {
		endpoint += "?session_id=" + url.QueryEscape(rc.sessionID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("tuigateway: build /events request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tuigateway: dial /events: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("tuigateway: /events returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// run is the long-lived worker that drains an SSE response body and
// reconnects on transport errors until ctx is cancelled. Closes the
// frames channel on exit so consumers never block indefinitely.
func (rc *RemoteClient) run(ctx context.Context, initial *http.Response) {
	defer close(rc.frames)

	resp := initial
	backoff := rc.reconnectInitial
	for {
		if resp == nil {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			r, err := rc.openStream(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				rc.publishError(err)
				backoff *= 2
				if backoff > rc.reconnectMax {
					backoff = rc.reconnectMax
				}
				continue
			}
			resp = r
			rc.reconnectCount.Add(1)
			backoff = rc.reconnectInitial
		}

		rc.consume(ctx, resp.Body)
		_ = resp.Body.Close()
		resp = nil

		if ctx.Err() != nil {
			return
		}
	}
}

// consume reads the SSE body until EOF or ctx cancellation, decoding
// frame events into kernel.RenderFrames. The local parser mirrors the
// minimal subset already used in internal/hermes/sse.go: lines beginning
// with "event: " set the discriminator, "data: " accumulates payload,
// blank lines flush.
func (rc *RemoteClient) consume(ctx context.Context, body io.Reader) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	var event, data string
	flush := func() {
		if event == "frame" && data != "" {
			var f kernel.RenderFrame
			if err := json.Unmarshal([]byte(data), &f); err == nil {
				select {
				case rc.frames <- f:
				case <-ctx.Done():
				}
			}
		}
		event, data = "", ""
	}
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, ":"):
			// SSE comment / keepalive — ignore.
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if data != "" {
				data += "\n"
			}
			data += strings.TrimPrefix(line, "data: ")
		}
	}
	// EOF without a trailing blank line: flush whatever we have.
	if event != "" || data != "" {
		flush()
	}
}

func (rc *RemoteClient) publishError(err error) {
	select {
	case rc.errors <- err:
	default:
		// Drain one and try again so the caller sees the freshest error.
		select {
		case <-rc.errors:
		default:
		}
		select {
		case rc.errors <- err:
		default:
		}
	}
}
