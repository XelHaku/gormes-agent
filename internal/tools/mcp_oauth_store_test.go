package tools

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMCPOAuthStore_AbsentByDefault(t *testing.T) {
	store := NewMCPOAuthStore()
	t0 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	got := store.StatusFor("srv", t0)

	want := MCPOAuthStatus{Server: "srv", State: "absent", Evidence: "no_token"}
	if got != want {
		t.Fatalf("StatusFor(absent) = %+v, want %+v", got, want)
	}
}

func TestMCPOAuthStore_SetGetRoundTrip(t *testing.T) {
	store := NewMCPOAuthStore()
	t0 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	tok := MCPOAuthToken{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		Scope:        "read",
		Issuer:       "https://example.test",
		ExpiresAt:    t0.Add(time.Hour),
	}

	if err := store.Set("srv", tok); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	got, ok := store.Get("srv")
	if !ok {
		t.Fatalf("Get(srv) ok=false after Set")
	}
	if got != tok {
		t.Fatalf("Get(srv) = %+v, want %+v", got, tok)
	}

	store.Clear("srv")
	if _, ok := store.Get("srv"); ok {
		t.Fatalf("Get(srv) ok=true after Clear")
	}
	if state := store.StatusFor("srv", t0).State; state != "absent" {
		t.Fatalf("StatusFor after Clear State = %q, want %q", state, "absent")
	}
}

func TestMCPOAuthStore_ExpiredWhenPastExpiresAt(t *testing.T) {
	store := NewMCPOAuthStore()
	t0 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	if err := store.Set("srv", MCPOAuthToken{
		AccessToken: "access-1",
		ExpiresAt:   t0.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	got := store.StatusFor("srv", t0)

	if got.State != "expired" {
		t.Fatalf("State = %q, want %q", got.State, "expired")
	}
	if got.Evidence != "token_expired" {
		t.Fatalf("Evidence = %q, want %q", got.Evidence, "token_expired")
	}
}

func TestMCPOAuthStore_ValidWhenFutureExpiresAt(t *testing.T) {
	store := NewMCPOAuthStore()
	t0 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	if err := store.Set("srv", MCPOAuthToken{
		AccessToken: "access-1",
		ExpiresAt:   t0.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	got := store.StatusFor("srv", t0)

	if got.State != "valid" {
		t.Fatalf("State = %q, want %q", got.State, "valid")
	}
	if got.Evidence != "ok" {
		t.Fatalf("Evidence = %q, want %q", got.Evidence, "ok")
	}
}

func TestMCPOAuthStore_StatusRedactsSecrets(t *testing.T) {
	store := NewMCPOAuthStore()
	t0 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	const accessSecret = "sk-abc-secret"
	const refreshSecret = "rt-xyz-secret"
	if err := store.Set("srv", MCPOAuthToken{
		AccessToken:  accessSecret,
		RefreshToken: refreshSecret,
		ExpiresAt:    t0.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	got := store.StatusFor("srv", t0)
	rendered := got.String()

	if strings.Contains(rendered, accessSecret) {
		t.Fatalf("Status.String() leaked AccessToken: %q", rendered)
	}
	if strings.Contains(rendered, refreshSecret) {
		t.Fatalf("Status.String() leaked RefreshToken: %q", rendered)
	}
	if strings.Contains(got.Evidence, accessSecret) {
		t.Fatalf("Status.Evidence leaked AccessToken: %q", got.Evidence)
	}
	if strings.Contains(got.Evidence, refreshSecret) {
		t.Fatalf("Status.Evidence leaked RefreshToken: %q", got.Evidence)
	}
}

func TestMCPOAuthStore_NoninteractiveErrorClass(t *testing.T) {
	if ErrMCPOAuthNoninteractiveRequired == nil {
		t.Fatalf("ErrMCPOAuthNoninteractiveRequired must be a non-nil exported error")
	}
	// Sanity: it should also satisfy errors.Is against itself.
	if !errors.Is(ErrMCPOAuthNoninteractiveRequired, ErrMCPOAuthNoninteractiveRequired) {
		t.Fatalf("ErrMCPOAuthNoninteractiveRequired should match itself via errors.Is")
	}

	store := NewMCPOAuthStore().WithNoninteractive(true)
	t0 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	got := store.StatusFor("srv", t0)

	if got.State != "noninteractive_required" {
		t.Fatalf("State = %q, want %q", got.State, "noninteractive_required")
	}
	if got.Evidence != "noninteractive_auth_unavailable" {
		t.Fatalf("Evidence = %q, want %q", got.Evidence, "noninteractive_auth_unavailable")
	}
}
