package hermes

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// azureFoundryProbeFreezeEnv records and restores AZURE_FOUNDRY_* env so
// the no-mutation contract test can prove DetectAzureFoundry never writes
// configuration. The helper installs sentinel values that the probe must
// not overwrite, then asserts they survive verbatim across the call.
func azureFoundryProbeFreezeEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"AZURE_FOUNDRY_BASE_URL",
		"AZURE_FOUNDRY_API_KEY",
		"AZURE_FOUNDRY_API_MODE",
		"AZURE_FOUNDRY_DEPLOYMENT",
	}
	for _, k := range keys {
		old, had := os.LookupEnv(k)
		t.Setenv(k, "azure-foundry-probe-sentinel:"+k)
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
}

// azureFoundryProbeAssertEnvUnchanged asserts the AZURE_FOUNDRY_* sentinels
// installed by azureFoundryProbeFreezeEnv survived a probe call.
func azureFoundryProbeAssertEnvUnchanged(t *testing.T) {
	t.Helper()
	keys := []string{
		"AZURE_FOUNDRY_BASE_URL",
		"AZURE_FOUNDRY_API_KEY",
		"AZURE_FOUNDRY_API_MODE",
		"AZURE_FOUNDRY_DEPLOYMENT",
	}
	for _, k := range keys {
		want := "azure-foundry-probe-sentinel:" + k
		if got := os.Getenv(k); got != want {
			t.Fatalf("env[%s] = %q after probe, want %q (probe must not mutate config)", k, got, want)
		}
	}
}

// TestAzureFoundryProbe_PathSniffShortCircuitsAnthropic asserts that a
// base URL ending in /anthropic classifies as anthropic_messages without
// any HTTP request being issued. The fake server fails the test if it
// is contacted at all - path sniffing must short-circuit the read model.
func TestAzureFoundryProbe_PathSniffShortCircuitsAnthropic(t *testing.T) {
	azureFoundryProbeFreezeEnv(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		t.Errorf("unexpected HTTP request during path-sniff short-circuit: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	base := srv.URL + "/anthropic"
	res, err := DetectAzureFoundry(context.Background(), &http.Client{}, base, "test-api-key")
	if err != nil {
		t.Fatalf("DetectAzureFoundry returned err = %v", err)
	}
	if res.Transport != AzureTransportAnthropic {
		t.Fatalf("Transport = %q, want %q", res.Transport, AzureTransportAnthropic)
	}
	if res.Reason == "" || !strings.Contains(strings.ToLower(res.Reason), "path") {
		t.Fatalf("Reason = %q, want to mention 'path' (path-sniff evidence)", res.Reason)
	}
	joined := strings.Join(res.Evidence, " | ")
	if !strings.Contains(strings.ToLower(joined), "azure_path_sniff") && !strings.Contains(strings.ToLower(joined), "path_sniff") {
		t.Fatalf("Evidence = %v, want to record path-sniff evidence", res.Evidence)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("HTTP call count = %d, want 0 (path sniff must not issue HTTP)", got)
	}
	azureFoundryProbeAssertEnvUnchanged(t)
}

// TestAzureFoundryProbe_OpenAIModelsClassification asserts that when the
// path sniff is inconclusive and /models returns an OpenAI-shaped catalog,
// DetectAzureFoundry classifies as openai_chat_completions and surfaces
// model IDs in Models and Evidence (advisory only - never persisted).
func TestAzureFoundryProbe_OpenAIModelsClassification(t *testing.T) {
	azureFoundryProbeFreezeEnv(t)
	var modelsCalls, messagesCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			atomic.AddInt32(&modelsCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-5.4","object":"model"},{"id":"o4-mini","object":"model"}]}`)
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

	res, err := DetectAzureFoundry(context.Background(), &http.Client{}, srv.URL, "test-api-key")
	if err != nil {
		t.Fatalf("DetectAzureFoundry returned err = %v", err)
	}
	if res.Transport != AzureTransportOpenAI {
		t.Fatalf("Transport = %q, want %q", res.Transport, AzureTransportOpenAI)
	}
	if len(res.Models) != 2 || res.Models[0] != "gpt-5.4" || res.Models[1] != "o4-mini" {
		t.Fatalf("Models = %v, want [gpt-5.4 o4-mini]", res.Models)
	}
	joined := strings.Join(res.Evidence, " | ")
	if !strings.Contains(joined, "gpt-5.4") || !strings.Contains(joined, "o4-mini") {
		t.Fatalf("Evidence = %v, want advisory model IDs", res.Evidence)
	}
	if got := atomic.LoadInt32(&modelsCalls); got != 1 {
		t.Fatalf("/models call count = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&messagesCalls); got != 0 {
		t.Fatalf("/v1/messages call count = %d, want 0", got)
	}
	azureFoundryProbeAssertEnvUnchanged(t)
}

// TestAzureFoundryProbe_AnthropicFallback asserts that when /models fails
// and /v1/messages returns a 4xx with an Anthropic-shaped error body, the
// read model classifies as anthropic_messages with explicit probe evidence.
func TestAzureFoundryProbe_AnthropicFallback(t *testing.T) {
	azureFoundryProbeFreezeEnv(t)
	var modelsCalls, messagesCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			atomic.AddInt32(&modelsCalls, 1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
		case "/v1/messages":
			atomic.AddInt32(&messagesCalls, 1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"deployment 'probe' not found in messages catalog"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	res, err := DetectAzureFoundry(context.Background(), &http.Client{}, srv.URL, "test-api-key")
	if err != nil {
		t.Fatalf("DetectAzureFoundry returned err = %v", err)
	}
	if res.Transport != AzureTransportAnthropic {
		t.Fatalf("Transport = %q, want %q", res.Transport, AzureTransportAnthropic)
	}
	joined := strings.Join(res.Evidence, " | ")
	if !strings.Contains(joined, "/v1/messages") {
		t.Fatalf("Evidence = %v, want anthropic probe URL", res.Evidence)
	}
	if !strings.Contains(joined, "/models") {
		t.Fatalf("Evidence = %v, want /models attempt recorded as evidence", res.Evidence)
	}
	if got := atomic.LoadInt32(&modelsCalls); got != 1 {
		t.Fatalf("/models call count = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&messagesCalls); got != 1 {
		t.Fatalf("/v1/messages call count = %d, want 1", got)
	}
	azureFoundryProbeAssertEnvUnchanged(t)
}

// TestAzureFoundryProbe_ManualRequiredOnTotalFailure asserts that when
// neither probe yields a recognisable shape, DetectAzureFoundry returns
// Transport=unknown with Reason=manual_required and a nil error - the
// wizard must keep manual api_mode entry available.
func TestAzureFoundryProbe_ManualRequiredOnTotalFailure(t *testing.T) {
	azureFoundryProbeFreezeEnv(t)
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

	res, err := DetectAzureFoundry(context.Background(), &http.Client{}, srv.URL, "test-api-key")
	if err != nil {
		t.Fatalf("DetectAzureFoundry returned err = %v (manual-required must not be a fatal error)", err)
	}
	if res.Transport != AzureTransportUnknown {
		t.Fatalf("Transport = %q, want %q", res.Transport, AzureTransportUnknown)
	}
	if res.Reason != "manual_required" {
		t.Fatalf("Reason = %q, want %q", res.Reason, "manual_required")
	}
	if len(res.Models) != 0 {
		t.Fatalf("Models = %v, want empty (no advisory model IDs on failure)", res.Models)
	}
	azureFoundryProbeAssertEnvUnchanged(t)
}

// TestAzureFoundryProbe_RespectsContextCancel asserts that mid-probe
// context cancellation surfaces ctx.Err to the caller rather than a
// swallowed transport error. The fake server cancels mid-flight.
func TestAzureFoundryProbe_RespectsContextCancel(t *testing.T) {
	azureFoundryProbeFreezeEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}))
	defer srv.Close()

	_, err := DetectAzureFoundry(ctx, &http.Client{}, srv.URL, "test-api-key")
	if err == nil {
		t.Fatalf("DetectAzureFoundry: err = nil, want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DetectAzureFoundry: err = %v, want errors.Is(err, context.Canceled)", err)
	}
	azureFoundryProbeAssertEnvUnchanged(t)
}
