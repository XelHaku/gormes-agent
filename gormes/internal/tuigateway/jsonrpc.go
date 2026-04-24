package tuigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// JSON-RPC 2.0 reserved error codes. These mirror the values used by the
// upstream `tui_gateway/server.py` helpers (`_err(rid, -32601, ...)`) so
// ported method implementations emit identical wire errors.
const (
	JSONRPCCodeParseError     = -32700
	JSONRPCCodeInvalidRequest = -32600
	JSONRPCCodeMethodNotFound = -32601
	JSONRPCCodeInvalidParams  = -32602
	JSONRPCCodeInternalError  = -32603
)

// maxRPCBody bounds the POST body read by NewJSONRPCHandler. 1 MiB is
// generous for JSON-RPC envelopes (which rarely exceed a few KB once params
// are encoded) without making the handler a DoS vector.
const maxRPCBody = 1 << 20

// JSONRPCRequest is the decoded shape of an incoming JSON-RPC 2.0 request.
// ID is kept as json.RawMessage so string, number, and null ids all survive
// the round trip unchanged — JSON-RPC 2.0 §4 permits any of those types.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is the reply envelope. Exactly one of Result or Error is
// populated for a well-formed response; the dispatcher guarantees this.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is the error sub-envelope carried by JSONRPCResponse. Matches
// `_err(rid, code, msg)` in `tui_gateway/server.py`.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("jsonrpc: %d: %s", e.Code, e.Message)
}

// JSONRPCHandler is the per-method callback the dispatcher invokes. Handlers
// receive the request context and raw params bytes (so they can decode into
// whatever concrete struct they own) and return either a JSON-encodable
// result or a *JSONRPCError. Exactly one of the return values must be set.
type JSONRPCHandler func(ctx context.Context, params json.RawMessage) (any, *JSONRPCError)

// JSONRPCDispatcher maps method names to handlers. It is the Go port of the
// `method(name)` decorator + `_methods` registry + `handle_request(req)`
// dispatcher at `tui_gateway/server.py:189-200`.
//
// Zero value is NOT usable; construct with NewJSONRPCDispatcher.
//
// Safe for concurrent Register/Dispatch calls: Register takes a write lock,
// Dispatch a read lock. In practice methods are registered at startup and
// dispatched under load, so the critical path is RLock-only.
type JSONRPCDispatcher struct {
	mu      sync.RWMutex
	methods map[string]JSONRPCHandler
}

// NewJSONRPCDispatcher returns an empty dispatcher ready for Register calls.
func NewJSONRPCDispatcher() *JSONRPCDispatcher {
	return &JSONRPCDispatcher{methods: make(map[string]JSONRPCHandler)}
}

// Register installs handler under name. A second Register for the same name
// replaces the previous entry — matching the Python `@method(name)` behaviour
// where a later decorator wins.
func (d *JSONRPCDispatcher) Register(name string, handler JSONRPCHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.methods[name] = handler
}

// Dispatch routes req to the registered handler and returns a response
// envelope with id, jsonrpc version, and either result or error populated.
//
// If req.Method has no registered handler, Dispatch returns a
// `-32601 method not found` error without ever consulting the handler map
// again.
func (d *JSONRPCDispatcher) Dispatch(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	d.mu.RLock()
	h, ok := d.methods[req.Method]
	d.mu.RUnlock()
	if !ok {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    JSONRPCCodeMethodNotFound,
				Message: fmt.Sprintf("unknown method: %s", req.Method),
			},
		}
	}
	result, rpcErr := h(ctx, req.Params)
	if rpcErr != nil {
		return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
	}
	return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

// NewJSONRPCHandler wraps a JSONRPCDispatcher in a minimal HTTP surface.
//
// Contract:
//   - POST only; other verbs return 405 without touching the dispatcher.
//   - Body must decode as a JSONRPCRequest; malformed JSON yields a 200
//     response whose body is a `-32700 parse error` JSON-RPC envelope, so
//     clients only need one decoder path.
//   - Successful dispatch → 200 with the JSON-encoded JSONRPCResponse.
//
// The handler deliberately does NOT implement batch requests or SSE push;
// both are follow-on scope once the first concrete method lands.
func NewJSONRPCHandler(d *JSONRPCDispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRPCBody))
		if err != nil {
			writeJSONRPC(w, JSONRPCResponse{
				JSONRPC: "2.0",
				Error: &JSONRPCError{
					Code:    JSONRPCCodeParseError,
					Message: "read body: " + err.Error(),
				},
			})
			return
		}
		var req JSONRPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONRPC(w, JSONRPCResponse{
				JSONRPC: "2.0",
				Error: &JSONRPCError{
					Code:    JSONRPCCodeParseError,
					Message: err.Error(),
				},
			})
			return
		}
		writeJSONRPC(w, d.Dispatch(r.Context(), req))
	})
}

func writeJSONRPC(w http.ResponseWriter, resp JSONRPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	// JSON-RPC keeps the status at 200 even for method-level errors: the
	// transport succeeded, the error is part of the protocol envelope.
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
