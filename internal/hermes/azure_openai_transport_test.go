package hermes

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAzureOpenAIBaseURLPreservesAPIVersionQueryOnChatCompletions(t *testing.T) {
	var capturedPath string
	var capturedQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query()
		writeAzureOpenAISSE(t, w)
	}))
	defer srv.Close()

	client := NewHTTPClientWithProvider(srv.URL+"?api-version=2025-04-01-preview&trace=fixture", "azure-key", "azure-openai")
	stream, err := client.OpenStream(context.Background(), ChatRequest{
		Model:  "gpt-4.1",
		Stream: true,
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	defer stream.Close()

	if capturedPath != defaultChatCompletionsPath {
		t.Fatalf("path = %q, want %q", capturedPath, defaultChatCompletionsPath)
	}
	if got := capturedQuery.Get("api-version"); got != "2025-04-01-preview" {
		t.Fatalf("api-version query = %q, want 2025-04-01-preview", got)
	}
	if got := capturedQuery.Get("trace"); got != "fixture" {
		t.Fatalf("trace query = %q, want fixture", got)
	}
}

func TestAzureOpenAIGPT5AndOSeriesUseChatCompletionsMaxCompletionTokens(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{name: "gpt5", model: "gpt-5.4-mini"},
		{name: "o-series", model: "o3-mini"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured azureOpenAICapturedRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				captured = azureOpenAICapturedRequest{
					Path:  r.URL.Path,
					Query: r.URL.Query(),
					Body:  append([]byte(nil), raw...),
				}
				writeAzureOpenAISSE(t, w)
			}))
			defer srv.Close()

			client := newAzureOpenAIHttptestClient(t, srv.URL, "http://my-resource.openai.azure.com?api-version=2025-04-01-preview")
			stream, err := client.OpenStream(context.Background(), ChatRequest{
				Model:     tt.model,
				MaxTokens: 4096,
				Stream:    true,
				Messages: []Message{
					{Role: "user", Content: "hello"},
				},
			})
			if err != nil {
				t.Fatalf("OpenStream() error = %v", err)
			}
			defer stream.Close()

			if captured.Path != defaultChatCompletionsPath {
				t.Fatalf("path = %q, want %q", captured.Path, defaultChatCompletionsPath)
			}
			if got := captured.Query.Get("api-version"); got != "2025-04-01-preview" {
				t.Fatalf("api-version query = %q, want 2025-04-01-preview", got)
			}
			body := decodeJSONMap(t, captured.Body)
			if got := body["model"]; got != tt.model {
				t.Fatalf("model = %#v, want %q in body %s", got, tt.model, captured.Body)
			}
			if _, ok := body["messages"]; !ok {
				t.Fatalf("chat completions request missing messages; body may have used Responses shape: %s", captured.Body)
			}
			if _, ok := body["input"]; ok {
				t.Fatalf("chat completions request unexpectedly contains Responses input: %s", captured.Body)
			}
			if _, ok := body["max_tokens"]; ok {
				t.Fatalf("Azure %s request emitted max_tokens instead of max_completion_tokens: %s", tt.model, captured.Body)
			}
			if got := body["max_completion_tokens"]; got != float64(4096) {
				t.Fatalf("max_completion_tokens = %#v, want 4096 in body %s", got, captured.Body)
			}

			status := ProviderStatusOf(client)
			if status.Runtime != "chat_completions" {
				t.Fatalf("Runtime = %q, want chat_completions", status.Runtime)
			}
		})
	}
}

func TestAzureOpenAIProviderStatusIncludesTransportEvidence(t *testing.T) {
	client := &httpClient{
		baseURL:  "https://my-resource.openai.azure.com?api-version=2025-04-01-preview",
		provider: "openai",
		http:     &http.Client{},
	}
	status := ProviderStatusOf(client)
	evidence := strings.Join([]string{
		status.Capabilities.PromptCache.Reason,
		status.Capabilities.ReasoningEcho.Reason,
		status.Capabilities.RateGuard.Reason,
		status.Capabilities.BudgetTelemetry.Reason,
	}, "\n")
	for _, want := range []string{"azure_query_preserved", "azure_chat_completions"} {
		if !strings.Contains(evidence, want) {
			t.Fatalf("ProviderStatus evidence = %q, want %s", evidence, want)
		}
	}
}

func TestAzureOpenAINonAzureGenericGPT5KeepsMaxTokensFirstRequest(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		captured = append([]byte(nil), raw...)
		writeAzureOpenAISSE(t, w)
	}))
	defer srv.Close()

	client := NewHTTPClientWithProvider(srv.URL, "", "openrouter")
	stream, err := client.OpenStream(context.Background(), ChatRequest{
		Model:     "gpt-5.4-mini",
		MaxTokens: 4096,
		Stream:    true,
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	defer stream.Close()

	body := decodeJSONMap(t, captured)
	if got := body["max_tokens"]; got != float64(4096) {
		t.Fatalf("generic max_tokens = %#v, want 4096 in body %s", got, captured)
	}
	if _, ok := body["max_completion_tokens"]; ok {
		t.Fatalf("generic provider unexpectedly emitted max_completion_tokens: %s", captured)
	}
}

type azureOpenAICapturedRequest struct {
	Path  string
	Query url.Values
	Body  []byte
}

func newAzureOpenAIHttptestClient(t *testing.T, serverURL, rawBaseURL string) *httpClient {
	t.Helper()
	target, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse httptest URL: %v", err)
	}
	return &httpClient{
		baseURL:  rawBaseURL,
		apiKey:   "azure-key",
		provider: "openai",
		http: &http.Client{
			Transport: azureOpenAIRewriteTransport{
				target: target,
				base:   http.DefaultTransport,
			},
		},
	}
}

type azureOpenAIRewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t azureOpenAIRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = t.target.Scheme
	cloned.URL.Host = t.target.Host
	cloned.Host = req.URL.Host
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}

func writeAzureOpenAISSE(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	bw := bufio.NewWriter(w)
	_, _ = fmt.Fprint(bw, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
	_, _ = fmt.Fprint(bw, "data: [DONE]\n\n")
	if err := bw.Flush(); err != nil {
		t.Fatalf("flush SSE fixture: %v", err)
	}
}
