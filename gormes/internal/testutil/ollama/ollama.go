package ollama

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	// RunEnvVar gates real Ollama-backed integration tests so the default
	// broad suite stays hermetic.
	RunEnvVar = "GORMES_RUN_OLLAMA_INTEGRATION"

	// EndpointEnvVar overrides the OpenAI-compatible Ollama base URL.
	EndpointEnvVar = "GORMES_EXTRACTOR_ENDPOINT"

	// ModelEnvVar overrides the chat model used by the live integration tests.
	ModelEnvVar = "GORMES_EXTRACTOR_MODEL"

	// DefaultEndpoint is the conventional local Ollama endpoint.
	DefaultEndpoint = "http://localhost:11434"

	// DefaultModel is the default chat model used by the live integration tests.
	DefaultModel = "gemma4:26b"
)

// Enabled reports whether live Ollama-backed integration tests are explicitly
// enabled for this run.
func Enabled() bool {
	v := strings.TrimSpace(os.Getenv(RunEnvVar))
	if v == "" {
		return false
	}
	enabled, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return enabled
}

// Endpoint returns the configured OpenAI-compatible Ollama base URL.
func Endpoint() string {
	if v := strings.TrimSpace(os.Getenv(EndpointEnvVar)); v != "" {
		return v
	}
	return DefaultEndpoint
}

// Model returns the configured chat model for live integration tests.
func Model() string {
	if v := strings.TrimSpace(os.Getenv(ModelEnvVar)); v != "" {
		return v
	}
	return DefaultModel
}

// SkipUnlessEnabled skips the test unless live Ollama integration was
// explicitly requested for this run.
func SkipUnlessEnabled(t *testing.T) {
	t.Helper()
	if Enabled() {
		return
	}
	t.Skipf("real Ollama integration disabled; set %s=1 to run live extractor/cron/recall tests", RunEnvVar)
}

// SkipUnlessExtractorReady skips unless live integration is enabled, the
// endpoint is reachable, and the configured model is listed by /v1/models.
func SkipUnlessExtractorReady(t *testing.T) {
	t.Helper()
	SkipUnlessEnabled(t)

	endpoint := Endpoint()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(endpoint + "/v1/models")
	if err != nil {
		t.Skipf("LLM endpoint %s not reachable (connection refused?): %v\n"+
			"  To run this test: start Ollama (or any OpenAI-compatible server)\n"+
			"  and optionally set %s / %s.",
			endpoint, err, EndpointEnvVar, ModelEnvVar)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Skipf("LLM endpoint %s returned HTTP %d: %s", endpoint, resp.StatusCode, string(body))
	}

	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		t.Skipf("could not decode /v1/models response: %v", err)
	}

	want := Model()
	for _, m := range models.Data {
		if m.ID == want {
			return
		}
	}

	available := make([]string, 0, len(models.Data))
	for _, m := range models.Data {
		available = append(available, m.ID)
	}
	t.Skipf("model %q not loaded on %s; available: %v\n"+
		"  Pull with: ollama pull %s\n"+
		"  Or override with %s=<one of the above>.",
		want, endpoint, available, want, ModelEnvVar)
}
