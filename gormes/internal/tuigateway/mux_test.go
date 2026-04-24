package tuigateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

// TestNewRemoteClient_JoinsBaseURLWithPathConstants proves NewRemoteClient
// returns a client whose URLs are the base joined with the exported
// FramesPath / EventsPath constants. Using named constants on both sides
// keeps the wire convention auditable.
func TestNewRemoteClient_JoinsBaseURLWithPathConstants(t *testing.T) {
	t.Parallel()
	c, err := NewRemoteClient("https://gw.example/tuigateway")
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	if want := "https://gw.example/tuigateway" + FramesPath; c.FramesURL != want {
		t.Fatalf("FramesURL = %q, want %q", c.FramesURL, want)
	}
	if want := "https://gw.example/tuigateway" + EventsPath; c.EventsURL != want {
		t.Fatalf("EventsURL = %q, want %q", c.EventsURL, want)
	}
}

// TestNewRemoteClient_TrimsTrailingSlash proves a baseURL ending in "/"
// produces the same wiring as one without, so operators can pass either
// form from a --remote flag without double-slashing the path.
func TestNewRemoteClient_TrimsTrailingSlash(t *testing.T) {
	t.Parallel()
	withSlash, err := NewRemoteClient("https://gw.example/tuigateway/")
	if err != nil {
		t.Fatalf("NewRemoteClient (slash): %v", err)
	}
	withoutSlash, err := NewRemoteClient("https://gw.example/tuigateway")
	if err != nil {
		t.Fatalf("NewRemoteClient (no slash): %v", err)
	}
	if withSlash.FramesURL != withoutSlash.FramesURL {
		t.Fatalf("FramesURL trimming: %q vs %q", withSlash.FramesURL, withoutSlash.FramesURL)
	}
	if withSlash.EventsURL != withoutSlash.EventsURL {
		t.Fatalf("EventsURL trimming: %q vs %q", withSlash.EventsURL, withoutSlash.EventsURL)
	}
}

// TestNewRemoteClient_RejectsEmpty proves the constructor fails loudly on
// an empty base URL rather than returning a half-baked client that would
// later hang on a dial to "".
func TestNewRemoteClient_RejectsEmpty(t *testing.T) {
	t.Parallel()
	if _, err := NewRemoteClient(""); err == nil {
		t.Fatal("NewRemoteClient(\"\"): want non-nil error")
	}
}

// TestNewRemoteClient_RejectsRelative proves the constructor refuses a
// scheme-less / relative URL. A --remote flag populated with "example.com"
// would produce nonsensical URLs at dial time; catching it at construction
// is strictly better.
func TestNewRemoteClient_RejectsRelative(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"example.com", "/tuigateway", "gw.example/tuigateway"} {
		if _, err := NewRemoteClient(bad); err == nil {
			t.Fatalf("NewRemoteClient(%q): want non-nil error", bad)
		}
	}
}

// TestNewGatewayMux_RoutesFrames proves a mux built over a render channel
// and a sink serves the SSE stream at FramesPath.
func TestNewGatewayMux_RoutesFrames(t *testing.T) {
	t.Parallel()
	frames := make(chan kernel.RenderFrame, 1)
	frames <- kernel.RenderFrame{Seq: 3, Phase: kernel.PhaseStreaming, DraftText: "mux"}
	close(frames)

	sink := func(kernel.PlatformEvent) error { return nil }
	srv := httptest.NewServer(NewGatewayMux(frames, sink))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := DialSSE(ctx, srv.URL+FramesPath)
	if err != nil {
		t.Fatalf("DialSSE: %v", err)
	}
	select {
	case f, ok := <-out:
		if !ok {
			t.Fatal("frames channel closed before frame arrived")
		}
		if f.Seq != 3 || f.DraftText != "mux" {
			t.Fatalf("frame = %+v", f)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for frame via mux")
	}
}

// TestNewGatewayMux_RoutesEvents proves the mux serves the event handler
// at EventsPath, reaching the caller-supplied sink.
func TestNewGatewayMux_RoutesEvents(t *testing.T) {
	t.Parallel()
	frames := make(chan kernel.RenderFrame)
	close(frames)
	got := make(chan kernel.PlatformEvent, 1)
	sink := func(e kernel.PlatformEvent) error { got <- e; return nil }

	srv := httptest.NewServer(NewGatewayMux(frames, sink))
	defer srv.Close()

	if err := PostPlatformEvent(context.Background(), srv.URL+EventsPath, kernel.PlatformEvent{
		Kind: kernel.PlatformEventSubmit,
		Text: "mux submit",
	}); err != nil {
		t.Fatalf("PostPlatformEvent: %v", err)
	}
	select {
	case e := <-got:
		if e.Kind != kernel.PlatformEventSubmit || e.Text != "mux submit" {
			t.Fatalf("sink got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("sink did not receive event via mux")
	}
}

// TestNewGatewayMux_UnknownPathIs404 proves the mux does not silently route
// stray requests into one of the handlers.
func TestNewGatewayMux_UnknownPathIs404(t *testing.T) {
	t.Parallel()
	frames := make(chan kernel.RenderFrame)
	close(frames)
	sink := func(kernel.PlatformEvent) error { return nil }
	srv := httptest.NewServer(NewGatewayMux(frames, sink))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatalf("GET /nope: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /nope status = %d, want 404", resp.StatusCode)
	}
}

// TestRemoteClient_RoundTripsViaMux proves the client and mux agree on the
// path convention: a RemoteClient built from NewRemoteClient pointed at a
// server wrapping NewGatewayMux both receives a frame AND delivers an
// upstream event without the caller ever naming a path.
func TestRemoteClient_RoundTripsViaMux(t *testing.T) {
	t.Parallel()
	frames := make(chan kernel.RenderFrame, 1)
	frames <- kernel.RenderFrame{Seq: 11, Phase: kernel.PhaseStreaming, DraftText: "round"}
	// Leave open so Frames() stays alive while we post an event; we'll
	// close via context cancel below.
	got := make(chan kernel.PlatformEvent, 1)
	sink := func(e kernel.PlatformEvent) error { got <- e; return nil }

	srv := httptest.NewServer(NewGatewayMux(frames, sink))
	defer srv.Close()

	client, err := NewRemoteClient(srv.URL)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := client.Frames(ctx)
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	select {
	case f := <-out:
		if f.Seq != 11 || f.DraftText != "round" {
			t.Fatalf("round-trip frame = %+v", f)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for round-trip frame")
	}

	if err := client.Submit(context.Background(), "round-trip submit"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case e := <-got:
		if e.Kind != kernel.PlatformEventSubmit || e.Text != "round-trip submit" {
			t.Fatalf("round-trip sink got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("round-trip sink did not receive event")
	}

	// The FramesURL/EventsURL should incorporate the exported paths; guard
	// explicitly so a future refactor that renames the path constants is
	// caught here and not only by the round-trip tests above.
	if !strings.HasSuffix(client.FramesURL, FramesPath) {
		t.Fatalf("FramesURL %q does not end with FramesPath %q", client.FramesURL, FramesPath)
	}
	if !strings.HasSuffix(client.EventsURL, EventsPath) {
		t.Fatalf("EventsURL %q does not end with EventsPath %q", client.EventsURL, EventsPath)
	}
}
