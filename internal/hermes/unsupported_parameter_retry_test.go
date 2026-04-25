package hermes

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnsupportedParameterRetryDetectorMatchesProviderPhrasings(t *testing.T) {
	matches := []struct {
		param   string
		message string
	}{
		{param: "temperature", message: `HTTP 400: Unsupported parameter: temperature`},
		{param: "temperature", message: `Error code: 400 - {"error":{"code":"unsupported_parameter","param":"temperature"}}`},
		{param: "temperature", message: `this model does not support temperature`},
		{param: "max_tokens", message: `HTTP 400: Unsupported parameter: max_tokens`},
		{param: "max_tokens", message: `Unknown parameter: max_tokens - use max_completion_tokens`},
		{param: "max_tokens", message: `Invalid parameter: max_tokens is not supported`},
		{param: "seed", message: `HTTP 400: unrecognized parameter: seed`},
		{param: "top_p", message: `Error: top_p is not supported for this model`},
	}
	for _, tt := range matches {
		t.Run(tt.param+"/"+tt.message, func(t *testing.T) {
			if !isUnsupportedParameterError(errors.New(tt.message), tt.param) {
				t.Fatalf("isUnsupportedParameterError(%q, %q) = false, want true", tt.message, tt.param)
			}
		})
	}

	unrelated := []struct {
		param   string
		message string
	}{
		{param: "temperature", message: `HTTP 400: max_tokens is too large`},
		{param: "temperature", message: `temperature must be between 0 and 2`},
		{param: "max_tokens", message: `Rate limit exceeded`},
		{param: "temperature", message: `Connection reset by peer`},
		{param: "", message: `HTTP 400: Unsupported parameter: temperature`},
	}
	for _, tt := range unrelated {
		t.Run("unrelated/"+tt.param+"/"+tt.message, func(t *testing.T) {
			if isUnsupportedParameterError(errors.New(tt.message), tt.param) {
				t.Fatalf("isUnsupportedParameterError(%q, %q) = true, want false", tt.message, tt.param)
			}
		})
	}
}

func TestUnsupportedParameterRetryTemperatureWrapperUsesGenericDetector(t *testing.T) {
	msg := `HTTP 400: unrecognized parameter: temperature`
	if !isUnsupportedTemperatureError(errors.New(msg)) {
		t.Fatalf("isUnsupportedTemperatureError(%q) = false, want true", msg)
	}
	if isUnsupportedTemperatureError(errors.New(`max_tokens is too large for this model`)) {
		t.Fatal("isUnsupportedTemperatureError(max_tokens size error) = true, want false")
	}
}

func TestUnsupportedParameterRetryMaxTokensSwitchesToMaxCompletionTokensOnce(t *testing.T) {
	var captured []parameterRetryCapturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		captured = append(captured, parameterRetryCapturedRequest{
			Header: r.Header.Clone(),
			Body:   append([]byte(nil), raw...),
		})
		if len(captured) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"message":"Unknown parameter: max_tokens - use max_completion_tokens","code":"unsupported_parameter","param":"max_tokens"}}`)
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
	stream, err := client.OpenStream(context.Background(), unsupportedParameterRetryRequest(700, ptrFloat64(0.4)))
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	defer stream.Close()

	if len(captured) != 2 {
		t.Fatalf("request count = %d, want exactly 2", len(captured))
	}
	for i, req := range captured {
		if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("request %d Authorization = %q, want Bearer test-key", i+1, got)
		}
		if got := req.Header.Get("X-Hermes-Session-Id"); got != "session-fixture" {
			t.Fatalf("request %d X-Hermes-Session-Id = %q, want session-fixture", i+1, got)
		}
	}

	first := decodeJSONMap(t, captured[0].Body)
	retry := decodeJSONMap(t, captured[1].Body)
	if got := first["max_tokens"]; got != float64(700) {
		t.Fatalf("first max_tokens = %#v, want 700 in original request body: %s", got, captured[0].Body)
	}
	if _, ok := first["max_completion_tokens"]; ok {
		t.Fatalf("first request unexpectedly contains max_completion_tokens: %s", captured[0].Body)
	}
	if _, ok := retry["max_tokens"]; ok {
		t.Fatalf("retry request still contains max_tokens: %s", captured[1].Body)
	}
	if got := retry["max_completion_tokens"]; got != float64(700) {
		t.Fatalf("retry max_completion_tokens = %#v, want 700 in retry body: %s", got, captured[1].Body)
	}
	if got := retry["temperature"]; got != 0.4 {
		t.Fatalf("retry temperature = %#v, want preserved 0.4", got)
	}

	delete(first, "max_tokens")
	delete(retry, "max_completion_tokens")
	if !mapsJSONEqual(first, retry) {
		t.Fatalf("retry changed fields other than max_tokens/max_completion_tokens\n--- first without max_tokens ---\n%s\n--- retry without max_completion_tokens ---\n%s",
			mustMarshalIndent(t, first), mustMarshalIndent(t, retry))
	}

	status := ProviderStatusOf(client)
	if status.UnsupportedParameterRetry.Attempts != 1 {
		t.Fatalf("UnsupportedParameterRetry.Attempts = %d, want 1", status.UnsupportedParameterRetry.Attempts)
	}
	if !status.UnsupportedParameterRetry.Stripped {
		t.Fatal("UnsupportedParameterRetry.Stripped = false, want true")
	}
	if status.UnsupportedParameterRetry.Parameter != "max_tokens" {
		t.Fatalf("UnsupportedParameterRetry.Parameter = %q, want max_tokens", status.UnsupportedParameterRetry.Parameter)
	}
	if status.UnsupportedParameterRetry.Replacement != "max_completion_tokens" {
		t.Fatalf("UnsupportedParameterRetry.Replacement = %q, want max_completion_tokens", status.UnsupportedParameterRetry.Replacement)
	}
	if status.UnsupportedParameterRetry.Model != "fixture-model" {
		t.Fatalf("UnsupportedParameterRetry.Model = %q, want fixture-model", status.UnsupportedParameterRetry.Model)
	}
	if !strings.Contains(status.UnsupportedParameterRetry.Reason, "Unknown parameter") {
		t.Fatalf("UnsupportedParameterRetry.Reason = %q, want unsupported-parameter evidence", status.UnsupportedParameterRetry.Reason)
	}
}

func TestUnsupportedParameterRetryMaxTokensRequiresFirstPayloadValue(t *testing.T) {
	var captured [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		captured = append(captured, append([]byte(nil), raw...))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Unsupported parameter: max_tokens","code":"unsupported_parameter","param":"max_tokens"}}`)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "")
	_, err := client.OpenStream(context.Background(), unsupportedParameterRetryRequest(0, ptrFloat64(0.4)))
	if err == nil {
		t.Fatal("OpenStream() error = nil, want provider error")
	}
	if len(captured) != 1 {
		t.Fatalf("request count = %d, want 1 when max_tokens was omitted", len(captured))
	}
	body := decodeJSONMap(t, captured[0])
	if _, ok := body["max_tokens"]; ok {
		t.Fatalf("first request emitted max_tokens despite zero MaxTokens: %s", captured[0])
	}
	if value, ok := body["max_completion_tokens"]; ok {
		t.Fatalf("request emitted max_completion_tokens=%#v despite zero MaxTokens: %s", value, captured[0])
	}
}

func TestUnsupportedParameterRetryMaxTokensDoesNotRetryUnrelatedErrors(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"max_tokens is too large for this model"}}`)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "")
	_, err := client.OpenStream(context.Background(), unsupportedParameterRetryRequest(700, ptrFloat64(0.4)))
	if err == nil {
		t.Fatal("OpenStream() error = nil, want unrelated provider error")
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want 1 for unrelated max_tokens error", requests)
	}
}

func TestUnsupportedParameterRetryDoesNotDeleteArbitraryUnsupportedParameters(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Unsupported parameter: seed","code":"unsupported_parameter","param":"seed"}}`)
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "")
	_, err := client.OpenStream(context.Background(), unsupportedParameterRetryRequest(700, ptrFloat64(0.4)))
	if err == nil {
		t.Fatal("OpenStream() error = nil, want unsupported seed provider error")
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want 1 for unsupported parameter without supported retry path", requests)
	}
}

func TestUnsupportedParameterRetryMaxTokensUsesCallerContext(t *testing.T) {
	var requests int
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Unsupported parameter: max_tokens","code":"unsupported_parameter","param":"max_tokens"}}`)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		cancel()
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "")
	_, err := client.OpenStream(ctx, unsupportedParameterRetryRequest(700, ptrFloat64(0.4)))
	if err == nil {
		t.Fatal("OpenStream() error = nil, want canceled caller context on retry")
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want retry to use canceled caller context before sending a second request", requests)
	}
}

type parameterRetryCapturedRequest struct {
	Header http.Header
	Body   []byte
}

func unsupportedParameterRetryRequest(maxTokens int, temp *float64) ChatRequest {
	req := unsupportedTemperatureRetryRequest(temp)
	req.MaxTokens = maxTokens
	req.SessionID = "session-fixture"
	return req
}
