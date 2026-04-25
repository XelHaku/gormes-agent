package weixin

import "testing"

func TestDecideRuntime_LongPollDefaults(t *testing.T) {
	plan, err := DecideRuntime(RuntimeConfig{
		AccountID: "account-1",
		Token:     "token-1",
	})
	if err != nil {
		t.Fatalf("DecideRuntime() error = %v, want nil", err)
	}

	if plan.Ingress.Mode != IngressModeLongPoll {
		t.Fatalf("Ingress.Mode = %q, want %q", plan.Ingress.Mode, IngressModeLongPoll)
	}
	if plan.Ingress.BaseURL != DefaultBaseURL {
		t.Fatalf("Ingress.BaseURL = %q, want %q", plan.Ingress.BaseURL, DefaultBaseURL)
	}
	if plan.Ingress.CDNBaseURL != DefaultCDNBaseURL {
		t.Fatalf("Ingress.CDNBaseURL = %q, want %q", plan.Ingress.CDNBaseURL, DefaultCDNBaseURL)
	}
	if plan.Ingress.RequiresPublicURL {
		t.Fatal("Ingress.RequiresPublicURL = true, want false")
	}
	if !plan.Ingress.SinglePollerPerToken {
		t.Fatal("Ingress.SinglePollerPerToken = false, want true")
	}
	if plan.Outbound.Mode != ReplyModeContextToken {
		t.Fatalf("Outbound.Mode = %q, want %q", plan.Outbound.Mode, ReplyModeContextToken)
	}
	if !plan.Outbound.RequiresContextToken {
		t.Fatal("Outbound.RequiresContextToken = false, want true")
	}
	if !plan.Outbound.PersistContextTokens {
		t.Fatal("Outbound.PersistContextTokens = false, want true")
	}
}

func TestDecideRuntime_RequiresCredentials(t *testing.T) {
	_, err := DecideRuntime(RuntimeConfig{AccountID: "account-1"})
	if err == nil {
		t.Fatal("DecideRuntime() error = nil, want credential validation failure")
	}
	if got := err.Error(); got != "weixin: account id and token are required for long-poll mode" {
		t.Fatalf("DecideRuntime() error = %q, want credential validation failure", got)
	}
}

func TestContextTokens_RememberRefreshesLatestToken(t *testing.T) {
	store := NewContextTokens()

	store.Remember("chat-1", "ctx-old")
	store.Remember("chat-1", "ctx-new")

	token, err := store.Lookup("chat-1")
	if err != nil {
		t.Fatalf("Lookup() error = %v, want nil", err)
	}
	if token != "ctx-new" {
		t.Fatalf("Lookup() = %q, want latest token", token)
	}
}

func TestContextTokens_LookupRejectsMissingOrBlankToken(t *testing.T) {
	store := NewContextTokens()
	store.Remember("chat-1", "   ")

	_, err := store.Lookup("chat-1")
	if err == nil {
		t.Fatal("Lookup() error = nil, want missing-token failure")
	}
	if got := err.Error(); got != "weixin: no context token for chat \"chat-1\"" {
		t.Fatalf("Lookup() error = %q, want missing-token failure", got)
	}
}
