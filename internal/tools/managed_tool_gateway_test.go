package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeManagedGatewayRequest is the JSON-RPC envelope decoded from a managed
// gateway HTTP request. Tests assert against the parsed view rather than the
// raw bytes so the assertions stay readable.
type fakeManagedGatewayRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// fakeManagedGatewayServer is an httptest-backed gateway. It speaks the same
// JSON-RPC subset Hermes' managed gateway proxies (initialize, tools/list,
// tools/call) so the bridge tests run with zero live dependencies.
type fakeManagedGatewayServer struct {
	srv      *httptest.Server
	requests atomic.Int64

	mu       sync.Mutex
	authSeen string
	lastBody []byte
	lastCall struct {
		name      string
		arguments json.RawMessage
	}
}

func newFakeManagedGatewayServer(t *testing.T, handler func(t *testing.T, w http.ResponseWriter, r *http.Request, req fakeManagedGatewayRequest)) *fakeManagedGatewayServer {
	t.Helper()
	f := &fakeManagedGatewayServer{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req fakeManagedGatewayRequest
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("decode body %q: %v", string(body), err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		f.mu.Lock()
		f.authSeen = r.Header.Get("Authorization")
		f.lastBody = append(f.lastBody[:0], body...)
		if req.Method == "tools/call" {
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err == nil {
				f.lastCall.name = p.Name
				f.lastCall.arguments = append(f.lastCall.arguments[:0], p.Arguments...)
			}
		}
		f.mu.Unlock()
		handler(t, w, r, req)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeManagedGatewayServer) URL() string { return f.srv.URL }

func (f *fakeManagedGatewayServer) Transport() http.RoundTripper {
	return f.srv.Client().Transport
}

func (f *fakeManagedGatewayServer) AuthHeader() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authSeen
}

func (f *fakeManagedGatewayServer) LastCallName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastCall.name
}

func (f *fakeManagedGatewayServer) LastCallArguments() json.RawMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(json.RawMessage, len(f.lastCall.arguments))
	copy(out, f.lastCall.arguments)
	return out
}

func writeManagedJSONResult(w http.ResponseWriter, id int64, result string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":`+strconv.FormatInt(id, 10)+`,"result":`+result+`}`)
}

func writeManagedJSONError(w http.ResponseWriter, id int64, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w,
		`{"jsonrpc":"2.0","id":`+strconv.FormatInt(id, 10)+
			`,"error":{"code":`+strconv.Itoa(code)+`,"message":`+strconv.Quote(message)+`}}`)
}

func newTestManagedGatewayBridge(t *testing.T, vendor string, srv *fakeManagedGatewayServer, token string) *ManagedGatewayBridge {
	t.Helper()
	def := ManagedGatewayDefinition{
		Vendor: vendor,
		Origin: srv.URL(),
		Token:  token,
	}
	bridge, err := NewManagedGatewayBridge(def, srv.Transport())
	if err != nil {
		t.Fatalf("NewManagedGatewayBridge: %v", err)
	}
	if bridge == nil {
		t.Fatal("NewManagedGatewayBridge returned nil bridge")
	}
	t.Cleanup(func() { _ = bridge.Close() })
	return bridge
}

// TestManagedGatewayBridge_DiscoversInventoryThroughSharedNormalizer covers
// acceptance #1: a fake managed gateway returns a tool inventory that
// normalizes through the same descriptor path as MCP fake servers.
func TestManagedGatewayBridge_DiscoversInventoryThroughSharedNormalizer(t *testing.T) {
	const schemaA = `{"type":"object","properties":{"query":{"type":"string"}}}`
	srv := newFakeManagedGatewayServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request, req fakeManagedGatewayRequest) {
		switch req.Method {
		case "initialize":
			writeManagedJSONResult(w, req.ID, `{"protocolVersion":"2024-11-05","capabilities":{}}`)
		case "tools/list":
			writeManagedJSONResult(w, req.ID,
				`{"tools":[`+
					`{"name":"web/search","description":"managed search","inputSchema":`+schemaA+`},`+
					`{"name":"web fetch","description":"managed fetch"}`+
					`]}`)
		default:
			t.Errorf("unexpected method %q", req.Method)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	})

	bridge := newTestManagedGatewayBridge(t, "firecrawl", srv, "nous-token")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	disc, err := bridge.Discover(ctx)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if disc.Evidence != ManagedGatewayEvidenceOK {
		t.Fatalf("Evidence = %q, want %q", disc.Evidence, ManagedGatewayEvidenceOK)
	}
	if len(disc.Rejected) != 0 {
		t.Fatalf("Rejected = %+v, want none", disc.Rejected)
	}
	if len(disc.Tools) != 2 {
		t.Fatalf("Tools len = %d, want 2", len(disc.Tools))
	}
	// Names must be sanitized through the shared NormalizeTools path so the
	// managed bridge stays interchangeable with stdio/http MCP fake servers.
	if disc.Tools[0].Name != "web_search" {
		t.Errorf("Tools[0].Name = %q, want %q", disc.Tools[0].Name, "web_search")
	}
	if disc.Tools[0].SourceRaw.Name != "web/search" {
		t.Errorf("Tools[0].SourceRaw.Name = %q, want %q", disc.Tools[0].SourceRaw.Name, "web/search")
	}
	if disc.Tools[0].ServerName != "firecrawl" {
		t.Errorf("Tools[0].ServerName = %q, want %q", disc.Tools[0].ServerName, "firecrawl")
	}
	if string(disc.Tools[0].InputSchema) != schemaA {
		t.Errorf("Tools[0].InputSchema = %s, want %s", string(disc.Tools[0].InputSchema), schemaA)
	}
	if disc.Tools[1].Name != "web_fetch" {
		t.Errorf("Tools[1].Name = %q, want %q", disc.Tools[1].Name, "web_fetch")
	}
	// The shared normalizer fills in a permissive default for missing schemas
	// instead of leaving the raw envelope blank.
	if !strings.Contains(string(disc.Tools[1].InputSchema), `"type":"object"`) {
		t.Errorf("Tools[1].InputSchema = %s, want defaulted object schema", string(disc.Tools[1].InputSchema))
	}
	// Auth header must arrive as a Bearer token derived from def.Token so the
	// managed gateway trusts the request without provider-specific code.
	if got := srv.AuthHeader(); got != "Bearer nous-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer nous-token")
	}
}

// TestManagedGatewayBridge_PassesThroughToolCallArguments covers acceptance
// #2: tool calls pass through request IDs, arguments, and produce normalized
// success results via the shared StructuredContent renderer.
func TestManagedGatewayBridge_PassesThroughToolCallArguments(t *testing.T) {
	srv := newFakeManagedGatewayServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request, req fakeManagedGatewayRequest) {
		switch req.Method {
		case "initialize":
			writeManagedJSONResult(w, req.ID, `{"protocolVersion":"2024-11-05","capabilities":{}}`)
		case "tools/call":
			// Echo the call envelope back so the test can prove the bridge
			// preserved the request ID alongside the arguments.
			writeManagedJSONResult(w, req.ID,
				`{"content":[{"type":"text","text":"echo ok"}],"isError":false}`)
		default:
			t.Errorf("unexpected method %q", req.Method)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	})

	bridge := newTestManagedGatewayBridge(t, "firecrawl", srv, "nous-token")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := bridge.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	args := map[string]any{"query": "openclaw", "limit": 3}
	res, evidence, err := bridge.CallTool(ctx, "web/search", args)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if evidence != ManagedGatewayEvidenceOK {
		t.Fatalf("Evidence = %q, want %q", evidence, ManagedGatewayEvidenceOK)
	}
	if res.IsError {
		t.Fatalf("res.IsError = true, want false")
	}
	if len(res.Content) != 1 {
		t.Fatalf("res.Content len = %d, want 1", len(res.Content))
	}
	if res.Content[0].Kind != "text" || res.Content[0].Text != "echo ok" {
		t.Errorf("res.Content[0] = %+v, want text/echo ok", res.Content[0])
	}
	if got := srv.LastCallName(); got != "web/search" {
		t.Errorf("server saw tool name %q, want %q", got, "web/search")
	}
	var parsed map[string]any
	if err := json.Unmarshal(srv.LastCallArguments(), &parsed); err != nil {
		t.Fatalf("decode forwarded args: %v", err)
	}
	if parsed["query"] != "openclaw" {
		t.Errorf("forwarded query = %v, want %q", parsed["query"], "openclaw")
	}
	// JSON numbers decode as float64; mirror that here so the assertion
	// stays representation-stable.
	if parsed["limit"] != float64(3) {
		t.Errorf("forwarded limit = %v, want 3", parsed["limit"])
	}
}

// TestManagedGatewayBridge_CancellationPropagatesToCallTool covers the
// timeout/cancellation half of acceptance #2: ctx cancellation must abort
// the in-flight tool call cleanly.
func TestManagedGatewayBridge_CancellationPropagatesToCallTool(t *testing.T) {
	srv := newFakeManagedGatewayServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request, req fakeManagedGatewayRequest) {
		if req.Method == "initialize" {
			writeManagedJSONResult(w, req.ID, `{"protocolVersion":"2024-11-05","capabilities":{}}`)
			return
		}
		// Acceptance: the bridge must honor ctx cancellation rather than
		// blocking on the gateway. The handler waits for the request context
		// to fire so teardown is fast on the server side too.
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	})

	bridge := newTestManagedGatewayBridge(t, "firecrawl", srv, "nous-token")

	initCtx, cancelInit := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelInit()
	if err := bridge.Initialize(initCtx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, evidence, err := bridge.CallTool(ctx, "web/search", map[string]any{"q": "x"})
	if err == nil {
		t.Fatal("CallTool err = nil, want context-related error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrConnectTimeout) {
		t.Fatalf("CallTool err = %v, want errors.Is DeadlineExceeded or ErrConnectTimeout", err)
	}
	if evidence != ManagedGatewayEvidenceToolCallFailed {
		t.Fatalf("Evidence = %q, want %q", evidence, ManagedGatewayEvidenceToolCallFailed)
	}
}

// TestManagedGatewayBridge_AuthRequiredReusesEvidence covers acceptance #3:
// 401 responses must surface as auth_required evidence and must not register
// any half-discovered tools.
func TestManagedGatewayBridge_AuthRequiredReusesEvidence(t *testing.T) {
	srv := newFakeManagedGatewayServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request, req fakeManagedGatewayRequest) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})

	bridge := newTestManagedGatewayBridge(t, "firecrawl", srv, "stale-token")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	disc, err := bridge.Discover(ctx)
	if err == nil {
		t.Fatal("Discover err = nil, want ErrAuthRequired")
	}
	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("Discover err = %v, want errors.Is ErrAuthRequired", err)
	}
	if disc.Evidence != ManagedGatewayEvidenceAuthRequired {
		t.Fatalf("Discover Evidence = %q, want %q", disc.Evidence, ManagedGatewayEvidenceAuthRequired)
	}
	if len(disc.Tools) != 0 {
		t.Fatalf("Discover Tools = %+v, want empty (no half-discovered tools)", disc.Tools)
	}

	_, callEvidence, callErr := bridge.CallTool(ctx, "web/search", map[string]any{"q": "x"})
	if callErr == nil {
		t.Fatal("CallTool err = nil, want ErrAuthRequired")
	}
	if !errors.Is(callErr, ErrAuthRequired) {
		t.Fatalf("CallTool err = %v, want errors.Is ErrAuthRequired", callErr)
	}
	if callEvidence != ManagedGatewayEvidenceAuthRequired {
		t.Fatalf("CallTool Evidence = %q, want %q", callEvidence, ManagedGatewayEvidenceAuthRequired)
	}
}

// TestManagedGatewayBridge_GatewayUnavailableReportsDegradedEvidence covers
// acceptance #3 (the unavailable half): non-auth transport failures surface
// as gateway_unavailable without registering tools.
func TestManagedGatewayBridge_GatewayUnavailableReportsDegradedEvidence(t *testing.T) {
	srv := newFakeManagedGatewayServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request, req fakeManagedGatewayRequest) {
		http.Error(w, "down for maintenance", http.StatusServiceUnavailable)
	})

	bridge := newTestManagedGatewayBridge(t, "firecrawl", srv, "nous-token")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	disc, err := bridge.Discover(ctx)
	if err == nil {
		t.Fatal("Discover err = nil, want non-nil")
	}
	if disc.Evidence != ManagedGatewayEvidenceUnavailable {
		t.Fatalf("Evidence = %q, want %q", disc.Evidence, ManagedGatewayEvidenceUnavailable)
	}
	if len(disc.Tools) != 0 {
		t.Fatalf("Tools = %+v, want empty", disc.Tools)
	}
}

// TestManagedGatewayBridge_SchemaRejectedKeepsBadToolsOut covers acceptance
// #1's degraded path: tools with non-object schemas must land in Rejected,
// must not appear in Tools, and must surface schema_rejected evidence.
func TestManagedGatewayBridge_SchemaRejectedKeepsBadToolsOut(t *testing.T) {
	srv := newFakeManagedGatewayServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request, req fakeManagedGatewayRequest) {
		switch req.Method {
		case "initialize":
			writeManagedJSONResult(w, req.ID, `{"protocolVersion":"2024-11-05","capabilities":{}}`)
		case "tools/list":
			writeManagedJSONResult(w, req.ID,
				`{"tools":[`+
					`{"name":"good","description":"ok","inputSchema":{"type":"object","properties":{}}},`+
					`{"name":"bad","description":"non-object schema","inputSchema":true}`+
					`]}`)
		default:
			t.Errorf("unexpected method %q", req.Method)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	})

	bridge := newTestManagedGatewayBridge(t, "firecrawl", srv, "nous-token")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	disc, err := bridge.Discover(ctx)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if disc.Evidence != ManagedGatewayEvidenceSchemaRejected {
		t.Fatalf("Evidence = %q, want %q", disc.Evidence, ManagedGatewayEvidenceSchemaRejected)
	}
	if len(disc.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1; tools=%+v", len(disc.Tools), disc.Tools)
	}
	if disc.Tools[0].Name != "good" {
		t.Errorf("Tools[0].Name = %q, want %q", disc.Tools[0].Name, "good")
	}
	if len(disc.Rejected) != 1 {
		t.Fatalf("Rejected len = %d, want 1; rejected=%+v", len(disc.Rejected), disc.Rejected)
	}
	rej := disc.Rejected[0]
	if rej.ToolName != "bad" {
		t.Errorf("Rejected[0].ToolName = %q, want %q", rej.ToolName, "bad")
	}
	if rej.Reason != SchemaRejectionReasonInputSchemaNotObject {
		t.Errorf("Rejected[0].Reason = %q, want %q", rej.Reason, SchemaRejectionReasonInputSchemaNotObject)
	}
	if rej.ServerName != "firecrawl" {
		t.Errorf("Rejected[0].ServerName = %q, want %q", rej.ServerName, "firecrawl")
	}
}

// TestManagedGatewayBridge_ToolCallFailureReportsDegradedEvidence covers
// acceptance #2's degraded half: a tools/call response with isError=true
// must surface tool_call_failed evidence with the structured content
// preserved for the caller.
func TestManagedGatewayBridge_ToolCallFailureReportsDegradedEvidence(t *testing.T) {
	srv := newFakeManagedGatewayServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request, req fakeManagedGatewayRequest) {
		switch req.Method {
		case "initialize":
			writeManagedJSONResult(w, req.ID, `{"protocolVersion":"2024-11-05","capabilities":{}}`)
		case "tools/call":
			writeManagedJSONResult(w, req.ID,
				`{"content":[{"type":"text","text":"upstream error"}],"isError":true}`)
		default:
			t.Errorf("unexpected method %q", req.Method)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	})

	bridge := newTestManagedGatewayBridge(t, "firecrawl", srv, "nous-token")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := bridge.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	res, evidence, err := bridge.CallTool(ctx, "web_search", map[string]any{"q": "x"})
	if err != nil {
		t.Fatalf("CallTool err = %v, want nil (the call succeeded; isError lives in the result)", err)
	}
	if evidence != ManagedGatewayEvidenceToolCallFailed {
		t.Fatalf("Evidence = %q, want %q", evidence, ManagedGatewayEvidenceToolCallFailed)
	}
	if !res.IsError {
		t.Fatalf("res.IsError = false, want true")
	}
	if len(res.Content) != 1 || res.Content[0].Text != "upstream error" {
		t.Errorf("res.Content = %+v, want text/upstream error", res.Content)
	}
}

// TestManagedGatewayBridge_RPCErrorReportsToolCallFailed covers tools/call
// failures that arrive as JSON-RPC errors (not structured isError envelopes):
// the bridge must surface tool_call_failed without registering the response
// as a successful call.
func TestManagedGatewayBridge_RPCErrorReportsToolCallFailed(t *testing.T) {
	srv := newFakeManagedGatewayServer(t, func(t *testing.T, w http.ResponseWriter, r *http.Request, req fakeManagedGatewayRequest) {
		switch req.Method {
		case "initialize":
			writeManagedJSONResult(w, req.ID, `{"protocolVersion":"2024-11-05","capabilities":{}}`)
		case "tools/call":
			writeManagedJSONError(w, req.ID, -32601, "method tools/call not implemented")
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	})

	bridge := newTestManagedGatewayBridge(t, "firecrawl", srv, "nous-token")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := bridge.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	_, evidence, err := bridge.CallTool(ctx, "web_search", map[string]any{"q": "x"})
	if err == nil {
		t.Fatal("CallTool err = nil, want non-nil JSON-RPC error")
	}
	if evidence != ManagedGatewayEvidenceToolCallFailed {
		t.Fatalf("Evidence = %q, want %q", evidence, ManagedGatewayEvidenceToolCallFailed)
	}
}

// TestManagedGatewayBridge_RejectsEmptyVendorOrOrigin covers acceptance #4:
// the constructor must refuse half-configured bridges so test code cannot
// accidentally launch a bridge without an origin or vendor name.
func TestManagedGatewayBridge_RejectsEmptyVendorOrOrigin(t *testing.T) {
	if _, err := NewManagedGatewayBridge(ManagedGatewayDefinition{Origin: "https://example.test"}, http.DefaultTransport); err == nil {
		t.Error("NewManagedGatewayBridge with empty vendor returned nil error")
	}
	if _, err := NewManagedGatewayBridge(ManagedGatewayDefinition{Vendor: "firecrawl"}, http.DefaultTransport); err == nil {
		t.Error("NewManagedGatewayBridge with empty origin returned nil error")
	}
}
