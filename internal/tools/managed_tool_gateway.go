package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ManagedGatewayEvidence is the operator-visible degraded-mode label for a
// managed tool gateway bridge. Values are stable strings consumed by status
// surfaces and structured logs; secret material is never used as evidence.
type ManagedGatewayEvidence string

const (
	// ManagedGatewayEvidenceOK signals a healthy bridge: discovery succeeded
	// and every advertised tool normalized through the shared MCP descriptor
	// path.
	ManagedGatewayEvidenceOK ManagedGatewayEvidence = "ok"

	// ManagedGatewayEvidenceUnavailable signals the gateway returned a
	// non-auth transport failure (timeout, 5xx, unreachable host). No tools
	// are registered when this evidence is reported.
	ManagedGatewayEvidenceUnavailable ManagedGatewayEvidence = "gateway_unavailable"

	// ManagedGatewayEvidenceAuthRequired signals the gateway rejected the
	// supplied bearer token. Callers should drive recovery through the
	// shared MCP OAuth refresh path; no tools are registered until the
	// bridge re-discovers successfully.
	ManagedGatewayEvidenceAuthRequired ManagedGatewayEvidence = "auth_required"

	// ManagedGatewayEvidenceSchemaRejected signals discovery succeeded but
	// at least one advertised tool failed schema normalization. The
	// successfully-normalized tools are still returned; rejected entries
	// land in the Rejected slice with their reason.
	ManagedGatewayEvidenceSchemaRejected ManagedGatewayEvidence = "schema_rejected"

	// ManagedGatewayEvidenceToolCallFailed signals a tools/call attempt
	// failed either at the transport layer or via a JSON-RPC error
	// envelope, or returned isError=true inside an otherwise successful
	// response.
	ManagedGatewayEvidenceToolCallFailed ManagedGatewayEvidence = "tool_call_failed"
)

// ManagedGatewayDefinition is the static config for a single managed-tool
// gateway. Token is the operator-provided bearer credential mounted onto the
// outgoing Authorization header; it is only retained on the bridge struct
// long enough to be wired into the underlying HTTP MCP client.
type ManagedGatewayDefinition struct {
	// Vendor is the gateway's logical name (e.g. "firecrawl"). It is also
	// used as the ServerName seed for the shared NormalizeTools path so
	// sanitized tool identifiers stay stable across runs.
	Vendor string
	// Origin is the gateway URL (scheme + host[:port]).
	Origin string
	// Token is the operator-level OAuth bearer token forwarded as
	// `Authorization: Bearer <token>` when no explicit Authorization header
	// is supplied via Headers.
	Token string
	// Headers is an optional map of extra headers forwarded with every
	// request. An explicit Authorization header here wins over Token.
	Headers map[string]string
	// Timeout caps individual request durations; 0 leaves the underlying
	// transport's default in place.
	Timeout time.Duration
}

// ManagedGatewayBridge bridges discovered managed-gateway tools onto the
// shared MCP descriptor/call contract. It speaks the same JSON-RPC subset as
// the HTTP MCP transport so fixtures can swap a fake managed gateway for a
// fake MCP server without touching call sites.
type ManagedGatewayBridge struct {
	def    ManagedGatewayDefinition
	client *HTTPClient

	initOnce  bool
	initialed bool
}

// ManagedGatewayDiscovery is the result of a Discover call. Tools and
// Rejected come from the shared NormalizeTools path; Evidence summarizes the
// degraded-mode state without leaking transport-specific error types.
type ManagedGatewayDiscovery struct {
	Tools    []NormalizedTool
	Rejected []SchemaRejection
	Evidence ManagedGatewayEvidence
}

// NewManagedGatewayBridge constructs a bridge over the supplied HTTP
// transport. Tests inject httptest.Server's RoundTripper so no live gateway
// is contacted. The constructor refuses partially-configured definitions so
// half-discovered tools cannot surface from a misconfigured bridge.
func NewManagedGatewayBridge(def ManagedGatewayDefinition, transport http.RoundTripper) (*ManagedGatewayBridge, error) {
	if strings.TrimSpace(def.Vendor) == "" {
		return nil, errors.New("managed gateway: empty vendor")
	}
	if strings.TrimSpace(def.Origin) == "" {
		return nil, errors.New("managed gateway: empty origin")
	}
	headers := map[string]string{}
	for k, v := range def.Headers {
		headers[k] = v
	}
	if def.Token != "" {
		// Only mount the auto-Bearer when the caller did not already
		// override Authorization explicitly; honoring the override keeps
		// vendor-specific schemes (e.g. "Token <...>") working without
		// special casing them here.
		if !hasAuthorizationHeader(headers) {
			headers["Authorization"] = "Bearer " + def.Token
		}
	}
	serverDef := MCPServerDefinition{
		Name:      def.Vendor,
		Enabled:   true,
		Transport: MCPTransportHTTP,
		URL:       def.Origin,
		Headers:   headers,
		Timeout:   def.Timeout,
	}
	client, err := NewHTTPClient(serverDef, HTTPClientOpts{
		Transport: transport,
		Logger:    slog.Default(),
	})
	if err != nil {
		return nil, fmt.Errorf("managed gateway: %w", err)
	}
	return &ManagedGatewayBridge{def: def, client: client}, nil
}

// hasAuthorizationHeader reports whether the supplied headers map already
// contains an Authorization entry (case-insensitive lookup).
func hasAuthorizationHeader(headers map[string]string) bool {
	for key := range headers {
		if strings.EqualFold(key, "Authorization") {
			return true
		}
	}
	return false
}

// Vendor returns the configured vendor name. Useful for callers logging the
// bridge identity without reaching into the definition struct.
func (b *ManagedGatewayBridge) Vendor() string {
	if b == nil {
		return ""
	}
	return b.def.Vendor
}

// Initialize negotiates the MCP handshake with the gateway. It is safe to
// call multiple times: only the first successful invocation actually opens a
// session.
func (b *ManagedGatewayBridge) Initialize(ctx context.Context) error {
	if b == nil {
		return errors.New("managed gateway: nil bridge")
	}
	if b.initialed {
		return nil
	}
	b.initOnce = true
	if err := b.client.Initialize(ctx); err != nil {
		return err
	}
	b.initialed = true
	return nil
}

// Discover initializes the gateway, lists tools, and runs them through the
// shared NormalizeTools path. On any transport-level error the result has
// zero registered tools and Evidence reports the degraded state, so callers
// cannot accidentally promote half-discovered inventory.
func (b *ManagedGatewayBridge) Discover(ctx context.Context) (ManagedGatewayDiscovery, error) {
	if b == nil {
		return ManagedGatewayDiscovery{Evidence: ManagedGatewayEvidenceUnavailable}, errors.New("managed gateway: nil bridge")
	}
	if err := b.Initialize(ctx); err != nil {
		return ManagedGatewayDiscovery{Evidence: classifyDiscoverError(err)}, err
	}
	raw, err := b.client.ListTools(ctx)
	if err != nil {
		return ManagedGatewayDiscovery{Evidence: classifyDiscoverError(err)}, err
	}
	norm := NormalizeTools(b.def.Vendor, raw)
	evidence := ManagedGatewayEvidenceOK
	if len(norm.Rejected) > 0 {
		evidence = ManagedGatewayEvidenceSchemaRejected
	}
	return ManagedGatewayDiscovery{
		Tools:    norm.Tools,
		Rejected: norm.Rejected,
		Evidence: evidence,
	}, nil
}

// CallTool invokes the named tool on the gateway, passing through arguments,
// timeouts (via ctx deadline), and cancellation. The bridge does not
// re-sanitize the name: callers are expected to forward either the
// SourceRaw.Name from a NormalizedTool descriptor or any name the gateway
// itself accepts so server-side dispatch stays stable across runs.
func (b *ManagedGatewayBridge) CallTool(ctx context.Context, name string, arguments map[string]any) (MCPCallResult, ManagedGatewayEvidence, error) {
	if b == nil {
		return MCPCallResult{}, ManagedGatewayEvidenceUnavailable, errors.New("managed gateway: nil bridge")
	}
	res, err := b.client.CallTool(ctx, name, arguments)
	if err != nil {
		return MCPCallResult{}, classifyCallToolError(err), err
	}
	if res.IsError {
		return res, ManagedGatewayEvidenceToolCallFailed, nil
	}
	return res, ManagedGatewayEvidenceOK, nil
}

// Close releases the underlying transport. Safe to call multiple times.
func (b *ManagedGatewayBridge) Close() error {
	if b == nil || b.client == nil {
		return nil
	}
	return b.client.Close()
}

// classifyDiscoverError maps an Initialize/ListTools failure onto the
// operator-visible evidence enum. Auth failures are routed through the
// shared MCP OAuth recovery path; everything else (connectivity, 5xx,
// timeouts) is indistinguishable to the caller and surfaces as
// gateway_unavailable so degraded-mode reporting stays consistent.
func classifyDiscoverError(err error) ManagedGatewayEvidence {
	if err == nil {
		return ManagedGatewayEvidenceOK
	}
	if errors.Is(err, ErrAuthRequired) {
		return ManagedGatewayEvidenceAuthRequired
	}
	return ManagedGatewayEvidenceUnavailable
}

// classifyCallToolError maps a tools/call transport failure onto the
// evidence enum. Auth failures route to auth_required so the caller can
// drive recovery via the MCP OAuth refresh contract; every other transport
// failure (timeouts, JSON-RPC errors, body decode errors) surfaces as
// tool_call_failed because the call itself never produced a structured
// result.
func classifyCallToolError(err error) ManagedGatewayEvidence {
	if err == nil {
		return ManagedGatewayEvidenceOK
	}
	if errors.Is(err, ErrAuthRequired) {
		return ManagedGatewayEvidenceAuthRequired
	}
	return ManagedGatewayEvidenceToolCallFailed
}
