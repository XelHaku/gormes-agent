package hermes

type ContextCompressorAuxiliaryBudgetConfig struct {
	Model                     string
	ContextLength             int
	ThresholdPercent          float64
	SummaryTargetRatio        float64
	ToolDescriptors           []ToolDescriptor
	AuxiliaryProvider         string
	AuxiliaryModel            string
	AuxiliaryBaseURL          string
	AuxiliaryRawContextLength int
	ContextResolver           ModelContextResolver
}

type ContextCompressorAuxiliaryBudget struct {
	Budget           *ContextCompressorBudget
	AuxiliaryContext ModelContextResolution
}

func NewContextCompressorAuxiliaryBudget(config ContextCompressorAuxiliaryBudgetConfig) ContextCompressorAuxiliaryBudget {
	resolver := config.ContextResolver
	if resolver.providerCaps == nil {
		resolver = DefaultModelContextResolver()
	}

	auxiliaryContext := resolver.Resolve(ModelContextQuery{
		Provider: config.AuxiliaryProvider,
		Model:    config.AuxiliaryModel,
		BaseURL:  config.AuxiliaryBaseURL,
		ModelInfo: ModelContextMetadata{
			ContextWindow: config.AuxiliaryRawContextLength,
		},
	})

	budget := NewContextCompressorBudget(ContextCompressorBudgetConfig{
		Model:                       config.Model,
		ContextLength:               config.ContextLength,
		ThresholdPercent:            config.ThresholdPercent,
		SummaryTargetRatio:          config.SummaryTargetRatio,
		AuxiliaryContextLength:      auxiliaryContext.ContextLength,
		AuxiliaryContextSource:      auxiliaryContext.Source,
		AuxiliaryContextLookupError: auxiliaryContext.ProviderLookupError,
		ToolDescriptors:             config.ToolDescriptors,
	})

	return ContextCompressorAuxiliaryBudget{
		Budget:           budget,
		AuxiliaryContext: auxiliaryContext,
	}
}
