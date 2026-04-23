package main

import (
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
)

func newLLMClient(cfg config.Config) (hermes.Client, string) {
	endpoint := hermes.EffectiveEndpoint(cfg.Hermes.Provider, cfg.Hermes.Endpoint)
	return hermes.NewClient(cfg.Hermes.Provider, endpoint, cfg.Hermes.APIKey), endpoint
}

func llmProviderLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		return "anthropic"
	case "gemini", "google-gemini":
		return "gemini"
	case "codex", "openai-codex":
		return "codex"
	default:
		return "api_server"
	}
}
