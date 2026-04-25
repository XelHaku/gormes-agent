package hermes

import "testing"

func TestModelRouterHonorsTurnOverrideBeforeAutomaticRouting(t *testing.T) {
	router := NewModelRouter(ModelRouterConfig{
		Registry:        routingFixtureRegistry(),
		ContextResolver: routingFixtureContextResolver(),
	})

	got := router.Select(ModelRoutingRequest{
		UserMessage: "thanks",
		Primary:     ModelRoute{Provider: "openai", Model: "gpt-4o-mini"},
		TurnOverride: ModelRoute{
			Provider: "anthropic",
			Model:    "claude-opus-4-20250514",
		},
		Automatic: AutomaticModelRoutingPolicy{
			Enabled:     true,
			SimpleRoute: ModelRoute{Provider: "openai", Model: "gpt-4o-mini"},
		},
		Providers: []ProviderAvailability{
			{Provider: "openai", Available: true},
			{Provider: "anthropic", Available: true},
		},
	})

	if got.Route.Provider != "anthropic" || got.Route.Model != "claude-opus-4-20250514" {
		t.Fatalf("Route = %#v, want anthropic claude-opus-4-20250514", got.Route)
	}
	if got.Reason != ModelRouteReasonTurnOverride {
		t.Fatalf("Reason = %q, want %q", got.Reason, ModelRouteReasonTurnOverride)
	}
	if !got.Metadata.Found {
		t.Fatal("Metadata.Found = false, want true")
	}
	if got.Metadata.ProviderFamily != "anthropic" {
		t.Fatalf("Metadata.ProviderFamily = %q, want anthropic", got.Metadata.ProviderFamily)
	}
	if !got.Metadata.Pricing.Known() {
		t.Fatal("Metadata.Pricing.Known() = false, want true")
	}
	if !got.Metadata.Capabilities.Known() {
		t.Fatal("Metadata.Capabilities.Known() = false, want true")
	}
	if got.Context.ContextLength != 200_000 || got.Context.Source != ModelContextSourceProviderCap {
		t.Fatalf("Context = %#v, want provider cap 200000", got.Context)
	}
	if hasRoutingStatus(got.Status, ModelRoutingStatusAutomaticRouteSkipped) {
		t.Fatalf("Status contains automatic-route skip despite explicit turn override: %#v", got.Status)
	}
}

func TestModelRouterRejectsInvalidTurnOverrideAndKeepsConfigRoute(t *testing.T) {
	router := NewModelRouter(ModelRouterConfig{
		Registry:        routingFixtureRegistry(),
		ContextResolver: routingFixtureContextResolver(),
	})

	got := router.Select(ModelRoutingRequest{
		UserMessage:    "hi",
		Primary:        ModelRoute{Provider: "openai", Model: "gpt-4o-mini"},
		ConfigOverride: ModelRoute{Provider: "openai", Model: "gpt-4o-mini"},
		TurnOverride:   ModelRoute{Provider: "anthropic", Model: "missing-model"},
		Automatic: AutomaticModelRoutingPolicy{
			Enabled:     true,
			SimpleRoute: ModelRoute{Provider: "anthropic", Model: "claude-opus-4-20250514"},
		},
		Providers: []ProviderAvailability{
			{Provider: "openai", Available: true},
			{Provider: "anthropic", Available: true},
		},
	})

	if got.Route.Provider != "openai" || got.Route.Model != "gpt-4o-mini" {
		t.Fatalf("Route = %#v, want config override openai gpt-4o-mini", got.Route)
	}
	if got.Reason != ModelRouteReasonConfigOverride {
		t.Fatalf("Reason = %q, want %q", got.Reason, ModelRouteReasonConfigOverride)
	}
	assertRoutingStatus(t, got.Status, ModelRoutingStatusInvalidOverride, "turn override model metadata is unavailable")
	assertRoutingStatus(t, got.Status, ModelRoutingStatusMetadataGap, "missing metadata for anthropic/missing-model")
}

func TestModelRouterChoosesFirstAvailableFallbackWithKnownMetadata(t *testing.T) {
	router := NewModelRouter(ModelRouterConfig{
		Registry:        routingFixtureRegistry(),
		ContextResolver: routingFixtureContextResolver(),
	})

	got := router.Select(ModelRoutingRequest{
		UserMessage:       "retry the turn",
		Primary:           ModelRoute{Provider: "openai-codex", Model: "gpt-5.5"},
		FallbackRequested: true,
		Fallback: FallbackModelPolicy{
			Enabled: true,
			Routes: []ModelRoute{
				{Provider: "anthropic", Model: "not-in-registry"},
				{Provider: "anthropic", Model: "claude-opus-4-20250514"},
				{Provider: "openai", Model: "gpt-4o-mini"},
			},
		},
		Providers: []ProviderAvailability{
			{Provider: "openai-codex", Available: false, Reason: "Codex auth expired"},
			{Provider: "anthropic", Available: true},
			{Provider: "openai", Available: true},
		},
	})

	if got.Route.Provider != "anthropic" || got.Route.Model != "claude-opus-4-20250514" {
		t.Fatalf("Route = %#v, want first available metadata-backed fallback", got.Route)
	}
	if got.Reason != ModelRouteReasonFallbackSelected {
		t.Fatalf("Reason = %q, want %q", got.Reason, ModelRouteReasonFallbackSelected)
	}
	if got.Context.ContextLength != 200_000 {
		t.Fatalf("Context.ContextLength = %d, want 200000", got.Context.ContextLength)
	}
	assertRoutingStatus(t, got.Status, ModelRoutingStatusProviderUnavailable, "openai-codex unavailable: Codex auth expired")
	assertRoutingStatus(t, got.Status, ModelRoutingStatusMetadataGap, "missing metadata for anthropic/not-in-registry")
}

func TestModelRouterReportsDisabledFallbackWithoutSwitching(t *testing.T) {
	router := NewModelRouter(ModelRouterConfig{
		Registry:        routingFixtureRegistry(),
		ContextResolver: routingFixtureContextResolver(),
	})

	got := router.Select(ModelRoutingRequest{
		UserMessage:       "retry after failure",
		Primary:           ModelRoute{Provider: "openai-codex", Model: "gpt-5.5"},
		FallbackRequested: true,
		Fallback: FallbackModelPolicy{
			Enabled: false,
			Routes: []ModelRoute{
				{Provider: "anthropic", Model: "claude-opus-4-20250514"},
			},
		},
		Providers: []ProviderAvailability{
			{Provider: "openai-codex", Available: false, Reason: "provider disabled by operator"},
			{Provider: "anthropic", Available: true},
		},
	})

	if got.Route.Provider != "openai-codex" || got.Route.Model != "gpt-5.5" {
		t.Fatalf("Route = %#v, want unchanged primary route when fallback is disabled", got.Route)
	}
	if got.Reason != ModelRouteReasonFallbackDisabled {
		t.Fatalf("Reason = %q, want %q", got.Reason, ModelRouteReasonFallbackDisabled)
	}
	assertRoutingStatus(t, got.Status, ModelRoutingStatusProviderUnavailable, "openai-codex unavailable: provider disabled by operator")
	assertRoutingStatus(t, got.Status, ModelRoutingStatusFallbackDisabled, "fallback policy disabled")
}

func TestModelRouterSelectsAutomaticRouteOnlyForSimpleSignals(t *testing.T) {
	router := NewModelRouter(ModelRouterConfig{
		Registry:        routingFixtureRegistry(),
		ContextResolver: routingFixtureContextResolver(),
	})
	base := ModelRoutingRequest{
		Primary: ModelRoute{Provider: "openai-codex", Model: "gpt-5.5"},
		Automatic: AutomaticModelRoutingPolicy{
			Enabled:     true,
			SimpleRoute: ModelRoute{Provider: "openai", Model: "gpt-4o-mini"},
		},
		Providers: []ProviderAvailability{
			{Provider: "openai-codex", Available: true},
			{Provider: "openai", Available: true},
		},
	}

	simple := base
	simple.UserMessage = "what time is it"
	gotSimple := router.Select(simple)
	if gotSimple.Route.Provider != "openai" || gotSimple.Route.Model != "gpt-4o-mini" {
		t.Fatalf("simple route = %#v, want automatic cheap route", gotSimple.Route)
	}
	if gotSimple.Reason != ModelRouteReasonAutomaticSimpleTurn {
		t.Fatalf("simple reason = %q, want %q", gotSimple.Reason, ModelRouteReasonAutomaticSimpleTurn)
	}

	complex := base
	complex.UserMessage = "debug this stacktrace and propose an implementation plan"
	gotComplex := router.Select(complex)
	if gotComplex.Route.Provider != "openai-codex" || gotComplex.Route.Model != "gpt-5.5" {
		t.Fatalf("complex route = %#v, want primary route", gotComplex.Route)
	}
	if gotComplex.Reason != ModelRouteReasonPrimary {
		t.Fatalf("complex reason = %q, want %q", gotComplex.Reason, ModelRouteReasonPrimary)
	}
	assertRoutingStatus(t, gotComplex.Status, ModelRoutingStatusAutomaticRouteSkipped, "complex task signal")
}

func routingFixtureRegistry() ModelRegistry {
	return NewStaticModelRegistry(ModelRegistrySnapshot{
		Source:    ModelRegistrySourceTestdata,
		Freshness: ModelRegistryFreshnessCurrent,
		Version:   "routing-test-fixture",
	}, []ModelRegistryEntry{
		{
			Provider:         "openai",
			Model:            "gpt-4o-mini",
			ProviderFamily:   "openai",
			ModelFamily:      "gpt-4o",
			RawContextWindow: 128_000,
			MaxOutputTokens:  16_384,
			Pricing:          knownModelPricing(0.15, 0.60, 0.075, 0, ModelPricingSourceOfficialDocsSnapshot, "openai-test-pricing"),
			Capabilities:     knownModelCapabilities(ModelCapabilitySupported, ModelCapabilitySupported, ModelCapabilityUnsupported, ModelCapabilityUnsupported, ModelCapabilityUnsupported, ModelCapabilitySupported, ModelCapabilityUnsupported),
		},
		{
			Provider:         "anthropic",
			Model:            "claude-opus-4-20250514",
			ProviderFamily:   "anthropic",
			ModelFamily:      "claude-opus-4",
			RawContextWindow: 200_000,
			MaxOutputTokens:  32_000,
			Pricing:          knownModelPricing(15, 75, 1.5, 18.75, ModelPricingSourceOfficialDocsSnapshot, "anthropic-test-pricing"),
			Capabilities:     knownModelCapabilities(ModelCapabilitySupported, ModelCapabilitySupported, ModelCapabilitySupported, ModelCapabilityUnsupported, ModelCapabilityUnsupported, ModelCapabilityUnsupported, ModelCapabilityUnsupported),
		},
		{
			Provider:         "openai-codex",
			Model:            "gpt-5.5",
			ProviderFamily:   "openai",
			ModelFamily:      "gpt-5",
			RawContextWindow: 1_050_000,
			MaxOutputTokens:  128_000,
			Pricing:          unknownModelPricing(),
			Capabilities:     knownModelCapabilities(ModelCapabilitySupported, ModelCapabilitySupported, ModelCapabilitySupported, ModelCapabilityUnsupported, ModelCapabilityUnsupported, ModelCapabilitySupported, ModelCapabilityUnsupported),
		},
	})
}

func routingFixtureContextResolver() ModelContextResolver {
	return NewModelContextResolver(StaticModelContextCaps{
		ModelContextKey{Provider: "anthropic", Model: "claude-opus-4-20250514"}: 200_000,
		ModelContextKey{Provider: "openai-codex", Model: "gpt-5.5"}:             272_000,
	})
}

func assertRoutingStatus(t *testing.T, got []ModelRoutingStatus, code ModelRoutingStatusCode, contains string) {
	t.Helper()
	for _, status := range got {
		if status.Code == code && status.Message == contains {
			return
		}
	}
	t.Fatalf("status %#v does not contain %q with message %q", got, code, contains)
}

func hasRoutingStatus(got []ModelRoutingStatus, code ModelRoutingStatusCode) bool {
	for _, status := range got {
		if status.Code == code {
			return true
		}
	}
	return false
}
