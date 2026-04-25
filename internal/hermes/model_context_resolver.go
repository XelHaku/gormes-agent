package hermes

import (
	"sort"
	"strings"
)

type ModelContextSource string

const (
	ModelContextSourceProviderCap ModelContextSource = "provider_cap"
	ModelContextSourceModelsDev   ModelContextSource = "models_dev"
	ModelContextSourceUnknown     ModelContextSource = "unknown"
)

type ModelContextMetadata struct {
	ContextWindow int
}

type ModelContextQuery struct {
	Provider  string
	Model     string
	BaseURL   string
	ModelInfo ModelContextMetadata
}

type ModelContextResolution struct {
	Provider            string
	Model               string
	ContextLength       int
	Source              ModelContextSource
	ProviderLookupError string
}

func (r ModelContextResolution) Known() bool {
	return r.ContextLength > 0 && r.Source != ModelContextSourceUnknown
}

type ModelContextLookup interface {
	LookupModelContext(ModelContextQuery) (int, bool, error)
}

type ModelContextLookupFunc func(ModelContextQuery) (int, bool, error)

func (fn ModelContextLookupFunc) LookupModelContext(query ModelContextQuery) (int, bool, error) {
	return fn(query)
}

type ModelContextKey struct {
	Provider string
	Model    string
}

type StaticModelContextCaps map[ModelContextKey]int

func (caps StaticModelContextCaps) LookupModelContext(query ModelContextQuery) (int, bool, error) {
	if len(caps) == 0 {
		return 0, false, nil
	}

	provider := normalizeModelContextProvider(query.Provider)
	model := normalizeModelContextText(query.Model)
	if provider == "" || model == "" {
		return 0, false, nil
	}

	type candidate struct {
		model string
		value int
	}
	var candidates []candidate
	for key, value := range caps {
		if value <= 0 || normalizeModelContextProvider(key.Provider) != provider {
			continue
		}
		keyModel := normalizeModelContextText(key.Model)
		if keyModel == "" {
			continue
		}
		if model == keyModel {
			return value, true, nil
		}
		if strings.Contains(model, keyModel) {
			candidates = append(candidates, candidate{model: keyModel, value: value})
		}
	}
	if len(candidates) == 0 {
		return 0, false, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i].model) > len(candidates[j].model)
	})
	return candidates[0].value, true, nil
}

type ModelContextResolver struct {
	providerCaps ModelContextLookup
}

func NewModelContextResolver(providerCaps ModelContextLookup) ModelContextResolver {
	return ModelContextResolver{providerCaps: providerCaps}
}

func DefaultModelContextResolver() ModelContextResolver {
	return NewModelContextResolver(defaultModelContextCaps)
}

func ResolveDisplayContextLength(query ModelContextQuery) ModelContextResolution {
	return DefaultModelContextResolver().Resolve(query)
}

func (r ModelContextResolver) Resolve(query ModelContextQuery) ModelContextResolution {
	result := ModelContextResolution{
		Provider: query.Provider,
		Model:    query.Model,
		Source:   ModelContextSourceUnknown,
	}

	if r.providerCaps != nil {
		length, ok, err := r.providerCaps.LookupModelContext(query)
		if err != nil {
			result.ProviderLookupError = err.Error()
		}
		if ok && length > 0 {
			result.ContextLength = length
			result.Source = ModelContextSourceProviderCap
			return result
		}
	}

	if query.ModelInfo.ContextWindow > 0 {
		result.ContextLength = query.ModelInfo.ContextWindow
		result.Source = ModelContextSourceModelsDev
		return result
	}

	return result
}

var defaultModelContextCaps = StaticModelContextCaps{
	// ChatGPT Codex OAuth caps these slugs below the raw OpenAI API windows.
	ModelContextKey{Provider: "openai-codex", Model: "gpt-5.1-codex-max"}:  272_000,
	ModelContextKey{Provider: "openai-codex", Model: "gpt-5.1-codex-mini"}: 272_000,
	ModelContextKey{Provider: "openai-codex", Model: "gpt-5.3-codex"}:      272_000,
	ModelContextKey{Provider: "openai-codex", Model: "gpt-5.2-codex"}:      272_000,
	ModelContextKey{Provider: "openai-codex", Model: "gpt-5.4-mini"}:       272_000,
	ModelContextKey{Provider: "openai-codex", Model: "gpt-5.5"}:            272_000,
	ModelContextKey{Provider: "openai-codex", Model: "gpt-5.4"}:            272_000,
	ModelContextKey{Provider: "openai-codex", Model: "gpt-5.2"}:            272_000,
	ModelContextKey{Provider: "openai-codex", Model: "gpt-5"}:              272_000,

	// Provider-enforced fixture caps for model families whose raw vendor
	// metadata is larger than the context actually usable through the provider.
	ModelContextKey{Provider: "copilot", Model: "claude-opus-4.6"}:   128_000,
	ModelContextKey{Provider: "copilot", Model: "claude-sonnet-4.6"}: 128_000,
	ModelContextKey{Provider: "nous", Model: "claude-opus-4-6"}:      200_000,
	ModelContextKey{Provider: "nous", Model: "claude-opus-4.6"}:      200_000,
}

func normalizeModelContextProvider(provider string) string {
	switch normalizeModelContextText(provider) {
	case "codex", "openai-codex":
		return "openai-codex"
	case "copilot", "copilot-acp", "github", "github-copilot", "github-models":
		return "copilot"
	default:
		return normalizeModelContextText(provider)
	}
}

func normalizeModelContextText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
