package tuigateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

// Wire-kind strings. Using names instead of the kernel's iota values is a
// wire-compatibility hedge: if a future PlatformEventKind is inserted before
// an existing entry, the numeric value shifts but the wire name does not.
const (
	wireKindSubmit = "submit"
	wireKindCancel = "cancel"
	wireKindReset  = "reset"
)

// maxEventBody bounds the POST body read by the handler. PlatformEvent is a
// small envelope (kind, text, session metadata); 256 KiB leaves generous
// headroom for multi-line user turns without making the handler a DoS
// vector. The kernel's own admission pass still enforces per-field limits
// once the event reaches it.
const maxEventBody = 256 * 1024

// PlatformEventSink is the callback the handler invokes for each decoded
// upstream event. Production wiring (`cmd/gormes/main.go`) passes
// `k.Submit`; tests pass an in-memory spy.
type PlatformEventSink func(kernel.PlatformEvent) error

// wireEvent is the JSON envelope used by PostPlatformEvent and
// NewEventHandler. PlatformEventKind is serialised as a string so iota
// reordering in the kernel cannot silently re-route events.
type wireEvent struct {
	Kind           string `json:"kind"`
	Text           string `json:"text,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	SessionContext string `json:"session_context,omitempty"`
	CronJobID      string `json:"cron_job_id,omitempty"`
}

// NewEventHandler returns an http.Handler that accepts POSTed JSON
// wireEvent bodies and forwards them to sink as kernel.PlatformEvents. It
// is the upstream counterpart of NewSSEHandler: together they form the
// bidirectional surface a remote Bubble Tea client needs to drive the
// kernel.
//
// Contract:
//   - POST only; other verbs return 405.
//   - Body must be a JSON wireEvent with a known `kind`; malformed or
//     unknown-kind requests return 400 without invoking sink.
//   - Sink errors (e.g. kernel.ErrEventMailboxFull) surface as 502 so the
//     client can back off and retry rather than silently desyncing.
//   - On success the handler replies 202 Accepted — the sink call is
//     synchronous, but "accepted" captures the fact that the event has
//     been enqueued on the kernel mailbox rather than fully processed.
//
// The handler deliberately does NOT mux on session_id; like NewSSEHandler,
// per-session fanout is explicit follow-on scope.
func NewEventHandler(sink PlatformEventSink) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEventBody))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		var we wireEvent
		if err := json.Unmarshal(body, &we); err != nil {
			http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
			return
		}
		kind, err := wireKindToPlatformEventKind(we.Kind)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		evt := kernel.PlatformEvent{
			Kind:           kind,
			Text:           we.Text,
			SessionID:      we.SessionID,
			SessionContext: we.SessionContext,
			CronJobID:      we.CronJobID,
		}
		if err := sink(evt); err != nil {
			// 502 Bad Gateway: we reached this process but the kernel
			// mailbox rejected the enqueue. Retryable from the client.
			http.Error(w, "sink: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

// PostPlatformEvent is the client-side counterpart of NewEventHandler. It
// marshals evt to a wireEvent and POSTs it to url, returning an error if
// the network call fails or the server replies with a non-2xx status.
//
// PlatformEventQuit is rejected locally: the documented disconnect signal
// is the SSE request-context cancel, not an upstream "quit" event.
func PostPlatformEvent(ctx context.Context, url string, evt kernel.PlatformEvent) error {
	wk, err := platformEventKindToWireKind(evt.Kind)
	if err != nil {
		return err
	}
	body, err := json.Marshal(wireEvent{
		Kind:           wk,
		Text:           evt.Text,
		SessionID:      evt.SessionID,
		SessionContext: evt.SessionContext,
		CronJobID:      evt.CronJobID,
	})
	if err != nil {
		return fmt.Errorf("tuigateway: encode event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain a short preview of the server's error message; enough for
		// diagnostics, bounded so a misbehaving server can't blow memory.
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("tuigateway: POST %s: %s: %s", url, resp.Status, bytes.TrimSpace(preview))
	}
	return nil
}

func wireKindToPlatformEventKind(s string) (kernel.PlatformEventKind, error) {
	switch s {
	case wireKindSubmit:
		return kernel.PlatformEventSubmit, nil
	case wireKindCancel:
		return kernel.PlatformEventCancel, nil
	case wireKindReset:
		return kernel.PlatformEventResetSession, nil
	default:
		return 0, fmt.Errorf("tuigateway: unknown event kind %q", s)
	}
}

func platformEventKindToWireKind(k kernel.PlatformEventKind) (string, error) {
	switch k {
	case kernel.PlatformEventSubmit:
		return wireKindSubmit, nil
	case kernel.PlatformEventCancel:
		return wireKindCancel, nil
	case kernel.PlatformEventResetSession:
		return wireKindReset, nil
	case kernel.PlatformEventQuit:
		return "", fmt.Errorf("tuigateway: quit is not a wire-transmittable event; rely on SSE context cancel")
	default:
		return "", fmt.Errorf("tuigateway: unknown PlatformEventKind %d", int(k))
	}
}
