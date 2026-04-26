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
	"sync/atomic"
	"testing"
	"time"
)

// recordingTransport wraps an inner http.RoundTripper and records every
// request that flows through it. Tests use it to prove the client honors a
// custom RoundTripper without ever opening a socket outside httptest.
type recordingTransport struct {
	inner    http.RoundTripper
	requests atomic.Int64
	lastReq  atomic.Pointer[http.Request]
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.requests.Add(1)
	r.lastReq.Store(req)
	return r.inner.RoundTrip(req)
}

// httpRequest is the partial JSON-RPC envelope the in-process httptest
// handler decodes from the client.
type httpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
}

func newTestHTTPServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func newTestHTTPClient(t *testing.T, def MCPServerDefinition, transport http.RoundTripper) *HTTPClient {
	t.Helper()
	client, err := NewHTTPClient(def, HTTPClientOpts{Transport: transport})
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	if client == nil {
		t.Fatal("NewHTTPClient returned nil client")
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestHTTPClient_InitializeSendsAuthHeadersFromConfig(t *testing.T) {
	var gotAuth string
	srv := newTestHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"fake","version":"0"}}}`)
	})

	def := MCPServerDefinition{
		Name:      "fake-http",
		Enabled:   true,
		Transport: MCPTransportHTTP,
		URL:       srv.URL,
		Headers:   map[string]string{"Authorization": "Bearer test"},
	}
	client := newTestHTTPClient(t, def, srv.Client().Transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if gotAuth != "Bearer test" {
		t.Errorf("Authorization header = %q; want %q", gotAuth, "Bearer test")
	}
}

func TestHTTPClient_ListToolsParsesResponse(t *testing.T) {
	const schemaA = `{"type":"object","properties":{"x":{"type":"string"}}}`
	srv := newTestHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		// Pull the request id out of the JSON-RPC envelope so we can echo it.
		var req httpRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode body %q: %v", string(body), err)
			return
		}
		if req.Method != "tools/list" {
			t.Errorf("unexpected method %q; want tools/list", req.Method)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w,
			`{"jsonrpc":"2.0","id":`+strconv.FormatInt(req.ID, 10)+
				`,"result":{"tools":[`+
				`{"name":"alpha","description":"first tool","inputSchema":`+schemaA+`},`+
				`{"name":"beta","description":"second tool"}`+
				`]}}`)
	})

	def := MCPServerDefinition{
		Name:      "fake-http",
		Enabled:   true,
		Transport: MCPTransportHTTP,
		URL:       srv.URL,
	}
	client := newTestHTTPClient(t, def, srv.Client().Transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("got %d tools; want 2", len(tools))
	}
	if tools[0].Name != "alpha" {
		t.Errorf("tools[0].Name = %q; want alpha", tools[0].Name)
	}
	if tools[0].Description != "first tool" {
		t.Errorf("tools[0].Description = %q; want %q", tools[0].Description, "first tool")
	}
	if string(tools[0].InputSchema) != schemaA {
		t.Errorf("tools[0].InputSchema = %s; want %s", string(tools[0].InputSchema), schemaA)
	}
	if tools[1].Name != "beta" {
		t.Errorf("tools[1].Name = %q; want beta", tools[1].Name)
	}
	if tools[1].Description != "second tool" {
		t.Errorf("tools[1].Description = %q; want %q", tools[1].Description, "second tool")
	}
	if len(tools[1].InputSchema) != 0 {
		t.Errorf("tools[1].InputSchema = %s; want empty", string(tools[1].InputSchema))
	}
}

func TestHTTPClient_401ReturnsErrAuthRequired(t *testing.T) {
	srv := newTestHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"mcp\"")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})

	def := MCPServerDefinition{
		Name:      "fake-http",
		Enabled:   true,
		Transport: MCPTransportHTTP,
		URL:       srv.URL,
	}
	client := newTestHTTPClient(t, def, srv.Client().Transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.ListTools(ctx)
	if err == nil {
		t.Fatal("ListTools err = nil; want ErrAuthRequired")
	}
	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("ListTools err = %v; want errors.Is ErrAuthRequired", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("ListTools err = %q; want it to mention status 401", err.Error())
	}
}

func TestHTTPClient_ConnectTimeoutHonoredViaContext(t *testing.T) {
	srv := newTestHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Acceptance contract: server sleeps 1s while the client's ctx
		// deadline is 10ms. We honor r.Context() so clean teardown does not
		// have to wait the full second after the client has already errored.
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	})

	def := MCPServerDefinition{
		Name:      "fake-http",
		Enabled:   true,
		Transport: MCPTransportHTTP,
		URL:       srv.URL,
	}
	client := newTestHTTPClient(t, def, srv.Client().Transport)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.ListTools(ctx)
	if err == nil {
		t.Fatal("ListTools err = nil; want context.DeadlineExceeded or ErrConnectTimeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrConnectTimeout) {
		t.Fatalf("ListTools err = %v; want errors.Is DeadlineExceeded or ErrConnectTimeout", err)
	}
}

func TestHTTPClient_RoundTripperIsInjectable(t *testing.T) {
	srv := newTestHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{}}}`)
	})

	rec := &recordingTransport{inner: srv.Client().Transport}

	def := MCPServerDefinition{
		Name:      "fake-http",
		Enabled:   true,
		Transport: MCPTransportHTTP,
		URL:       srv.URL,
	}
	client := newTestHTTPClient(t, def, rec)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if got := rec.requests.Load(); got != 1 {
		t.Errorf("recordingTransport saw %d requests; want 1", got)
	}
	if last := rec.lastReq.Load(); last == nil {
		t.Error("recordingTransport.lastReq = nil; want a captured request")
	} else if last.URL == nil || !strings.HasPrefix(last.URL.String(), srv.URL) {
		t.Errorf("recordingTransport.lastReq.URL = %v; want it to target the test server", last.URL)
	}
}
