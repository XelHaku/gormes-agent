package hermes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReasoningContentEchoDetectsThinkingProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		baseURL  string
		want     bool
	}{
		{
			name:     "deepseek provider name",
			provider: "DeepSeek",
			model:    "fixture-model",
			baseURL:  "https://proxy.example.test",
			want:     true,
		},
		{
			name:     "deepseek model substring",
			provider: "custom",
			model:    "accounts/fireworks/models/deepseek-v4-flash",
			baseURL:  "https://proxy.example.test",
			want:     true,
		},
		{
			name:     "deepseek api host",
			provider: "custom",
			model:    "aliased-thinking-model",
			baseURL:  "https://api.deepseek.com/v1",
			want:     true,
		},
		{
			name:     "kimi provider name",
			provider: "kimi-coding",
			model:    "kimi-k2",
			baseURL:  "https://proxy.example.test",
			want:     true,
		},
		{
			name:     "moonshot api host",
			provider: "custom",
			model:    "kimi-k2",
			baseURL:  "https://api.moonshot.ai/v1",
			want:     true,
		},
		{
			name:     "moonshot model routed through openrouter is not direct moonshot",
			provider: "openrouter",
			model:    "moonshotai/kimi-k2",
			baseURL:  "https://openrouter.ai/api/v1",
			want:     false,
		},
		{
			name:     "plain openai-compatible provider",
			provider: "openrouter",
			model:    "anthropic/claude-sonnet-4.6",
			baseURL:  "https://openrouter.ai/api/v1",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := openAICompatibleRequiresReasoningEcho(tt.provider, tt.model, tt.baseURL)
			if got != tt.want {
				t.Fatalf("openAICompatibleRequiresReasoningEcho(%q, %q, %q) = %v, want %v", tt.provider, tt.model, tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestReasoningContentEchoPadsAssistantToolCallReplayForThinkingProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		baseURL  string
	}{
		{
			name:     "deepseek provider name",
			provider: "deepseek",
			model:    "fixture-model",
			baseURL:  "https://proxy.example.test",
		},
		{
			name:     "deepseek model substring",
			provider: "custom",
			model:    "deepseek-v4-pro",
			baseURL:  "https://proxy.example.test",
		},
		{
			name:     "deepseek host",
			provider: "custom",
			model:    "aliased-thinking-model",
			baseURL:  "https://api.deepseek.com",
		},
		{
			name:     "kimi provider name",
			provider: "kimi-coding",
			model:    "kimi-k2",
			baseURL:  "https://proxy.example.test",
		},
		{
			name:     "moonshot host",
			provider: "custom",
			model:    "kimi-k2",
			baseURL:  "https://api.moonshot.ai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := captureOpenAICompatibleMessages(t, &httpClient{
				baseURL:  tt.baseURL,
				provider: tt.provider,
				http:     captureRequestHTTPClient(t),
			}, ChatRequest{
				Model:  tt.model,
				Stream: true,
				Messages: []Message{
					{Role: "user", Content: "run terminal"},
					{
						Role:    "assistant",
						Content: "Calling terminal.",
						ToolCalls: []ToolCall{{
							ID:        "call_terminal",
							Name:      "terminal",
							Arguments: json.RawMessage(`{"cmd":"pwd"}`),
						}},
					},
					{Role: "assistant", Content: "No tool call here."},
				},
			})

			toolAssistant := messages[1]
			if got, ok := toolAssistant["reasoning_content"].(string); !ok || got != "" {
				t.Fatalf("assistant tool-call reasoning_content = %v (present=%v), want empty string", toolAssistant["reasoning_content"], ok)
			}
			if got := toolAssistant["content"]; got != "Calling terminal." {
				t.Fatalf("assistant content = %v, want ordinary content preserved", got)
			}
			if _, ok := messages[2]["reasoning_content"]; ok {
				t.Fatalf("non-tool assistant got reasoning_content padding: %+v", messages[2])
			}
		})
	}
}

func TestReasoningContentEchoPreservesExplicitReasoningFields(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		want    string
	}{
		{
			name: "explicit reasoning_content",
			message: Message{
				Role:             "assistant",
				Content:          "Calling terminal.",
				ReasoningContent: stringPtr("vendor trace"),
				ToolCalls: []ToolCall{{
					ID:        "call_terminal",
					Name:      "terminal",
					Arguments: json.RawMessage(`{"cmd":"pwd"}`),
				}},
			},
			want: "vendor trace",
		},
		{
			name: "normalized reasoning",
			message: Message{
				Role:    "assistant",
				Content: "Calling terminal.",
				Reasoning: &ReasoningContent{
					Text: "normalized trace",
				},
				ToolCalls: []ToolCall{{
					ID:        "call_terminal",
					Name:      "terminal",
					Arguments: json.RawMessage(`{"cmd":"pwd"}`),
				}},
			},
			want: "normalized trace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := captureOpenAICompatibleMessages(t, &httpClient{
				baseURL:  "https://proxy.example.test",
				provider: "deepseek",
				http:     captureRequestHTTPClient(t),
			}, ChatRequest{
				Model:    "deepseek-v4-flash",
				Stream:   true,
				Messages: []Message{{Role: "user", Content: "run terminal"}, tt.message},
			})

			assistant := messages[1]
			if got := assistant["reasoning_content"]; got != tt.want {
				t.Fatalf("reasoning_content = %v, want %q", got, tt.want)
			}
			if got := assistant["content"]; got != "Calling terminal." {
				t.Fatalf("assistant content = %v, want ordinary content preserved", got)
			}
			if _, ok := assistant["reasoning"]; ok {
				t.Fatalf("assistant request leaked storage-only reasoning field: %+v", assistant)
			}
		})
	}
}

func TestReasoningContentEchoLeavesNonThinkingProvidersUntouched(t *testing.T) {
	messages := captureOpenAICompatibleMessages(t, &httpClient{
		baseURL:  "https://openrouter.ai",
		provider: "openrouter",
		http:     captureRequestHTTPClient(t),
	}, ChatRequest{
		Model:  "anthropic/claude-sonnet-4.6",
		Stream: true,
		Messages: []Message{
			{Role: "user", Content: "run terminal"},
			{
				Role:             "assistant",
				Content:          "Calling terminal.",
				ReasoningContent: stringPtr("stored trace for another provider"),
				ToolCalls: []ToolCall{{
					ID:        "call_terminal",
					Name:      "terminal",
					Arguments: json.RawMessage(`{"cmd":"pwd"}`),
				}},
			},
		},
	})

	if _, ok := messages[1]["reasoning_content"]; ok {
		t.Fatalf("non-thinking provider got reasoning_content padding: %+v", messages[1])
	}
	if _, ok := messages[1]["reasoning"]; ok {
		t.Fatalf("non-thinking provider got storage-only reasoning field: %+v", messages[1])
	}
}

func TestReasoningContentEchoProviderStatusExplainsPaddingAndRepair(t *testing.T) {
	thinking := ProviderStatusOf(&httpClient{
		baseURL:  "https://api.deepseek.com",
		provider: "custom",
		http:     http.DefaultClient,
	})
	if !thinking.Capabilities.ReasoningEcho.Available {
		t.Fatalf("ReasoningEcho.Available = false, want true for DeepSeek host")
	}
	reason := thinking.Capabilities.ReasoningEcho.Reason
	for _, want := range []string{"reasoning_content", "assistant tool-call", "repaired"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("ReasoningEcho.Reason = %q, want it to mention %q", reason, want)
		}
	}

	generic := ProviderStatusOf(NewHTTPClient("https://openrouter.ai", ""))
	if generic.Capabilities.ReasoningEcho.Available {
		t.Fatalf("generic ReasoningEcho.Available = true, want false")
	}
	if generic.Capabilities.ReasoningEcho.Reason == "" {
		t.Fatal("generic ReasoningEcho.Reason is empty, want visible status")
	}
}

func captureOpenAICompatibleMessages(t *testing.T, client Client, req ChatRequest) []map[string]any {
	t.Helper()
	stream, err := client.OpenStream(context.Background(), req)
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	defer stream.Close()

	capture, ok := client.(*httpClient)
	if !ok {
		t.Fatalf("client type = %T, want *httpClient", client)
	}
	raw := capturedRequestBody(t, capture.http)
	var body struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode request body: %v\n%s", err, raw)
	}
	return body.Messages
}

func captureRequestHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	transport := &captureRoundTripper{}
	transport.roundTrip = func(req *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		transport.captured = raw
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")),
			Request:    req,
		}, nil
	}
	return &http.Client{Transport: transport}
}

func capturedRequestBody(t *testing.T, client *http.Client) []byte {
	t.Helper()
	transport, ok := client.Transport.(capturingTransport)
	if !ok {
		t.Fatalf("transport type = %T, want capturingTransport", client.Transport)
	}
	return transport.CapturedBody()
}

type capturingTransport interface {
	http.RoundTripper
	CapturedBody() []byte
}

type captureRoundTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
	captured  []byte
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return c.roundTrip(req)
}

func (c *captureRoundTripper) CapturedBody() []byte {
	return c.captured
}

func stringPtr(value string) *string {
	return &value
}
