package hermes

import (
	"context"
	"strings"
)

const titlePrompt = "Generate a short, descriptive title (3-7 words) for a conversation that starts with the following exchange. The title should capture the main topic or intent. Return ONLY the title text, nothing else. No quotes, no punctuation at the end, no prefixes."

const (
	defaultTitleMaxContentChars = 500
	defaultTitleMaxChars        = 80
	defaultTitleMaxTokens       = 500
	defaultTitleTemperature     = 0.3
)

type TitleStatus string

const (
	TitleStatusGenerated        TitleStatus = "auto_title_generated"
	TitleStatusAutoTitleSkipped TitleStatus = "auto_title_skipped"
	TitleStatusBlankResult      TitleStatus = "title_blank_result"
	TitleStatusProviderFailed   TitleStatus = "title_provider_failed"
)

type TitleMessage struct {
	Role    string
	Content string
}

type TitleRequest struct {
	History            []TitleMessage
	MaxHistoryMessages int
	MaxContentChars    int
	MaxTitleChars      int
}

type TitleModelMessage struct {
	Role    string
	Content string
}

type TitleModelRequest struct {
	Messages    []TitleModelMessage
	MaxTokens   int
	Temperature float64
}

type TitleModelFunc func(context.Context, TitleModelRequest) (string, error)

type TitleResult struct {
	Title    string
	Status   TitleStatus
	Evidence TitleEvidence
	Err      error
}

type TitleEvidence struct {
	Kind    TitleStatus
	Message string
}

type TitleProviderError struct {
	Kind TitleStatus
	Err  error
}

func (e *TitleProviderError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return string(e.Kind)
	}
	return string(e.Kind) + ": " + e.Err.Error()
}

func (e *TitleProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func GenerateTitle(ctx context.Context, req TitleRequest, model TitleModelFunc) TitleResult {
	if len(boundedTitleHistory(req)) == 0 {
		return TitleResult{
			Status: TitleStatusAutoTitleSkipped,
			Evidence: TitleEvidence{
				Kind:    TitleStatusAutoTitleSkipped,
				Message: "auto-title skipped: empty title history",
			},
		}
	}

	modelReq := TitleModelRequest{
		Messages: []TitleModelMessage{
			{Role: "system", Content: titlePrompt},
			{Role: "user", Content: buildTitlePrompt(req)},
		},
		MaxTokens:   defaultTitleMaxTokens,
		Temperature: defaultTitleTemperature,
	}
	title, err := model(ctx, modelReq)
	if err != nil {
		return TitleResult{
			Status: TitleStatusProviderFailed,
			Evidence: TitleEvidence{
				Kind:    TitleStatusProviderFailed,
				Message: "title provider failed: " + err.Error(),
			},
			Err: &TitleProviderError{Kind: TitleStatusProviderFailed, Err: err},
		}
	}
	title = cleanTitleCandidate(title, req.MaxTitleChars)
	if title == "" {
		return TitleResult{
			Status: TitleStatusBlankResult,
			Evidence: TitleEvidence{
				Kind:    TitleStatusBlankResult,
				Message: "title model returned a blank title",
			},
		}
	}
	return TitleResult{
		Title:  title,
		Status: TitleStatusGenerated,
	}
}

func buildTitlePrompt(req TitleRequest) string {
	history := boundedTitleHistory(req)
	maxContentChars := req.MaxContentChars
	if maxContentChars <= 0 {
		maxContentChars = defaultTitleMaxContentChars
	}

	var b strings.Builder
	for i, msg := range history {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(titleRoleLabel(msg.Role))
		b.WriteString(": ")
		b.WriteString(truncateTitleText(msg.Content, maxContentChars))
	}
	return b.String()
}

func boundedTitleHistory(req TitleRequest) []TitleMessage {
	filtered := make([]TitleMessage, 0, len(req.History))
	for _, msg := range req.History {
		switch msg.Role {
		case "user", "assistant":
			filtered = append(filtered, msg)
		}
	}
	if req.MaxHistoryMessages <= 0 || len(filtered) <= req.MaxHistoryMessages {
		return filtered
	}
	return filtered[len(filtered)-req.MaxHistoryMessages:]
}

func titleRoleLabel(role string) string {
	if role == "assistant" {
		return "Assistant"
	}
	return "User"
}

func truncateTitleText(s string, maxChars int) string {
	if maxChars <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}

func cleanTitleCandidate(candidate string, maxChars int) string {
	title := strings.TrimSpace(candidate)
	title = strings.Trim(title, `"'`)
	if strings.HasPrefix(strings.ToLower(title), "title:") {
		title = strings.TrimSpace(title[len("title:"):])
	}
	title = strings.Join(strings.Fields(title), " ")

	if maxChars <= 0 {
		maxChars = defaultTitleMaxChars
	}
	runes := []rune(title)
	if len(runes) <= maxChars {
		return title
	}
	if maxChars <= len("...") {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-len("...")]) + "..."
}
