package tuigateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestJSONRPCDispatcher_DispatchSuccess proves a registered handler is invoked
// with the request params and its non-error return value round-trips as the
// response `result`. This mirrors the upstream `tui_gateway/server.py` shape:
// `_ok(rid, result)` returns `{"jsonrpc":"2.0","id":rid,"result":result}`.
func TestJSONRPCDispatcher_DispatchSuccess(t *testing.T) {
	t.Parallel()

	d := NewJSONRPCDispatcher()
	d.Register("ping", func(_ context.Context, params json.RawMessage) (any, *JSONRPCError) {
		return map[string]string{"pong": string(params)}, nil
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"req-1"`),
		Method:  "ping",
		Params:  json.RawMessage(`"hello"`),
	}
	resp := d.Dispatch(context.Background(), req)

	if resp.JSONRPC != "2.0" {
		t.Fatalf("JSONRPC = %q, want 2.0", resp.JSONRPC)
	}
	if string(resp.ID) != `"req-1"` {
		t.Fatalf("ID = %s, want \"req-1\"", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("Error = %+v, want nil", resp.Error)
	}
	// Result should be a map with the echoed params.
	m, ok := resp.Result.(map[string]string)
	if !ok {
		t.Fatalf("Result type = %T, want map[string]string", resp.Result)
	}
	if m["pong"] != `"hello"` {
		t.Fatalf(`Result["pong"] = %q, want "hello"`, m["pong"])
	}
}

// TestJSONRPCDispatcher_UnknownMethod proves an unknown method yields the
// JSON-RPC standard error `-32601: method not found` (matching upstream
// `_err(rid, -32601, f"unknown method: {method}")` at server.py:199) without
// invoking any registered handler.
func TestJSONRPCDispatcher_UnknownMethod(t *testing.T) {
	t.Parallel()

	var invocations int32
	d := NewJSONRPCDispatcher()
	d.Register("known", func(context.Context, json.RawMessage) (any, *JSONRPCError) {
		atomic.AddInt32(&invocations, 1)
		return nil, nil
	})

	resp := d.Dispatch(context.Background(), JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`42`),
		Method:  "nonexistent",
	})
	if resp.Error == nil {
		t.Fatal("Error = nil, want method-not-found error")
	}
	if resp.Error.Code != JSONRPCCodeMethodNotFound {
		t.Fatalf("Error.Code = %d, want %d", resp.Error.Code, JSONRPCCodeMethodNotFound)
	}
	if resp.Result != nil {
		t.Fatalf("Result = %v, want nil on error", resp.Result)
	}
	if got := atomic.LoadInt32(&invocations); got != 0 {
		t.Fatalf("known handler invoked %d times; expected 0", got)
	}
}

// TestJSONRPCDispatcher_HandlerError proves a handler-returned *JSONRPCError
// surfaces as the response's `error` field and `result` stays nil. This
// matches server.py where every method helper returns `_err(...)` on failure
// rather than raising.
func TestJSONRPCDispatcher_HandlerError(t *testing.T) {
	t.Parallel()

	d := NewJSONRPCDispatcher()
	d.Register("bang", func(context.Context, json.RawMessage) (any, *JSONRPCError) {
		return nil, &JSONRPCError{Code: 5032, Message: "agent not ready"}
	})

	resp := d.Dispatch(context.Background(), JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"r"`),
		Method:  "bang",
	})
	if resp.Error == nil {
		t.Fatal("Error = nil, want handler error")
	}
	if resp.Error.Code != 5032 {
		t.Fatalf("Error.Code = %d, want 5032", resp.Error.Code)
	}
	if resp.Error.Message != "agent not ready" {
		t.Fatalf("Error.Message = %q, want %q", resp.Error.Message, "agent not ready")
	}
	if resp.Result != nil {
		t.Fatalf("Result = %v, want nil when Error is set", resp.Result)
	}
	if string(resp.ID) != `"r"` {
		t.Fatalf("ID = %s, want \"r\"", resp.ID)
	}
}

// TestJSONRPCHandler_HappyPath proves the HTTP handler decodes a posted
// request, dispatches to the registered method, and encodes the JSONRPCResponse.
func TestJSONRPCHandler_HappyPath(t *testing.T) {
	t.Parallel()

	d := NewJSONRPCDispatcher()
	d.Register("echo", func(_ context.Context, params json.RawMessage) (any, *JSONRPCError) {
		return json.RawMessage(params), nil
	})

	srv := httptest.NewServer(NewJSONRPCHandler(d))
	defer srv.Close()

	body := []byte(`{"jsonrpc":"2.0","id":7,"method":"echo","params":{"k":"v"}}`)
	res, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *JSONRPCError   `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if envelope.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", envelope.JSONRPC)
	}
	if string(envelope.ID) != "7" {
		t.Errorf("id = %s, want 7", envelope.ID)
	}
	if envelope.Error != nil {
		t.Errorf("error = %+v, want nil", envelope.Error)
	}
	if !bytes.Contains(envelope.Result, []byte(`"k":"v"`)) {
		t.Errorf("result = %s, want params echoed", envelope.Result)
	}
}

// TestJSONRPCHandler_ParseError proves malformed JSON yields the JSON-RPC
// `-32700 parse error` response (with id=null) rather than a raw 400 — this
// keeps the error channel consistent so clients only need one decoder path.
func TestJSONRPCHandler_ParseError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewJSONRPCHandler(NewJSONRPCDispatcher()))
	defer srv.Close()

	res, err := http.Post(srv.URL, "application/json", strings.NewReader("not-json"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var envelope struct {
		Error *JSONRPCError `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if envelope.Error == nil || envelope.Error.Code != JSONRPCCodeParseError {
		t.Fatalf("Error = %+v, want code %d", envelope.Error, JSONRPCCodeParseError)
	}
}

// TestJSONRPCHandler_NonPost proves only POST reaches the dispatcher.
func TestJSONRPCHandler_NonPost(t *testing.T) {
	t.Parallel()

	var invocations int32
	d := NewJSONRPCDispatcher()
	d.Register("x", func(context.Context, json.RawMessage) (any, *JSONRPCError) {
		atomic.AddInt32(&invocations, 1)
		return nil, nil
	})
	h := NewJSONRPCHandler(d)

	req := httptest.NewRequest(http.MethodGet, "/rpc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := atomic.LoadInt32(&invocations); got != 0 {
		t.Fatalf("dispatcher invoked %d times on GET; expected 0", got)
	}
}
