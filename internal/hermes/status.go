package hermes

// CapabilityStatus reports whether a provider-side resilience capability can
// be relied on by routing and status callers.
type CapabilityStatus struct {
	Available bool
	Reason    string
}

// ProviderCapabilities are intentionally small: Phase 4.H only needs the
// provider to make cache/rate/budget availability visible before routing
// decisions depend on those surfaces.
type ProviderCapabilities struct {
	PromptCache     CapabilityStatus
	ReasoningEcho   CapabilityStatus
	RateGuard       CapabilityStatus
	BudgetTelemetry CapabilityStatus
}

// ProviderTemperatureRetryStatus records the last provider recovery where an
// unsupported temperature parameter was stripped and retried.
type ProviderTemperatureRetryStatus struct {
	Attempts int
	Stripped bool
	Model    string
	Reason   string
}

// ProviderStatus is the provider-owned status snapshot the kernel can attach
// to render frames without knowing adapter-specific behavior.
type ProviderStatus struct {
	Provider         string
	Runtime          string
	Capabilities     ProviderCapabilities
	TemperatureRetry ProviderTemperatureRetryStatus
}

type providerStatusReporter interface {
	ProviderStatus() ProviderStatus
}

// ProviderStatusOf returns a normalized provider status snapshot. Adapters
// that do not implement ProviderStatus are visible as unknown/degraded rather
// than silently assumed to support optional resilience features.
func ProviderStatusOf(client Client) ProviderStatus {
	reporter, ok := client.(providerStatusReporter)
	if !ok || reporter == nil {
		return unknownProviderStatus()
	}
	return normalizeProviderStatus(reporter.ProviderStatus())
}

func openAICompatibleProviderStatus(provider, baseURL string) ProviderStatus {
	providerName := provider
	if providerName == "" {
		providerName = "openai_compatible"
	}
	return ProviderStatus{
		Provider: providerName,
		Runtime:  "chat_completions",
		Capabilities: ProviderCapabilities{
			PromptCache:     unavailableCapability("cache_control stripped by openai_compatible request mapping"),
			ReasoningEcho:   openAICompatibleReasoningEchoStatus(provider, baseURL),
			RateGuard:       unavailableCapability("provider rate guard not implemented"),
			BudgetTelemetry: unavailableCapability("budget telemetry not implemented"),
		},
	}
}

func anthropicProviderStatus() ProviderStatus {
	return ProviderStatus{
		Provider: "anthropic",
		Runtime:  "anthropic_messages",
		Capabilities: ProviderCapabilities{
			PromptCache:     CapabilityStatus{Available: true, Reason: "cache_control supported by anthropic messages content blocks"},
			ReasoningEcho:   unavailableCapability("reasoning_content echo padding is not required by anthropic messages"),
			RateGuard:       unavailableCapability("provider rate guard not implemented"),
			BudgetTelemetry: unavailableCapability("budget telemetry not implemented"),
		},
	}
}

func codexResponsesProviderStatus() ProviderStatus {
	return ProviderStatus{
		Provider: "openai-codex",
		Runtime:  "responses_unavailable",
		Capabilities: ProviderCapabilities{
			PromptCache:     unavailableCapability("Codex Responses auth wiring not configured"),
			RateGuard:       unavailableCapability("Codex provider rate guard not implemented"),
			BudgetTelemetry: unavailableCapability("Codex budget telemetry not implemented"),
		},
	}
}

func unknownProviderStatus() ProviderStatus {
	return ProviderStatus{
		Provider: "unknown",
		Runtime:  "unknown",
		Capabilities: ProviderCapabilities{
			PromptCache:     unavailableCapability("provider status unavailable"),
			ReasoningEcho:   unavailableCapability("provider status unavailable"),
			RateGuard:       unavailableCapability("provider status unavailable"),
			BudgetTelemetry: unavailableCapability("provider status unavailable"),
		},
	}
}

func normalizeProviderStatus(status ProviderStatus) ProviderStatus {
	if status.Provider == "" {
		status.Provider = "unknown"
	}
	if status.Runtime == "" {
		status.Runtime = "unknown"
	}
	status.Capabilities.PromptCache = normalizeCapability(status.Capabilities.PromptCache, "prompt cache status unavailable")
	status.Capabilities.ReasoningEcho = normalizeCapability(status.Capabilities.ReasoningEcho, "reasoning echo status unavailable")
	status.Capabilities.RateGuard = normalizeCapability(status.Capabilities.RateGuard, "provider rate guard status unavailable")
	status.Capabilities.BudgetTelemetry = normalizeCapability(status.Capabilities.BudgetTelemetry, "budget telemetry status unavailable")
	return status
}

func normalizeCapability(status CapabilityStatus, fallback string) CapabilityStatus {
	if status.Reason == "" {
		status.Reason = fallback
	}
	return status
}

func unavailableCapability(reason string) CapabilityStatus {
	return CapabilityStatus{Available: false, Reason: reason}
}

func openAICompatibleReasoningEchoStatus(provider, baseURL string) CapabilityStatus {
	if openAICompatibleRequiresReasoningEcho(provider, "", baseURL) {
		return CapabilityStatus{
			Available: true,
			Reason:    "thinking-mode provider requires reasoning_content on assistant tool-call replay; stored transcripts without reasoning are repaired with empty reasoning_content padding",
		}
	}
	return unavailableCapability("reasoning_content echo padding is not required for this openai-compatible provider")
}
