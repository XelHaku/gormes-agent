package hermes

import (
	"errors"
	"testing"
)

func TestResolveDisplayContextLengthUsesDefaultProviderCaps(t *testing.T) {
	got := ResolveDisplayContextLength(ModelContextQuery{
		Provider: "openai-codex",
		Model:    "gpt-5.5",
		ModelInfo: ModelContextMetadata{
			ContextWindow: 1_050_000,
		},
	})

	if got.ContextLength != 272_000 {
		t.Fatalf("ContextLength = %d, want Codex OAuth cap 272000", got.ContextLength)
	}
	if got.Source != ModelContextSourceProviderCap {
		t.Fatalf("Source = %q, want %q", got.Source, ModelContextSourceProviderCap)
	}
	if !got.Known() {
		t.Fatal("Known() = false, want true")
	}
}

func TestModelContextResolverPrefersProviderCapsOverModelInfo(t *testing.T) {
	resolver := NewModelContextResolver(StaticModelContextCaps{
		ModelContextKey{Provider: "openai-codex", Model: "gpt-5.5"}:             272_000,
		ModelContextKey{Provider: "copilot", Model: "claude-opus-4.6"}:          128_000,
		ModelContextKey{Provider: "github-copilot", Model: "claude-opus-4.6"}:   128_000,
		ModelContextKey{Provider: "nous", Model: "claude-opus-4-6"}:             200_000,
		ModelContextKey{Provider: "nous", Model: "anthropic/claude-opus-4.6"}:   200_000,
		ModelContextKey{Provider: "openai-codex", Model: "gpt-5.4-mini"}:        272_000,
		ModelContextKey{Provider: "copilot-acp", Model: "claude-sonnet-4.6"}:    128_000,
		ModelContextKey{Provider: "github-copilot", Model: "claude-sonnet-4.6"}: 128_000,
	})

	cases := []struct {
		name      string
		provider  string
		model     string
		modelInfo int
		want      int
	}{
		{
			name:      "codex oauth cap beats raw openai window",
			provider:  "openai-codex",
			model:     "gpt-5.5",
			modelInfo: 1_050_000,
			want:      272_000,
		},
		{
			name:      "copilot cap beats claude models.dev fallback",
			provider:  "copilot",
			model:     "claude-opus-4.6",
			modelInfo: 1_000_000,
			want:      128_000,
		},
		{
			name:      "nous cap beats direct anthropic fallback",
			provider:  "nous",
			model:     "claude-opus-4-6",
			modelInfo: 1_000_000,
			want:      200_000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolver.Resolve(ModelContextQuery{
				Provider: tc.provider,
				Model:    tc.model,
				ModelInfo: ModelContextMetadata{
					ContextWindow: tc.modelInfo,
				},
			})

			if got.ContextLength != tc.want {
				t.Fatalf("ContextLength = %d, want %d", got.ContextLength, tc.want)
			}
			if got.Source != ModelContextSourceProviderCap {
				t.Fatalf("Source = %q, want %q", got.Source, ModelContextSourceProviderCap)
			}
			if !got.Known() {
				t.Fatal("Known() = false, want true")
			}
		})
	}
}

func TestModelContextResolverFallsBackToModelInfo(t *testing.T) {
	resolver := NewModelContextResolver(ModelContextLookupFunc(func(ModelContextQuery) (int, bool, error) {
		return 0, false, errors.New("provider metadata unavailable")
	}))

	got := resolver.Resolve(ModelContextQuery{
		Provider: "some-provider",
		Model:    "some-model",
		ModelInfo: ModelContextMetadata{
			ContextWindow: 1_048_576,
		},
	})

	if got.ContextLength != 1_048_576 {
		t.Fatalf("ContextLength = %d, want model metadata fallback 1048576", got.ContextLength)
	}
	if got.Source != ModelContextSourceModelsDev {
		t.Fatalf("Source = %q, want %q", got.Source, ModelContextSourceModelsDev)
	}
	if !got.Known() {
		t.Fatal("Known() = false, want true")
	}
}

func TestModelContextResolverReportsUnknownWhenNoSourcesHaveContext(t *testing.T) {
	resolver := NewModelContextResolver(ModelContextLookupFunc(func(ModelContextQuery) (int, bool, error) {
		return 0, false, errors.New("provider metadata unavailable")
	}))

	got := resolver.Resolve(ModelContextQuery{
		Provider: "unknown-provider",
		Model:    "unknown-model",
	})

	if got.ContextLength != 0 {
		t.Fatalf("ContextLength = %d, want 0", got.ContextLength)
	}
	if got.Source != ModelContextSourceUnknown {
		t.Fatalf("Source = %q, want %q", got.Source, ModelContextSourceUnknown)
	}
	if got.Known() {
		t.Fatal("Known() = true, want false")
	}
}
