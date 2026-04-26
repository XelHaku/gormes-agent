package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// httpProtocolVersion is the MCP protocol version Gormes negotiates over the
// HTTP transport. It mirrors stdioProtocolVersion so both clients present a
// consistent capability surface to the server.
const httpProtocolVersion = "2024-11-05"

// ErrAuthRequired is returned when the HTTP MCP server replies with 401
// Unauthorized. The OAuth follow-up row keys recovery off this typed error.
var ErrAuthRequired = errors.New("mcp http: authentication required")

// HTTPClientOpts injects the http.RoundTripper plus optional observability
// hooks. Tests pass an httptest.Server-backed transport so no real socket is
// opened outside httptest.
type HTTPClientOpts struct {
	Transport http.RoundTripper
	Logger    *slog.Logger
	Now       func() time.Time
}

// HTTPClient speaks JSON-RPC over HTTP to an MCP server. It is the minimal
// MCP HTTP surface needed for `initialize` plus `tools/list`; SSE response
// streaming, OAuth, and structured content normalization live in follow-up
// rows.
type HTTPClient struct {
	def    MCPServerDefinition
	http   *http.Client
	logger *slog.Logger
	now    func() time.Time

	nextID atomic.Int64

	closeMu sync.Mutex
	closed  bool

	versionMu       sync.RWMutex
	protocolVersion string
}

// NewHTTPClient constructs an HTTPClient over the supplied transport. The
// def.URL must be a non-empty HTTP(S) endpoint; transport choice is left to
// the caller so tests can inject httptest.Server's RoundTripper without ever
// opening a real socket.
func NewHTTPClient(def MCPServerDefinition, opts HTTPClientOpts) (*HTTPClient, error) {
	if def.URL == "" {
		return nil, errors.New("mcp http: empty URL")
	}
	transport := opts.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &HTTPClient{
		def:    def,
		http:   &http.Client{Transport: transport},
		logger: logger,
		now:    now,
	}, nil
}

// ProtocolVersion returns the protocol version the server reported during
// Initialize. Empty before Initialize succeeds.
func (c *HTTPClient) ProtocolVersion() string {
	c.versionMu.RLock()
	defer c.versionMu.RUnlock()
	return c.protocolVersion
}

// Initialize performs the MCP handshake over HTTP and records the negotiated
// protocol version.
func (c *HTTPClient) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": httpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "gormes",
			"version": "0.0.0",
		},
	}
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		var rpcErr *jsonRPCError
		if errors.As(err, &rpcErr) {
			return fmt.Errorf("%w: %s", ErrInitializeFailed, rpcErr.Message)
		}
		return err
	}
	c.versionMu.Lock()
	c.protocolVersion = result.ProtocolVersion
	c.versionMu.Unlock()
	return nil
}

// ListTools fetches the server's tools/list response and returns the verbatim
// tool envelopes (no schema normalization).
func (c *HTTPClient) ListTools(ctx context.Context) ([]MCPRawTool, error) {
	var result struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema,omitempty"`
		} `json:"tools"`
	}
	if err := c.call(ctx, "tools/list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	out := make([]MCPRawTool, 0, len(result.Tools))
	for _, t := range result.Tools {
		out = append(out, MCPRawTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return out, nil
}

// Close marks the client closed and releases idle connections held by the
// underlying transport. Safe to call multiple times.
func (c *HTTPClient) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if t, ok := c.http.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
	return nil
}

// call posts a single JSON-RPC request and returns the decoded result.
// HTTP-level failures are mapped to typed errors so callers can branch on
// auth/timeout cases without inspecting status codes themselves.
func (c *HTTPClient) call(ctx context.Context, method string, params any, out any) error {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()
	if closed {
		return io.ErrClosedPipe
	}

	id := c.nextID.Add(1)
	req := jsonRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mcp http: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.def.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("mcp http: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// MCP Streamable HTTP servers may reply with JSON or SSE. We only parse
	// JSON in this slice; advertising both keeps spec-compliant servers happy
	// while letting them choose the JSON branch.
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.def.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrConnectTimeout, err)
		}
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		return fmt.Errorf("mcp http: do request: %w", err)
	}
	defer func() {
		// Drain and close even on error paths; do not panic if the server
		// hung up mid-stream.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: status %d", ErrAuthRequired, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp http: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrConnectTimeout, err)
		}
		return fmt.Errorf("mcp http: read response: %w", err)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJSONRPCResponse, err)
	}
	if rpcResp.Error != nil {
		return rpcResp.Error
	}
	if out != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, out); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidJSONRPCResponse, err)
		}
	}
	return nil
}
