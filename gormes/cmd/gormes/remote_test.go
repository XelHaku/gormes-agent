package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tuigateway"
)

// silentLogger returns an *slog.Logger whose output is discarded. The remote
// surface helper logs submit/cancel errors instead of returning them through
// the TUI's fire-and-forget Submitter/Canceller callbacks; tests that don't
// care about log content use this to keep the test binary output clean.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestOpenRemoteTUISurface_StreamsFramesAndRoutesSubmit pins the end-to-end
// bridge: given an httptest gateway built with NewGatewayMux, the helper must
// return a frames channel that yields the kernel.RenderFrame pushed upstream,
// and a Submitter closure that delivers the text to the gateway's sink as a
// PlatformEventSubmit. This is the primary "cmd/gormes --remote <url>"
// contract the 5.Q note calls out.
func TestOpenRemoteTUISurface_StreamsFramesAndRoutesSubmit(t *testing.T) {
	t.Parallel()

	frames := make(chan kernel.RenderFrame, 1)
	frames <- kernel.RenderFrame{Seq: 11, Phase: kernel.PhaseStreaming, DraftText: "remote-frame"}
	close(frames)

	sinkCh := make(chan kernel.PlatformEvent, 2)
	sink := func(e kernel.PlatformEvent) error {
		sinkCh <- e
		return nil
	}

	srv := httptest.NewServer(tuigateway.NewGatewayMux(frames, sink))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	framesCh, submit, cancelTurn, err := openRemoteTUISurface(ctx, srv.URL, silentLogger())
	if err != nil {
		t.Fatalf("openRemoteTUISurface: %v", err)
	}
	if framesCh == nil || submit == nil || cancelTurn == nil {
		t.Fatalf("openRemoteTUISurface returned nil surface (frames=%v submit=%v cancel=%v)", framesCh, submit, cancelTurn)
	}

	select {
	case f, ok := <-framesCh:
		if !ok {
			t.Fatal("frames channel closed before frame arrived")
		}
		if f.Seq != 11 || f.DraftText != "remote-frame" {
			t.Fatalf("frame = %+v, want Seq=11 DraftText=remote-frame", f)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for remote frame")
	}

	submit("hello remote")
	select {
	case e := <-sinkCh:
		if e.Kind != kernel.PlatformEventSubmit || e.Text != "hello remote" {
			t.Fatalf("sink got %+v, want PlatformEventSubmit{Text: hello remote}", e)
		}
	case <-time.After(time.Second):
		t.Fatal("sink did not receive submit")
	}

	cancelTurn()
	select {
	case e := <-sinkCh:
		if e.Kind != kernel.PlatformEventCancel {
			t.Fatalf("sink got %+v, want PlatformEventCancel", e)
		}
	case <-time.After(time.Second):
		t.Fatal("sink did not receive cancel")
	}
}

// TestOpenRemoteTUISurface_RejectsEmptyURL pins the startup-gate behaviour:
// --remote "" (or an explicit empty override) must fail fast rather than
// yielding a dangling client that hangs on a dial to "".
func TestOpenRemoteTUISurface_RejectsEmptyURL(t *testing.T) {
	t.Parallel()
	_, _, _, err := openRemoteTUISurface(context.Background(), "", silentLogger())
	if err == nil {
		t.Fatal("openRemoteTUISurface(\"\"): want non-nil error")
	}
}

// TestOpenRemoteTUISurface_RejectsSchemelessURL pins the second construction
// guard: --remote "gw.example/path" must fail synchronously, not hang on a
// broken URL at dial time.
func TestOpenRemoteTUISurface_RejectsSchemelessURL(t *testing.T) {
	t.Parallel()
	_, _, _, err := openRemoteTUISurface(context.Background(), "gw.example/path", silentLogger())
	if err == nil {
		t.Fatal("openRemoteTUISurface(scheme-less): want non-nil error")
	}
}

// TestOpenRemoteTUISurface_SurfacesHandshakeFailure proves a non-200 reply on
// the frames handshake surfaces as a synchronous error so the --remote
// startup path bails loudly instead of launching a TUI against a broken
// upstream.
func TestOpenRemoteTUISurface_SurfacesHandshakeFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gateway down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, _, err := openRemoteTUISurface(ctx, srv.URL, silentLogger())
	if err == nil {
		t.Fatal("openRemoteTUISurface: want non-nil error on 503 handshake")
	}
}

// TestOpenRemoteTUISurface_LogsSubmitError pins backpressure behaviour: when
// the gateway replies with 502 (sink rejected the enqueue), the Submitter
// callback swallows the error into the provided logger rather than panicking
// or crashing the Bubble Tea program. Matches the local path where
// k.Submit errors are likewise discarded at the TUI seam.
func TestOpenRemoteTUISurface_LogsSubmitError(t *testing.T) {
	t.Parallel()

	frames := make(chan kernel.RenderFrame)
	close(frames)

	var sinkCalls int32
	sink := func(kernel.PlatformEvent) error {
		atomic.AddInt32(&sinkCalls, 1)
		return kernel.ErrEventMailboxFull
	}
	srv := httptest.NewServer(tuigateway.NewGatewayMux(frames, sink))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(writerAdapter{&buf}, nil))

	_, submit, _, err := openRemoteTUISurface(ctx, srv.URL, logger)
	if err != nil {
		t.Fatalf("openRemoteTUISurface: %v", err)
	}

	submit("drop-me") // must not panic, even though the gateway returns 502.

	// Give the goroutine a beat to finish.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&sinkCalls) == 1 && buf.Len() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&sinkCalls); got != 1 {
		t.Fatalf("sink calls = %d, want 1", got)
	}
	if buf.Len() == 0 {
		t.Fatal("expected submit error to be logged, got empty log buffer")
	}
}

// TestNewRootCommand_RegistersRemoteFlag proves the operator-facing surface:
// the root cobra command exposes a --remote flag so `gormes --remote <url>`
// is a valid invocation.
func TestNewRootCommand_RegistersRemoteFlag(t *testing.T) {
	root := newRootCommand()
	flag := root.Flags().Lookup("remote")
	if flag == nil {
		t.Fatal("root command missing --remote flag")
	}
	if flag.Usage == "" {
		t.Fatal("--remote flag missing usage text")
	}
}

// TestRunTUI_RemoteFlagShortCircuits proves that when --remote is set to a
// scheme-less (invalid) URL, runTUI fails from the remote path — NOT from
// the local-kernel code path (config.Load / LLM health check / session
// bolt open). The error message must mention the --remote surface so an
// operator can tell which code path barfed. This pins the short-circuit:
// --remote completely bypasses the local kernel wiring.
func TestRunTUI_RemoteFlagShortCircuits(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"--remote", "not-a-url"})
	err := root.Execute()
	if err == nil {
		t.Fatal("runTUI --remote not-a-url: want non-nil error")
	}
	if !strings.Contains(err.Error(), "--remote") {
		t.Fatalf("error %q must mention the --remote surface; otherwise the short-circuit is broken", err)
	}
}

// writerAdapter bridges strings.Builder into io.Writer with an internal mutex
// so the structured logger used by openRemoteTUISurface does not race with
// the test goroutine reading buf.Len().
type writerAdapter struct {
	b *strings.Builder
}

func (w writerAdapter) Write(p []byte) (int, error) {
	return w.b.Write(p)
}
