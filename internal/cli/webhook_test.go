package cli

import (
	"errors"
	"testing"
)

func TestNormalizeWebhookURL_TrimsAndCanonicalizes(t *testing.T) {
	got, err := NormalizeWebhookURL("  HTTPS://Example.COM/Hooks/x/  ")
	if err != nil {
		t.Fatalf("NormalizeWebhookURL returned error: %v", err)
	}
	const want = "https://example.com/Hooks/x"
	if got != want {
		t.Fatalf("NormalizeWebhookURL = %q, want %q", got, want)
	}
}

func TestNormalizeWebhookURL_StripsTrailingSlashOnRoot(t *testing.T) {
	got, err := NormalizeWebhookURL("https://example.com/")
	if err != nil {
		t.Fatalf("NormalizeWebhookURL returned error: %v", err)
	}
	const want = "https://example.com"
	if got != want {
		t.Fatalf("NormalizeWebhookURL = %q, want %q", got, want)
	}
}

func TestNormalizeWebhookURL_PreservesQueryAndFragment(t *testing.T) {
	got, err := NormalizeWebhookURL("https://example.com/x/?a=1#f")
	if err != nil {
		t.Fatalf("NormalizeWebhookURL returned error: %v", err)
	}
	const want = "https://example.com/x?a=1#f"
	if got != want {
		t.Fatalf("NormalizeWebhookURL = %q, want %q", got, want)
	}
}

func TestNormalizeWebhookURL_RejectsEmpty(t *testing.T) {
	got, err := NormalizeWebhookURL("   ")
	if got != "" {
		t.Fatalf("NormalizeWebhookURL = %q, want empty string", got)
	}
	if !errors.Is(err, ErrWebhookURLEmpty) {
		t.Fatalf("NormalizeWebhookURL err = %v, want ErrWebhookURLEmpty", err)
	}
}

func TestNormalizeWebhookURL_RejectsNonHTTP(t *testing.T) {
	cases := []string{
		"ftp://example.com",
		"javascript:alert(1)",
	}
	for _, raw := range cases {
		got, err := NormalizeWebhookURL(raw)
		if got != "" {
			t.Errorf("NormalizeWebhookURL(%q) = %q, want empty string", raw, got)
		}
		if !errors.Is(err, ErrWebhookURLBadScheme) {
			t.Errorf("NormalizeWebhookURL(%q) err = %v, want ErrWebhookURLBadScheme", raw, err)
		}
	}
}

func TestNormalizeWebhookURL_RejectsUserInfo(t *testing.T) {
	got, err := NormalizeWebhookURL("https://user:pass@example.com/x")
	if got != "" {
		t.Fatalf("NormalizeWebhookURL = %q, want empty string", got)
	}
	if !errors.Is(err, ErrWebhookURLUserInfoForbidden) {
		t.Fatalf("NormalizeWebhookURL err = %v, want ErrWebhookURLUserInfoForbidden", err)
	}
}
