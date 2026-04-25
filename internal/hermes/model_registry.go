package hermes

type ModelFactStatus string

const (
	ModelFactKnown   ModelFactStatus = "known"
	ModelFactUnknown ModelFactStatus = "unknown"
)

type ModelCapabilityFlag string

const (
	ModelCapabilitySupported   ModelCapabilityFlag = "supported"
	ModelCapabilityUnsupported ModelCapabilityFlag = "unsupported"
	ModelCapabilityUnknown     ModelCapabilityFlag = "unknown"
)

type ModelPricingSource string

const (
	ModelPricingSourceNone                 ModelPricingSource = "none"
	ModelPricingSourceOfficialDocsSnapshot ModelPricingSource = "official_docs_snapshot"
	ModelPricingSourceModelsDevSnapshot    ModelPricingSource = "models_dev_snapshot"
)

type ModelRegistrySource string

const (
	ModelRegistrySourceEmbedded ModelRegistrySource = "embedded"
	ModelRegistrySourceTestdata ModelRegistrySource = "testdata"
)

type ModelRegistryFreshness string

const (
	ModelRegistryFreshnessCurrent ModelRegistryFreshness = "current"
	ModelRegistryFreshnessStale   ModelRegistryFreshness = "stale"
)

type ModelPricing struct {
	Status                  ModelFactStatus
	InputUSDPerMillion      float64
	OutputUSDPerMillion     float64
	CacheReadUSDPerMillion  float64
	CacheWriteUSDPerMillion float64
	Source                  ModelPricingSource
	Version                 string
}

func (p ModelPricing) Known() bool {
	return p.Status == ModelFactKnown
}

type ModelCapabilityFlags struct {
	Status           ModelFactStatus
	Tools            ModelCapabilityFlag
	Vision           ModelCapabilityFlag
	Reasoning        ModelCapabilityFlag
	PDF              ModelCapabilityFlag
	AudioInput       ModelCapabilityFlag
	StructuredOutput ModelCapabilityFlag
	OpenWeights      ModelCapabilityFlag
}

func (c ModelCapabilityFlags) Known() bool {
	return c.Status == ModelFactKnown
}

type ModelRegistrySnapshot struct {
	Source    ModelRegistrySource
	Freshness ModelRegistryFreshness
	Version   string
	Reason    string
}

type ModelRegistryQuery struct {
	Provider string
	Model    string
}

type ModelRegistryKey struct {
	Provider string
	Model    string
}

type ModelRegistryEntry struct {
	Provider         string
	Model            string
	ProviderFamily   string
	ModelFamily      string
	RawContextWindow int
	MaxOutputTokens  int
	Pricing          ModelPricing
	Capabilities     ModelCapabilityFlags
}

type ModelMetadataResult struct {
	Found bool
	ModelRegistryEntry
	Registry ModelRegistrySnapshot
}

type ModelRegistry struct {
	snapshot ModelRegistrySnapshot
	entries  map[ModelRegistryKey]ModelRegistryEntry
}

func NewStaticModelRegistry(snapshot ModelRegistrySnapshot, entries []ModelRegistryEntry) ModelRegistry {
	registry := ModelRegistry{
		snapshot: normalizeModelRegistrySnapshot(snapshot),
		entries:  make(map[ModelRegistryKey]ModelRegistryEntry, len(entries)),
	}
	for _, entry := range entries {
		entry = normalizeModelRegistryEntry(entry)
		key := ModelRegistryKey{Provider: entry.Provider, Model: entry.Model}
		if key.Provider == "" || key.Model == "" {
			continue
		}
		registry.entries[key] = entry
	}
	return registry
}

func DefaultModelRegistry() ModelRegistry {
	return defaultModelRegistry
}

func LookupModelMetadata(query ModelRegistryQuery) ModelMetadataResult {
	return DefaultModelRegistry().Lookup(query)
}

func (r ModelRegistry) Lookup(query ModelRegistryQuery) ModelMetadataResult {
	result := ModelMetadataResult{
		Registry: r.Snapshot(),
		Found:    false,
		ModelRegistryEntry: ModelRegistryEntry{
			Pricing:      unknownModelPricing(),
			Capabilities: unknownModelCapabilities(),
		},
	}
	key := ModelRegistryKey{
		Provider: normalizeModelContextProvider(query.Provider),
		Model:    normalizeModelContextText(query.Model),
	}
	if key.Provider == "" || key.Model == "" {
		return result
	}
	entry, ok := r.entries[key]
	if !ok {
		return result
	}
	result.Found = true
	result.ModelRegistryEntry = entry
	return result
}

func (r ModelRegistry) Snapshot() ModelRegistrySnapshot {
	return normalizeModelRegistrySnapshot(r.snapshot)
}

func normalizeModelRegistrySnapshot(snapshot ModelRegistrySnapshot) ModelRegistrySnapshot {
	if snapshot.Source == "" {
		snapshot.Source = ModelRegistrySourceEmbedded
	}
	if snapshot.Freshness == "" {
		snapshot.Freshness = ModelRegistryFreshnessCurrent
	}
	return snapshot
}

func normalizeModelRegistryEntry(entry ModelRegistryEntry) ModelRegistryEntry {
	entry.Provider = normalizeModelContextProvider(entry.Provider)
	entry.Model = normalizeModelContextText(entry.Model)
	if entry.ProviderFamily == "" {
		entry.ProviderFamily = entry.Provider
	}
	entry.Pricing = normalizeModelPricing(entry.Pricing)
	entry.Capabilities = normalizeModelCapabilities(entry.Capabilities)
	return entry
}

func normalizeModelPricing(pricing ModelPricing) ModelPricing {
	if pricing.Status == "" {
		pricing.Status = ModelFactUnknown
	}
	if pricing.Source == "" {
		pricing.Source = ModelPricingSourceNone
	}
	return pricing
}

func normalizeModelCapabilities(capabilities ModelCapabilityFlags) ModelCapabilityFlags {
	if capabilities.Status == "" {
		capabilities.Status = ModelFactUnknown
	}
	capabilities.Tools = normalizeModelCapabilityFlag(capabilities.Tools)
	capabilities.Vision = normalizeModelCapabilityFlag(capabilities.Vision)
	capabilities.Reasoning = normalizeModelCapabilityFlag(capabilities.Reasoning)
	capabilities.PDF = normalizeModelCapabilityFlag(capabilities.PDF)
	capabilities.AudioInput = normalizeModelCapabilityFlag(capabilities.AudioInput)
	capabilities.StructuredOutput = normalizeModelCapabilityFlag(capabilities.StructuredOutput)
	capabilities.OpenWeights = normalizeModelCapabilityFlag(capabilities.OpenWeights)
	return capabilities
}

func normalizeModelCapabilityFlag(flag ModelCapabilityFlag) ModelCapabilityFlag {
	if flag == "" {
		return ModelCapabilityUnknown
	}
	return flag
}

func unknownModelPricing() ModelPricing {
	return normalizeModelPricing(ModelPricing{Status: ModelFactUnknown})
}

func unknownModelCapabilities() ModelCapabilityFlags {
	return normalizeModelCapabilities(ModelCapabilityFlags{Status: ModelFactUnknown})
}

func knownModelPricing(input, output, cacheRead, cacheWrite float64, source ModelPricingSource, version string) ModelPricing {
	return ModelPricing{
		Status:                  ModelFactKnown,
		InputUSDPerMillion:      input,
		OutputUSDPerMillion:     output,
		CacheReadUSDPerMillion:  cacheRead,
		CacheWriteUSDPerMillion: cacheWrite,
		Source:                  source,
		Version:                 version,
	}
}

func knownModelCapabilities(tools, vision, reasoning, pdf, audioInput, structuredOutput, openWeights ModelCapabilityFlag) ModelCapabilityFlags {
	return normalizeModelCapabilities(ModelCapabilityFlags{
		Status:           ModelFactKnown,
		Tools:            tools,
		Vision:           vision,
		Reasoning:        reasoning,
		PDF:              pdf,
		AudioInput:       audioInput,
		StructuredOutput: structuredOutput,
		OpenWeights:      openWeights,
	})
}

var defaultModelRegistry = NewStaticModelRegistry(ModelRegistrySnapshot{
	Source:    ModelRegistrySourceEmbedded,
	Freshness: ModelRegistryFreshnessCurrent,
	Version:   "models.dev-fixture-2026-04-25",
}, []ModelRegistryEntry{
	{
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		ProviderFamily:   "openai",
		ModelFamily:      "gpt-4o",
		RawContextWindow: 128_000,
		MaxOutputTokens:  16_384,
		Pricing: knownModelPricing(
			0.15,
			0.60,
			0.075,
			0,
			ModelPricingSourceOfficialDocsSnapshot,
			"openai-pricing-2026-03-16",
		),
		Capabilities: knownModelCapabilities(
			ModelCapabilitySupported,
			ModelCapabilitySupported,
			ModelCapabilityUnsupported,
			ModelCapabilityUnsupported,
			ModelCapabilityUnsupported,
			ModelCapabilitySupported,
			ModelCapabilityUnsupported,
		),
	},
	{
		Provider:         "anthropic",
		Model:            "claude-opus-4-20250514",
		ProviderFamily:   "anthropic",
		ModelFamily:      "claude-opus-4",
		RawContextWindow: 200_000,
		MaxOutputTokens:  32_000,
		Pricing: knownModelPricing(
			15.00,
			75.00,
			1.50,
			18.75,
			ModelPricingSourceOfficialDocsSnapshot,
			"anthropic-prompt-caching-2026-03-16",
		),
		Capabilities: knownModelCapabilities(
			ModelCapabilitySupported,
			ModelCapabilitySupported,
			ModelCapabilitySupported,
			ModelCapabilityUnsupported,
			ModelCapabilityUnsupported,
			ModelCapabilityUnsupported,
			ModelCapabilityUnsupported,
		),
	},
	{
		Provider:         "openai-codex",
		Model:            "gpt-5.5",
		ProviderFamily:   "openai",
		ModelFamily:      "gpt-5",
		RawContextWindow: 1_050_000,
		MaxOutputTokens:  128_000,
		Pricing:          unknownModelPricing(),
		Capabilities: knownModelCapabilities(
			ModelCapabilitySupported,
			ModelCapabilitySupported,
			ModelCapabilitySupported,
			ModelCapabilityUnsupported,
			ModelCapabilityUnsupported,
			ModelCapabilitySupported,
			ModelCapabilityUnsupported,
		),
	},
})
