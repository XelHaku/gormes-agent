package tuigateway

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

// FramesPath and EventsPath are the canonical mount points on a remote TUI
// gateway. NewGatewayMux mounts NewSSEHandler at FramesPath and
// NewEventHandler at EventsPath; NewRemoteClient builds its FramesURL and
// EventsURL by joining the operator-supplied base URL with these constants.
// Keeping both sides referenced through named constants means a future
// wire-convention change surfaces as a compile-time diff instead of a
// silent mismatch between the `cmd/gormes --remote` flag and the server.
const (
	FramesPath = "/frames"
	EventsPath = "/events"
)

// NewRemoteClient constructs a RemoteClient for a gateway rooted at
// baseURL. It concatenates FramesPath and EventsPath onto baseURL (with any
// trailing slash trimmed) so operators can pass either `https://x/tui` or
// `https://x/tui/` from a --remote flag without double-slashing the path.
//
// baseURL must be a parseable absolute URL with both scheme and host set.
// Empty or scheme-less inputs fail at construction time rather than at
// dial time so the --remote startup path bails loudly with a useful error
// message instead of hanging on an unreachable URL.
func NewRemoteClient(baseURL string) (*RemoteClient, error) {
	trimmed := strings.TrimRight(baseURL, "/")
	if trimmed == "" {
		return nil, fmt.Errorf("tuigateway: base URL is empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("tuigateway: parse base URL %q: %w", baseURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("tuigateway: base URL %q must be absolute (scheme://host/...)", baseURL)
	}
	return &RemoteClient{
		FramesURL: trimmed + FramesPath,
		EventsURL: trimmed + EventsPath,
	}, nil
}

// NewGatewayMux returns an http.Handler that mounts NewSSEHandler(frames)
// at FramesPath and NewEventHandler(sink) at EventsPath. Any other path
// returns 404 so stray requests are never silently forwarded to one of the
// handlers. It is the mirror of NewRemoteClient: the two constructors share
// the same path constants so operators configuring a gateway cannot drift
// away from the wire convention.
func NewGatewayMux(frames <-chan kernel.RenderFrame, sink PlatformEventSink) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(FramesPath, NewSSEHandler(frames))
	mux.Handle(EventsPath, NewEventHandler(sink))
	return mux
}
