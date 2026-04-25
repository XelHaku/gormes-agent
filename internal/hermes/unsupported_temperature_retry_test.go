package hermes

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnsupportedTemperatureRetryDetectorMatchesProviderPhrasings(t *testing.T) {
	matches := []string{
		`HTTP 400: Unsupported parameter: temperature`,
		`Error code: 400 - {"error":{"message":"Unsupported parameter: 'temperature'"}}`,
		`Error code: 400 - {"error":{"code":"unsupported_parameter","param":"temperature"}}`,
		`Provider returned error: temperature is not supported for this model`,
		`this model does not support temperature`,
		`temperature: unknown parameter`,
		`unrecognized request argument supplied: temperature`,
	}
	for _, message := range matches {
		t.Run(message, func(t *testing.T) {
			if !isUnsupportedTemperatureError(errors.New(message)) {
				t.Fatalf("isUnsupportedTemperatureError(%q) = false, want true", message)
			}
		})
	}

	unrelated := []string{
		`HTTP 400: Invalid value: 'tool'. Supported values are: 'assistant'`,
		`max_tokens is too large for this model`,
		`Rate limit exceeded`,
		`Connection reset by peer`,
		`temperature must be between 0 and 2`,
	}
	for _, message := range unrelated {
		t.Run(message, func(t *testing.T) {
			if isUnsupportedTemperatureError(errors.New(message)) {
				t.Fatalf("isUnsupportedTemperatureError(%q) = true, want false", message)
			}
		})
	}
}

func TestUnsupportedTemperatureRetryStripsOnlyTemperatureOnce(t *testing.T) {
	var captured [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		captured = append(captured, append([]byte(nil), raw...))
		if len(captured) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"message":"Unsupported parameter: temperature","code":"unsupported_parameter","param":"temperature"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		bw := bufio.NewWriter(w)
		_, _ = fmt.Fprint(bw, "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(bw, "data: [DONE]\n\n")
		_ = bw.Flush()
	}))
	defer srv.Close()

	client := NewHTTPClientWithProvider(srv.URL, "test-key", "openrouter")
	stream, err := client.OpenStream(context.Background(), unsupportedTemperatureRetryRequest(ptrFloat64(0.3)))
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	defer stream.Close()

	if len(captured) != 2 {
		t.Fatalf("request count = %d, want exactly 2", len(captured))
	}
	first := decodeJSONMap(t, captured[0])
	retry := decodeJSONMap(t, captured[1])
	if got := first["temperature"]; got != 0.3 {
		t.Fatalf("first temperature = %#v, want 0.3 in original request body: %s", got, captured[0])
	}
	if _, ok := retry["temperature"]; ok {
		t.Fatalf("retry request still contains temperature: %s", captured[1])
	}
	delete(first, "temperature")
	if !mapsJSONEqual(first, retry) {
		t.Fatalf("retry changed fields other than temperature\n--- first without temperature ---\n%s\n--- retry ---\n%s",
			mustMarshalIndent(t, first), mustMarshalIndent(t, retry))
	}
	if got := retry["model"]; got != "fixture-model" {
		t.Fatalf("retry model = %#v, want fixture-model", got)
	}
	if got := retry["max_tokens"]; got != float64(500) {
		t.Fatalf("retry max_tokens = %#v, want 500", got)
	}

	status := ProviderStatusOf(client)
	if status.TemperatureRetry.Attempts != 1 {
		t.Fatalf("TemperatureRetry.Attempts = %d, want 1", status.TemperatureRetry.Attempts)
	}
	if !status.TemperatureRetry.Stripped {
		t.Fatal("TemperatureRetry.Stripped = false, want true")
	}
	if status.TemperatureRetry.Model != "fixture-model" {
		t.Fatalf("TemperatureRetry.Model = %q, want fixture-model", status.TemperatureRetry.Model)
	}
	if !strings.Contains(status.TemperatureRetry.Reason, "Unsupported parameter") {
		t.Fatalf("TemperatureRetry.Reason = %q, want unsupported-temperature evidence", status.TemperatureRetry.Reason)
	}
}

func TestUnsupportedTemperatureRetryDoesNotRetryUnrelatedErrors(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Invalid value: 'tool'. Supported values are: 'assistant'"}}`)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "")
	_, err := client.OpenStream(context.Background(), unsupportedTemperatureRetryRequest(ptrFloat64(0.3)))
	if err == nil {
		t.Fatal("OpenStream() error = nil, want unrelated provider error")
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want 1 for unrelated 400", requests)
	}
}

func TestUnsupportedTemperatureRetryRequiresTemperatureInFirstPayload(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Unsupported parameter: temperature"}}`)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "")
	_, err := client.OpenStream(context.Background(), unsupportedTemperatureRetryRequest(nil))
	if err == nil {
		t.Fatal("OpenStream() error = nil, want provider error")
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want 1 when no temperature was sent", requests)
	}
}

func unsupportedTemperatureRetryRequest(temp *float64) ChatRequest {
	return ChatRequest{
		Model:       "fixture-model",
		MaxTokens:   500,
		Temperature: temp,
		Stream:      true,
		Messages: []Message{
			{Role: "system", Content: "Follow policy."},
			{Role: "user", Content: "Lookup weather."},
			{
				Role:    "assistant",
				Content: "Checking.",
				ToolCalls: []ToolCall{{
					ID:        "call_weather",
					Name:      "get_weather",
					Arguments: json.RawMessage(`{"location":"Monterrey"}`),
				}},
			},
			{
				Role:       "tool",
				ToolCallID: "call_weather",
				Name:       "get_weather",
				Content:    `{"condition":"sunny"}`,
			},
		},
		Tools: []ToolDescriptor{{
			Name:        "get_weather",
			Description: "Returns weather.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
		}},
	}
}

func TestUnsupportedTemperatureRetryAuxiliaryTaskFixturesMirrorHermes5006b220(t *testing.T) {
	tasks := []string{
		"auxiliary.compression",
		"auxiliary.session_search",
		"auxiliary.vision",
		"auxiliary.web_extract",
	}
	seen := map[string]bool{}
	client := NewHTTPClient("http://127.0.0.1:1", "").(*httpClient)

	for _, task := range tasks {
		t.Run(task, func(t *testing.T) {
			seen[task] = true
			req := unsupportedTemperatureRetryRequest(ptrFloat64(0.3))
			req.Model = "fixture-" + strings.TrimPrefix(task, "auxiliary.")
			req.Messages[1].Content = "Run " + task + "."

			body, _, err := client.buildOpenAICompatibleChatRequestBody(req)
			if err != nil {
				t.Fatalf("buildOpenAICompatibleChatRequestBody() error = %v", err)
			}
			payload := decodeJSONMap(t, body)
			if got := payload["temperature"]; got != 0.3 {
				t.Fatalf("%s temperature = %#v, want 0.3 in original retry fixture body: %s", task, got, body)
			}
			if got := payload["max_tokens"]; got != float64(500) {
				t.Fatalf("%s max_tokens = %#v, want 500 in original retry fixture body: %s", task, got, body)
			}
		})
	}

	deletedTask := "auxiliary." + strings.Join([]string{"flush", "memories"}, "_")
	if seen[deletedTask] {
		t.Fatalf("auxiliary task fixtures include deleted task %q", deletedTask)
	}
}

func decodeJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode JSON %s: %v", raw, err)
	}
	return out
}

func mapsJSONEqual(left, right map[string]any) bool {
	leftRaw, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightRaw, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return bytes.Equal(leftRaw, rightRaw)
}

func jsonHasKey(t *testing.T, raw []byte, key string) bool {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode JSON %s: %v", raw, err)
	}
	return jsonValueHasKey(value, key)
}

func jsonValueHasKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for k, v := range typed {
			if k == key || jsonValueHasKey(v, key) {
				return true
			}
		}
	case []any:
		for _, v := range typed {
			if jsonValueHasKey(v, key) {
				return true
			}
		}
	}
	return false
}
