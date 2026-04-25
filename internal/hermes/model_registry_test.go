package hermes

import "testing"

func TestDefaultModelRegistryExposesPricingCapabilitiesAndRawContext(t *testing.T) {
	got := LookupModelMetadata(ModelRegistryQuery{
		Provider: "openai",
		Model:    "gpt-4o-mini",
	})

	if !got.Found {
		t.Fatal("Found = false, want true")
	}
	if got.ProviderFamily != "openai" {
		t.Fatalf("ProviderFamily = %q, want openai", got.ProviderFamily)
	}
	if got.ModelFamily != "gpt-4o" {
		t.Fatalf("ModelFamily = %q, want gpt-4o", got.ModelFamily)
	}
	if got.RawContextWindow != 128_000 {
		t.Fatalf("RawContextWindow = %d, want 128000", got.RawContextWindow)
	}
	if got.MaxOutputTokens != 16_384 {
		t.Fatalf("MaxOutputTokens = %d, want 16384", got.MaxOutputTokens)
	}

	if got.Pricing.Status != ModelFactKnown {
		t.Fatalf("Pricing.Status = %q, want %q", got.Pricing.Status, ModelFactKnown)
	}
	if got.Pricing.InputUSDPerMillion != 0.15 {
		t.Fatalf("Pricing.InputUSDPerMillion = %v, want 0.15", got.Pricing.InputUSDPerMillion)
	}
	if got.Pricing.OutputUSDPerMillion != 0.60 {
		t.Fatalf("Pricing.OutputUSDPerMillion = %v, want 0.60", got.Pricing.OutputUSDPerMillion)
	}
	if got.Pricing.CacheReadUSDPerMillion != 0.075 {
		t.Fatalf("Pricing.CacheReadUSDPerMillion = %v, want 0.075", got.Pricing.CacheReadUSDPerMillion)
	}
	if got.Pricing.Source != ModelPricingSourceOfficialDocsSnapshot {
		t.Fatalf("Pricing.Source = %q, want %q", got.Pricing.Source, ModelPricingSourceOfficialDocsSnapshot)
	}

	if got.Capabilities.Status != ModelFactKnown {
		t.Fatalf("Capabilities.Status = %q, want %q", got.Capabilities.Status, ModelFactKnown)
	}
	if got.Capabilities.Tools != ModelCapabilitySupported {
		t.Fatalf("Capabilities.Tools = %q, want %q", got.Capabilities.Tools, ModelCapabilitySupported)
	}
	if got.Capabilities.Vision != ModelCapabilitySupported {
		t.Fatalf("Capabilities.Vision = %q, want %q", got.Capabilities.Vision, ModelCapabilitySupported)
	}
	if got.Capabilities.Reasoning != ModelCapabilityUnsupported {
		t.Fatalf("Capabilities.Reasoning = %q, want %q", got.Capabilities.Reasoning, ModelCapabilityUnsupported)
	}
	if got.Capabilities.StructuredOutput != ModelCapabilitySupported {
		t.Fatalf("Capabilities.StructuredOutput = %q, want %q", got.Capabilities.StructuredOutput, ModelCapabilitySupported)
	}
	if got.Registry.Source != ModelRegistrySourceEmbedded {
		t.Fatalf("Registry.Source = %q, want %q", got.Registry.Source, ModelRegistrySourceEmbedded)
	}
	if got.Registry.Freshness != ModelRegistryFreshnessCurrent {
		t.Fatalf("Registry.Freshness = %q, want %q", got.Registry.Freshness, ModelRegistryFreshnessCurrent)
	}
}

func TestModelRegistryKeepsMissingPricingAndCapabilitiesUnknown(t *testing.T) {
	registry := NewStaticModelRegistry(ModelRegistrySnapshot{
		Source:    ModelRegistrySourceEmbedded,
		Freshness: ModelRegistryFreshnessCurrent,
		Version:   "test-fixture",
	}, []ModelRegistryEntry{
		{
			Provider:         "fixture-provider",
			Model:            "bare-model",
			ProviderFamily:   "fixture",
			ModelFamily:      "bare",
			RawContextWindow: 64_000,
			MaxOutputTokens:  4_096,
		},
	})

	got := registry.Lookup(ModelRegistryQuery{
		Provider: "fixture-provider",
		Model:    "bare-model",
	})

	if !got.Found {
		t.Fatal("Found = false, want true")
	}
	if got.Pricing.Status != ModelFactUnknown {
		t.Fatalf("Pricing.Status = %q, want %q", got.Pricing.Status, ModelFactUnknown)
	}
	if got.Pricing.Known() {
		t.Fatal("Pricing.Known() = true, want false")
	}
	if got.Capabilities.Status != ModelFactUnknown {
		t.Fatalf("Capabilities.Status = %q, want %q", got.Capabilities.Status, ModelFactUnknown)
	}
	if got.Capabilities.Known() {
		t.Fatal("Capabilities.Known() = true, want false")
	}
	if got.Capabilities.Tools != ModelCapabilityUnknown {
		t.Fatalf("Capabilities.Tools = %q, want %q", got.Capabilities.Tools, ModelCapabilityUnknown)
	}
	if got.Capabilities.Vision != ModelCapabilityUnknown {
		t.Fatalf("Capabilities.Vision = %q, want %q", got.Capabilities.Vision, ModelCapabilityUnknown)
	}
	if got.Capabilities.Reasoning != ModelCapabilityUnknown {
		t.Fatalf("Capabilities.Reasoning = %q, want %q", got.Capabilities.Reasoning, ModelCapabilityUnknown)
	}
}

func TestModelRegistryReportsUnknownModelAndStaleEmbeddedSnapshot(t *testing.T) {
	registry := NewStaticModelRegistry(ModelRegistrySnapshot{
		Source:    ModelRegistrySourceEmbedded,
		Freshness: ModelRegistryFreshnessStale,
		Version:   "models.dev-2026-02-01",
		Reason:    "embedded registry snapshot is older than the operator freshness policy",
	}, []ModelRegistryEntry{
		{
			Provider: "openai",
			Model:    "fixture-model",
		},
	})

	got := registry.Lookup(ModelRegistryQuery{
		Provider: "openai",
		Model:    "not-in-fixture",
	})

	if got.Found {
		t.Fatal("Found = true, want false")
	}
	if got.Pricing.Status != ModelFactUnknown {
		t.Fatalf("Pricing.Status = %q, want %q", got.Pricing.Status, ModelFactUnknown)
	}
	if got.Capabilities.Status != ModelFactUnknown {
		t.Fatalf("Capabilities.Status = %q, want %q", got.Capabilities.Status, ModelFactUnknown)
	}
	if got.Registry.Freshness != ModelRegistryFreshnessStale {
		t.Fatalf("Registry.Freshness = %q, want %q", got.Registry.Freshness, ModelRegistryFreshnessStale)
	}
	if got.Registry.Reason == "" {
		t.Fatal("Registry.Reason is empty, want stale-data reason")
	}
}
