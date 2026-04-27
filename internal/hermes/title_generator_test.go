package hermes

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTitlePrompt_BuildsFromBoundedHistory(t *testing.T) {
	t.Parallel()

	var captured TitleModelRequest
	calls := 0
	result := GenerateTitle(context.Background(), TitleRequest{
		History: []TitleMessage{
			{Role: "system", Content: "system content must not enter title prompt"},
			{Role: "user", Content: "old user must not enter title prompt"},
			{Role: "assistant", Content: "old assistant must not enter title prompt"},
			{Role: "tool", Content: "tool output must not enter title prompt"},
			{Role: "user", Content: "0123456789ABCDEFGHIJ"},
			{Role: "assistant", Content: "abcdefghijKLMNOPQRST"},
		},
		MaxHistoryMessages: 2,
		MaxContentChars:    10,
	}, func(ctx context.Context, req TitleModelRequest) (string, error) {
		calls++
		captured = req
		return "Bounded Prompt Title", nil
	})

	if result.Status != TitleStatusGenerated {
		t.Fatalf("Status = %q; want %q (result: %+v)", result.Status, TitleStatusGenerated, result)
	}
	if result.Title != "Bounded Prompt Title" {
		t.Fatalf("Title = %q; want %q", result.Title, "Bounded Prompt Title")
	}
	if calls != 1 {
		t.Fatalf("model calls = %d; want 1", calls)
	}
	if len(captured.Messages) != 2 {
		t.Fatalf("model messages = %#v; want system and user prompt messages", captured.Messages)
	}
	if captured.Messages[0].Role != "system" {
		t.Fatalf("first model message role = %q; want system", captured.Messages[0].Role)
	}
	if !strings.Contains(captured.Messages[0].Content, "Generate a short, descriptive title") {
		t.Fatalf("system prompt = %q; want Hermes-compatible title instruction", captured.Messages[0].Content)
	}
	if captured.Messages[1].Role != "user" {
		t.Fatalf("second model message role = %q; want user", captured.Messages[1].Role)
	}

	userPrompt := captured.Messages[1].Content
	for _, forbidden := range []string{
		"system content",
		"old user",
		"old assistant",
		"tool output",
		"ABCDEFGHIJ",
		"KLMNOPQRST",
	} {
		if strings.Contains(userPrompt, forbidden) {
			t.Fatalf("prompt %q contains forbidden content %q", userPrompt, forbidden)
		}
	}
	for _, required := range []string{
		"User: 0123456789",
		"Assistant: abcdefghij",
	} {
		if !strings.Contains(userPrompt, required) {
			t.Fatalf("prompt %q missing required bounded content %q", userPrompt, required)
		}
	}
	if captured.MaxTokens != 500 {
		t.Fatalf("MaxTokens = %d; want 500", captured.MaxTokens)
	}
	if captured.Temperature != 0.3 {
		t.Fatalf("Temperature = %v; want 0.3", captured.Temperature)
	}
}

func TestTitleGenerator_TruncatesAndCleansCandidate(t *testing.T) {
	t.Parallel()

	result := GenerateTitle(context.Background(), TitleRequest{
		History: []TitleMessage{
			{Role: "user", Content: "why did the title fail"},
			{Role: "assistant", Content: "because the provider returned noisy output"},
		},
		MaxTitleChars: 27,
	}, func(ctx context.Context, req TitleModelRequest) (string, error) {
		return " \n\t\"Title: Kubernetes Pod Debugging and Provider Failure Visibility\nAcross Gateways\"  \n", nil
	})

	if result.Status != TitleStatusGenerated {
		t.Fatalf("Status = %q; want %q (result: %+v)", result.Status, TitleStatusGenerated, result)
	}
	if result.Title != "Kubernetes Pod Debugging..." {
		t.Fatalf("Title = %q; want %q", result.Title, "Kubernetes Pod Debugging...")
	}
	if len([]rune(result.Title)) != 27 {
		t.Fatalf("Title length = %d; want 27", len([]rune(result.Title)))
	}
}

func TestTitleGenerator_EmptyHistorySkipsModel(t *testing.T) {
	t.Parallel()

	calls := 0
	result := GenerateTitle(context.Background(), TitleRequest{}, func(ctx context.Context, req TitleModelRequest) (string, error) {
		calls++
		return "should not be used", nil
	})

	if calls != 0 {
		t.Fatalf("model calls = %d; want 0", calls)
	}
	if result.Title != "" {
		t.Fatalf("Title = %q; want empty title", result.Title)
	}
	if result.Status != TitleStatusAutoTitleSkipped {
		t.Fatalf("Status = %q; want %q", result.Status, TitleStatusAutoTitleSkipped)
	}
	if result.Evidence.Kind != TitleStatusAutoTitleSkipped {
		t.Fatalf("Evidence.Kind = %q; want %q", result.Evidence.Kind, TitleStatusAutoTitleSkipped)
	}
	if result.Evidence.Message == "" {
		t.Fatalf("Evidence.Message is empty; want operator-visible skip cause")
	}
}

func TestTitleGenerator_BlankModelOutput(t *testing.T) {
	t.Parallel()

	result := GenerateTitle(context.Background(), TitleRequest{
		History: []TitleMessage{
			{Role: "user", Content: "name this session"},
			{Role: "assistant", Content: "the model returns a blank title"},
		},
	}, func(ctx context.Context, req TitleModelRequest) (string, error) {
		return " \n\t\"\"  \n", nil
	})

	if result.Title != "" {
		t.Fatalf("Title = %q; want empty title", result.Title)
	}
	if result.Status != TitleStatusBlankResult {
		t.Fatalf("Status = %q; want %q", result.Status, TitleStatusBlankResult)
	}
	if result.Evidence.Kind != TitleStatusBlankResult {
		t.Fatalf("Evidence.Kind = %q; want %q", result.Evidence.Kind, TitleStatusBlankResult)
	}
	if result.Evidence.Message == "" {
		t.Fatalf("Evidence.Message is empty; want operator-visible blank result cause")
	}
}

func TestTitleGenerator_ProviderFailureReturnsTypedEvidence(t *testing.T) {
	t.Parallel()

	providerErr := errors.New("openrouter 402: credits exhausted")
	result := GenerateTitle(context.Background(), TitleRequest{
		History: []TitleMessage{
			{Role: "user", Content: "please title this failing run"},
			{Role: "assistant", Content: "the provider will fail"},
		},
	}, func(ctx context.Context, req TitleModelRequest) (string, error) {
		return "", providerErr
	})

	if result.Title != "" {
		t.Fatalf("Title = %q; want empty title", result.Title)
	}
	if result.Status != TitleStatusProviderFailed {
		t.Fatalf("Status = %q; want %q", result.Status, TitleStatusProviderFailed)
	}
	if result.Evidence.Kind != TitleStatusProviderFailed {
		t.Fatalf("Evidence.Kind = %q; want %q", result.Evidence.Kind, TitleStatusProviderFailed)
	}
	if !strings.Contains(result.Evidence.Message, "openrouter 402") {
		t.Fatalf("Evidence.Message = %q; want provider failure detail", result.Evidence.Message)
	}
	var typed *TitleProviderError
	if !errors.As(result.Err, &typed) {
		t.Fatalf("Err = %T %[1]v; want *TitleProviderError", result.Err)
	}
	if typed.Kind != TitleStatusProviderFailed {
		t.Fatalf("TitleProviderError.Kind = %q; want %q", typed.Kind, TitleStatusProviderFailed)
	}
	if !errors.Is(result.Err, providerErr) {
		t.Fatalf("Err = %v; want wrapping provider error %v", result.Err, providerErr)
	}
}
