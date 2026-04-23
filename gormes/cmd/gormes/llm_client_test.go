package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestNewLLMClient_UsesSelectedHermesAccount(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	cfgDir := filepath.Join(cfgHome, "gormes")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "work-key" {
			t.Fatalf("x-api-key = %q, want work-key", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(`
[hermes]
provider = "anthropic"
account = "work"

[[hermes.accounts]]
name = "personal"
api_key = "personal-key"

[[hermes.accounts]]
name = "work"
api_key = "work-key"
endpoint = "`+srv.URL+`"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	client, endpoint := newLLMClient(cfg)
	if endpoint != srv.URL {
		t.Fatalf("endpoint = %q, want %q", endpoint, srv.URL)
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestNewLLMClient_UsesTokenVaultCredential(t *testing.T) {
	cfgHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	cfgDir := filepath.Join(cfgHome, "gormes")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "vault-key" {
			t.Fatalf("x-api-key = %q, want vault-key", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(`
[hermes]
provider = "anthropic"
endpoint = "`+srv.URL+`"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	authPath := config.AuthTokenVaultPath()
	if err := os.MkdirAll(filepath.Dir(authPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, []byte(`{
  "version": 1,
  "credential_pool": {
    "anthropic": [
      {
        "access_token": "vault-key"
      }
    ]
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	client, endpoint := newLLMClient(cfg)
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

func TestNewLLMClient_GoogleCodeAssistRewritesMarkerEndpoint(t *testing.T) {
	_, endpoint := newLLMClient(config.Config{
		Hermes: config.HermesCfg{
			Provider: "google-gemini-cli",
			Endpoint: "cloudcode-pa://google",
		},
	})
	if endpoint != "https://cloudcode-pa.googleapis.com" {
		t.Fatalf("endpoint = %q, want https://cloudcode-pa.googleapis.com", endpoint)
	}
}

func TestNewLLMClient_OpenRouterUsesModelsHealthEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Fatalf("path = %s, want /api/v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want Bearer test-key", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client, endpoint := newLLMClient(config.Config{
		Hermes: config.HermesCfg{
			Provider: "openrouter",
			Endpoint: srv.URL + "/api/v1",
			APIKey:   "test-key",
		},
	})
	if endpoint != srv.URL+"/api/v1" {
		t.Fatalf("endpoint = %q, want %q", endpoint, srv.URL+"/api/v1")
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestNewLLMClient_OpenRouterRewritesLegacyDefaultEndpoint(t *testing.T) {
	_, endpoint := newLLMClient(config.Config{
		Hermes: config.HermesCfg{
			Provider: "openrouter",
			Endpoint: "http://127.0.0.1:8642",
		},
	})
	if endpoint != "https://openrouter.ai/api/v1" {
		t.Fatalf("endpoint = %q, want https://openrouter.ai/api/v1", endpoint)
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

func TestLLMProviderLabel_GoogleGeminiCLI(t *testing.T) {
	if got := llmProviderLabel("google-gemini-cli"); got != "google-gemini-cli" {
		t.Fatalf("label = %q, want google-gemini-cli", got)
	}
}

func TestLLMProviderLabel_OpenRouter(t *testing.T) {
	if got := llmProviderLabel("openrouter"); got != "openrouter" {
		t.Fatalf("label = %q, want openrouter", got)
	}
}
