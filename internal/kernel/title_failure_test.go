package kernel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
)

func TestTitleFailureCallback_KernelAuxiliaryWarningFrame(t *testing.T) {
	t.Parallel()

	providerErr := errors.New("openrouter 402: credits exhausted")
	var warning RenderFrame
	foreground := RenderFrame{
		Phase: PhaseIdle,
		History: []hermes.Message{
			{Role: "assistant", Content: "foreground answer survived"},
		},
	}

	result := hermes.GenerateTitle(context.Background(), hermes.TitleRequest{
		History: []hermes.TitleMessage{
			{Role: "user", Content: "please name this turn"},
			{Role: "assistant", Content: "foreground answer survived"},
		},
		FailureCallback: func(ctx context.Context, evidence hermes.TitleEvidence) error {
			warning = RenderFrame{
				Phase:      PhaseIdle,
				StatusText: string(evidence.Kind),
				LastError:  evidence.Message,
				History:    foreground.History,
			}
			return nil
		},
	}, func(ctx context.Context, req hermes.TitleModelRequest) (string, error) {
		return "", providerErr
	})

	if result.Title != "" {
		t.Fatalf("Title = %q; want empty title", result.Title)
	}
	if result.Status != hermes.TitleStatusProviderFailed {
		t.Fatalf("Status = %q; want %q", result.Status, hermes.TitleStatusProviderFailed)
	}
	if warning.StatusText != string(hermes.TitleStatusProviderFailed) {
		t.Fatalf("warning StatusText = %q; want %q", warning.StatusText, hermes.TitleStatusProviderFailed)
	}
	if !strings.Contains(warning.LastError, "openrouter 402") {
		t.Fatalf("warning LastError = %q; want provider failure detail", warning.LastError)
	}
	if got := warning.History[len(warning.History)-1].Content; got != "foreground answer survived" {
		t.Fatalf("foreground answer = %q; want preserved answer", got)
	}
}
