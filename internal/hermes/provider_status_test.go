package hermes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderStatusOfReportsCacheRateAndBudgetCapabilities(t *testing.T) {
	tests := []struct {
		name                 string
		client               Client
		wantProvider         string
		wantRuntime          string
		wantPromptCache      bool
		wantPromptCacheCause string
	}{
		{
			name:                 "openai compatible",
			client:               NewHTTPClient("http://example.test", ""),
			wantProvider:         "openai_compatible",
			wantRuntime:          "chat_completions",
			wantPromptCache:      false,
			wantPromptCacheCause: "cache_control stripped",
		},
		{
			name:                 "anthropic",
			client:               NewAnthropicClient("http://example.test", "sk-ant-api-test"),
			wantProvider:         "anthropic",
			wantRuntime:          "anthropic_messages",
			wantPromptCache:      true,
			wantPromptCacheCause: "cache_control supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProviderStatusOf(tt.client)
			if got.Provider != tt.wantProvider {
				t.Fatalf("Provider = %q, want %q", got.Provider, tt.wantProvider)
			}
			if got.Runtime != tt.wantRuntime {
				t.Fatalf("Runtime = %q, want %q", got.Runtime, tt.wantRuntime)
			}
			if got.Capabilities.PromptCache.Available != tt.wantPromptCache {
				t.Fatalf("PromptCache.Available = %v, want %v", got.Capabilities.PromptCache.Available, tt.wantPromptCache)
			}
			if !strings.Contains(got.Capabilities.PromptCache.Reason, tt.wantPromptCacheCause) {
				t.Fatalf("PromptCache.Reason = %q, want it to mention %q", got.Capabilities.PromptCache.Reason, tt.wantPromptCacheCause)
			}
			assertUnavailableCapability(t, "RateGuard", got.Capabilities.RateGuard)
			assertUnavailableCapability(t, "BudgetTelemetry", got.Capabilities.BudgetTelemetry)
		})
	}
}

func TestOpenAICompatibleCacheControlUnsupportedIsVisibleAndStripped(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != defaultChatCompletionsPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, defaultChatCompletionsPath)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		captured = raw
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "")
	status := ProviderStatusOf(client)
	if status.Capabilities.PromptCache.Available {
		t.Fatal("PromptCache.Available = true, want unsupported for OpenAI-compatible adapter")
	}
	if !strings.Contains(status.Capabilities.PromptCache.Reason, "cache_control stripped") {
		t.Fatalf("PromptCache.Reason = %q, want visible stripped-cache path", status.Capabilities.PromptCache.Reason)
	}

	stream, err := client.OpenStream(context.Background(), ChatRequest{
		Model:  "fixture-model",
		Stream: true,
		Messages: []Message{
			{Role: "system", Content: "stable system", CacheControl: &CacheControl{Type: "ephemeral"}},
			{Role: "user", Content: "hello", CacheControl: &CacheControl{Type: "ephemeral"}},
		},
	})
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	defer stream.Close()

	if bytes.Contains(captured, []byte("cache_control")) {
		t.Fatalf("request body contains unsupported cache_control metadata: %s", captured)
	}
}

func assertUnavailableCapability(t *testing.T, name string, got CapabilityStatus) {
	t.Helper()
	if got.Available {
		t.Fatalf("%s.Available = true, want unavailable until implementation lands", name)
	}
	if got.Reason == "" {
		t.Fatalf("%s.Reason is empty, want visible degradation reason", name)
	}
}
