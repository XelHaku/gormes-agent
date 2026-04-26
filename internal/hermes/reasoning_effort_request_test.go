package hermes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReasoningEffortRequestCarriesValidatedValues(t *testing.T) {
	tests := []string{"none", "minimal", "low", "medium", "high", "xhigh"}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			effort := ReasoningEffort(value)
			body := openAICompatibleReasoningRequestBody(t, ChatRequest{
				Model:           "reasoning-model",
				Messages:        []Message{{Role: "user", Content: "think"}},
				ReasoningEffort: &effort,
			})

			got, ok := body["reasoning_effort"].(string)
			if !ok {
				t.Fatalf("reasoning_effort missing or not a string in request body: %#v", body)
			}
			if got != value {
				t.Fatalf("reasoning_effort = %q, want %q", got, value)
			}

			evidence := ResolveReasoningEffort(value, ReasoningEffortSourceTurnOverride, chatCompletionsReasoningStatus())
			if !evidence.Forwarded {
				t.Fatalf("Forwarded = false, want true for %q: %+v", value, evidence)
			}
			if evidence.Effort != effort {
				t.Fatalf("Effort = %q, want %q", evidence.Effort, effort)
			}
		})
	}
}

func TestReasoningEffortRequestOmitsAbsentDefault(t *testing.T) {
	body := openAICompatibleReasoningRequestBody(t, ChatRequest{
		Model:    "reasoning-model",
		Messages: []Message{{Role: "user", Content: "default"}},
	})

	if _, ok := body["reasoning_effort"]; ok {
		t.Fatalf("reasoning_effort present for absent request field: %#v", body)
	}

	evidence := ResolveReasoningEffort("", ReasoningEffortSourceConfigDefault, chatCompletionsReasoningStatus())
	if evidence.State != ReasoningEffortStateDefault {
		t.Fatalf("State = %q, want %q", evidence.State, ReasoningEffortStateDefault)
	}
	if evidence.Forwarded {
		t.Fatalf("Forwarded = true, want false for provider-default reasoning: %+v", evidence)
	}
}

func TestReasoningEffortRequestRejectsInvalidValues(t *testing.T) {
	effort := ReasoningEffort("max")
	client := &httpClient{baseURL: "http://example.test", provider: "openai_compatible"}
	_, _, err := client.buildOpenAICompatibleChatRequestBody(ChatRequest{
		Model:           "reasoning-model",
		Messages:        []Message{{Role: "user", Content: "bad"}},
		ReasoningEffort: &effort,
	})
	if err == nil {
		t.Fatal("buildOpenAICompatibleChatRequestBody() error = nil, want invalid reasoning_effort error")
	}
	if !strings.Contains(err.Error(), "invalid reasoning_effort") {
		t.Fatalf("error = %q, want invalid reasoning_effort evidence", err)
	}

	evidence := ResolveReasoningEffort("max", ReasoningEffortSourceTurnOverride, chatCompletionsReasoningStatus())
	if evidence.State != ReasoningEffortStateInvalid {
		t.Fatalf("State = %q, want %q", evidence.State, ReasoningEffortStateInvalid)
	}
	if evidence.Forwarded {
		t.Fatalf("Forwarded = true, want false for invalid reasoning effort: %+v", evidence)
	}
}

func TestReasoningEffortRequestReportsUnsupportedProvider(t *testing.T) {
	evidence := ResolveReasoningEffort("high", ReasoningEffortSourceTurnOverride, ProviderStatus{
		Provider: "anthropic",
		Runtime:  "anthropic_messages",
	})

	if evidence.State != ReasoningEffortStateUnsupported {
		t.Fatalf("State = %q, want %q", evidence.State, ReasoningEffortStateUnsupported)
	}
	if evidence.Forwarded {
		t.Fatalf("Forwarded = true, want false for unsupported provider: %+v", evidence)
	}
	if !strings.Contains(evidence.Reason, "anthropic_messages") {
		t.Fatalf("Reason = %q, want provider runtime evidence", evidence.Reason)
	}
}

func openAICompatibleReasoningRequestBody(t *testing.T, req ChatRequest) map[string]any {
	t.Helper()
	client := &httpClient{baseURL: "http://example.test", provider: "openai_compatible"}
	body, _, err := client.buildOpenAICompatibleChatRequestBody(req)
	if err != nil {
		t.Fatalf("buildOpenAICompatibleChatRequestBody() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", body, err)
	}
	return decoded
}

func chatCompletionsReasoningStatus() ProviderStatus {
	return ProviderStatus{Provider: "openai_compatible", Runtime: "chat_completions"}
}
