package hermes

import "strings"

const (
	defaultOpenAIEndpoint   = "http://127.0.0.1:8642"
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	defaultGeminiBaseURL    = "https://generativelanguage.googleapis.com/v1beta"
)

// NewClient returns a provider-aware Client that preserves the canonical
// hermes.Client contract. The default provider remains the existing
// OpenAI-compatible HTTP client used by Hermes' api_server.
func NewClient(provider, endpoint, apiKey string) Client {
	switch normalizedProvider(provider) {
	case "anthropic":
		return newAnthropicClient(EffectiveEndpoint(provider, endpoint), apiKey)
	case "bedrock":
		return newBedrockClient(EffectiveEndpoint(provider, endpoint))
	case "gemini":
		return newGeminiClient(EffectiveEndpoint(provider, endpoint), apiKey)
	case "openrouter":
		return newOpenRouterClient(EffectiveEndpoint(provider, endpoint), apiKey)
	case "google-gemini-cli":
		return newGoogleCodeAssistClient(EffectiveEndpoint(provider, endpoint), apiKey)
	case "codex":
		return newCodexClient(EffectiveEndpoint(provider, endpoint), apiKey)
	default:
		return NewHTTPClient(EffectiveEndpoint(provider, endpoint), apiKey)
	}
}

// EffectiveEndpoint resolves empty or legacy-default endpoints for a provider
// into the actual base URL that the client should use.
func EffectiveEndpoint(provider, endpoint string) string {
	base := strings.TrimSpace(endpoint)
	switch normalizedProvider(provider) {
	case "anthropic":
		if base == "" || base == defaultOpenAIEndpoint {
			return defaultAnthropicBaseURL
		}
	case "bedrock":
		if base == "" || base == defaultOpenAIEndpoint {
			return defaultBedrockBaseURL("")
		}
	case "gemini":
		if base == "" || base == defaultOpenAIEndpoint {
			return defaultGeminiBaseURL
		}
	case "openrouter":
		if base == "" || base == defaultOpenAIEndpoint {
			return defaultOpenRouterBaseURL
		}
	case "google-gemini-cli":
		if base == "" || base == defaultOpenAIEndpoint || base == googleCodeAssistMarkerEndpoint {
			return defaultGoogleCodeAssistBaseURL
		}
	case "codex":
		if base == "" || base == defaultOpenAIEndpoint {
			return defaultCodexBaseURL
		}
	default:
		if base == "" {
			return defaultOpenAIEndpoint
		}
	}
	return strings.TrimRight(base, "/")
}

func normalizedProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "openai", "openai-compatible", "openai_compatible", "hermes":
		return "openai"
	case "anthropic":
		return "anthropic"
	case "bedrock", "aws-bedrock", "amazon-bedrock":
		return "bedrock"
	case "gemini", "google-gemini", "google_gemini":
		return "gemini"
	case "openrouter":
		return "openrouter"
	case "google-gemini-cli", "gemini-cli", "gemini-oauth", "google-code-assist", "google_code_assist":
		return "google-gemini-cli"
	case "codex", "openai-codex", "openai_codex":
		return "codex"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}
