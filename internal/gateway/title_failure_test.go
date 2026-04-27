package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

func TestTitleFailureCallback_GatewayWarningWithoutTitlePersistence(t *testing.T) {
	t.Parallel()

	providerErr := errors.New("openrouter 402: credits exhausted")
	persistedTitles := 0
	var userVisibleWarning string

	result := hermes.GenerateTitle(context.Background(), hermes.TitleRequest{
		History: []hermes.TitleMessage{
			{Role: "user", Content: "please name this turn"},
			{Role: "assistant", Content: "foreground answer survived"},
		},
		FailureCallback: func(ctx context.Context, evidence hermes.TitleEvidence) error {
			userVisibleWarning = FormatErrorPlain(kernel.RenderFrame{
				Phase:     kernel.PhaseIdle,
				LastError: evidence.Message,
			})
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
	if !strings.Contains(userVisibleWarning, "openrouter 402") {
		t.Fatalf("userVisibleWarning = %q; want provider failure detail", userVisibleWarning)
	}
	if result.Title != "" {
		persistedTitles++
	}
	if persistedTitles != 0 {
		t.Fatalf("persisted titles = %d; want no title persistence on provider failure", persistedTitles)
	}
}
