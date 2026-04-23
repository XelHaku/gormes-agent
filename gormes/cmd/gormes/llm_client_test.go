package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

func TestNewLLMClient_AnthropicUsesModelsHealthEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Fatalf("x-api-key = %q, want test-key", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client, endpoint := newLLMClient(config.Config{
		Hermes: config.HermesCfg{
			Provider: "anthropic",
			Endpoint: srv.URL,
			APIKey:   "test-key",
		},
	})
	if endpoint != srv.URL {
		t.Fatalf("endpoint = %q, want %q", endpoint, srv.URL)
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestNewLLMClient_AnthropicRewritesLegacyDefaultEndpoint(t *testing.T) {
	_, endpoint := newLLMClient(config.Config{
		Hermes: config.HermesCfg{
			Provider: "anthropic",
			Endpoint: "http://127.0.0.1:8642",
		},
	})
	if endpoint != "https://api.anthropic.com" {
		t.Fatalf("endpoint = %q, want https://api.anthropic.com", endpoint)
	}
}

func TestNewLLMClient_CodexUsesModelsHealthEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want Bearer test-key", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client, endpoint := newLLMClient(config.Config{
		Hermes: config.HermesCfg{
			Provider: "codex",
			Endpoint: srv.URL,
			APIKey:   "test-key",
		},
	})
	if endpoint != srv.URL {
		t.Fatalf("endpoint = %q, want %q", endpoint, srv.URL)
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestNewLLMClient_CodexRewritesLegacyDefaultEndpoint(t *testing.T) {
	_, endpoint := newLLMClient(config.Config{
		Hermes: config.HermesCfg{
			Provider: "openai-codex",
			Endpoint: "http://127.0.0.1:8642",
		},
	})
	if endpoint != "https://api.openai.com" {
		t.Fatalf("endpoint = %q, want https://api.openai.com", endpoint)
	}
}

func TestNewLLMClient_GeminiUsesModelsHealthEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			t.Fatalf("path = %s, want /v1beta/models", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Fatalf("x-goog-api-key = %q, want test-key", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client, endpoint := newLLMClient(config.Config{
		Hermes: config.HermesCfg{
			Provider: "gemini",
			Endpoint: srv.URL + "/v1beta",
			APIKey:   "test-key",
		},
	})
	if endpoint != srv.URL+"/v1beta" {
		t.Fatalf("endpoint = %q, want %q", endpoint, srv.URL+"/v1beta")
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestNewLLMClient_GeminiRewritesLegacyDefaultEndpoint(t *testing.T) {
	_, endpoint := newLLMClient(config.Config{
		Hermes: config.HermesCfg{
			Provider: "gemini",
			Endpoint: "http://127.0.0.1:8642",
		},
	})
	if endpoint != "https://generativelanguage.googleapis.com/v1beta" {
		t.Fatalf("endpoint = %q, want https://generativelanguage.googleapis.com/v1beta", endpoint)
	}
}

func TestLLMProviderLabel_Codex(t *testing.T) {
	if got := llmProviderLabel("codex"); got != "codex" {
		t.Fatalf("label = %q, want codex", got)
	}
}

func TestLLMProviderLabel_Gemini(t *testing.T) {
	if got := llmProviderLabel("gemini"); got != "gemini" {
		t.Fatalf("label = %q, want gemini", got)
	}
}
