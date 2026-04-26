package hermes

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestProbeAzureFoundry_OpenAIModels200WithList verifies that a /models
// 200 response carrying an OpenAI-shaped data list yields
// AzureTransportOpenAI with the model IDs surfaced in both Models and
// Evidence. The Anthropic probe must never fire on a successful /models hit.
func TestProbeAzureFoundry_OpenAIModels200WithList(t *testing.T) {
	var modelsCalls, messagesCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			atomic.AddInt32(&modelsCalls, 1)
			if r.Method != http.MethodGet {
				t.Errorf("/models method = %q, want GET", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-4.1","object":"model"},{"id":"o3-mini","object":"model"}]}`)
		case "/v1/messages":
			atomic.AddInt32(&messagesCalls, 1)
			t.Errorf("/v1/messages must not be called when /models classifies as OpenAI")
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	res, err := ProbeAzureFoundry(context.Background(), &http.Client{}, srv.URL, "test-api-key")
	if err != nil {
		t.Fatalf("ProbeAzureFoundry returned err = %v", err)
	}
	if res.Transport != AzureTransportOpenAI {
		t.Fatalf("Transport = %q, want %q", res.Transport, AzureTransportOpenAI)
	}
	if len(res.Models) != 2 || res.Models[0] != "gpt-4.1" || res.Models[1] != "o3-mini" {
		t.Fatalf("Models = %v, want [gpt-4.1 o3-mini]", res.Models)
	}
	joined := strings.Join(res.Evidence, " | ")
	if !strings.Contains(joined, "gpt-4.1") || !strings.Contains(joined, "o3-mini") {
		t.Fatalf("Evidence = %v, want to include model IDs", res.Evidence)
	}
	if got := atomic.LoadInt32(&modelsCalls); got != 1 {
		t.Fatalf("/models call count = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&messagesCalls); got != 0 {
		t.Fatalf("/v1/messages call count = %d, want 0", got)
	}
}

// TestProbeAzureFoundry_OpenAIModels200EmptyList verifies that a /models
// 200 response carrying an OpenAI-shaped but empty data list still
// classifies as OpenAI (Azure resources frequently expose the catalog
// schema without listing any deployments via API key auth).
func TestProbeAzureFoundry_OpenAIModels200EmptyList(t *testing.T) {
	var messagesCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"object":"list","data":[]}`)
		case "/v1/messages":
			atomic.AddInt32(&messagesCalls, 1)
			t.Errorf("/v1/messages must not be called when /models returns OpenAI shape")
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	res, err := ProbeAzureFoundry(context.Background(), &http.Client{}, srv.URL, "test-api-key")
	if err != nil {
		t.Fatalf("ProbeAzureFoundry returned err = %v", err)
	}
	if res.Transport != AzureTransportOpenAI {
		t.Fatalf("Transport = %q, want %q", res.Transport, AzureTransportOpenAI)
	}
	if len(res.Models) != 0 {
		t.Fatalf("Models = %v, want empty slice", res.Models)
	}
	if !strings.Contains(res.Reason, "shape OK, empty list") {
		t.Fatalf("Reason = %q, want to contain 'shape OK, empty list'", res.Reason)
	}
	if got := atomic.LoadInt32(&messagesCalls); got != 0 {
		t.Fatalf("/v1/messages call count = %d, want 0", got)
	}
}

// TestProbeAzureFoundry_AnthropicMessages400ValidShape verifies that when
// /models 404s but /v1/messages returns a 4xx whose body mentions
// 'messages' or 'model', the classification falls through to
// AzureTransportAnthropic. The probe must POST a zero-token payload that
// includes both the 'model' and 'messages' fields.
func TestProbeAzureFoundry_AnthropicMessages400ValidShape(t *testing.T) {
	var modelsCalls, messagesCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			atomic.AddInt32(&modelsCalls, 1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found"}`)
		case "/v1/messages":
			atomic.AddInt32(&messagesCalls, 1)
			if r.Method != http.MethodPost {
				t.Errorf("/v1/messages method = %q, want POST", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"model"`) || !strings.Contains(string(body), `"messages"`) {
				t.Errorf("/v1/messages payload = %q, want fields 'model' and 'messages'", string(body))
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"deployment 'probe' not found in messages catalog"}}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	res, err := ProbeAzureFoundry(context.Background(), &http.Client{}, srv.URL, "test-api-key")
	if err != nil {
		t.Fatalf("ProbeAzureFoundry returned err = %v", err)
	}
	if res.Transport != AzureTransportAnthropic {
		t.Fatalf("Transport = %q, want %q", res.Transport, AzureTransportAnthropic)
	}
	if got := atomic.LoadInt32(&modelsCalls); got != 1 {
		t.Fatalf("/models call count = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&messagesCalls); got != 1 {
		t.Fatalf("/v1/messages call count = %d, want 1", got)
	}
}

// TestProbeAzureFoundry_BothFailReturnUnknown verifies that when neither
// probe yields a recognisable shape, the helper falls back to
// AzureTransportUnknown with Reason="manual_required" - the wizard's
// signal to ask the operator for an api_mode value.
func TestProbeAzureFoundry_BothFailReturnUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `<html>not found</html>`)
		case "/v1/messages":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `<html>not found</html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	res, err := ProbeAzureFoundry(context.Background(), &http.Client{}, srv.URL, "test-api-key")
	if err != nil {
		t.Fatalf("ProbeAzureFoundry returned err = %v", err)
	}
	if res.Transport != AzureTransportUnknown {
		t.Fatalf("Transport = %q, want %q", res.Transport, AzureTransportUnknown)
	}
	if res.Reason != "manual_required" {
		t.Fatalf("Reason = %q, want %q", res.Reason, "manual_required")
	}
}

// TestProbeAzureFoundry_RespectsContextCancel verifies that cancelling
// the caller's context mid-probe surfaces ctx.Err() rather than a swallowed
// transport error. The fake server cancels the caller after the request
// arrives, then waits for the request context to drain.
func TestProbeAzureFoundry_RespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}))
	defer srv.Close()

	_, err := ProbeAzureFoundry(ctx, &http.Client{}, srv.URL, "test-api-key")
	if err == nil {
		t.Fatalf("ProbeAzureFoundry: err = nil, want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ProbeAzureFoundry: err = %v, want errors.Is(err, context.Canceled)", err)
	}
}
