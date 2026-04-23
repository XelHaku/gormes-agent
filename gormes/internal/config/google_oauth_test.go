package config

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestLoadProviderToken_GoogleGeminiCLIAutoRefreshesExpiredCredentialFile(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("HERMES_GEMINI_CLIENT_ID", "refresh-client-id")
	t.Setenv("HERMES_GEMINI_CLIENT_SECRET", "refresh-client-secret")
	t.Setenv("GORMES_GOOGLE_OAUTH_TOKEN_ENDPOINT", "")

	now := time.Date(2026, 4, 23, 15, 0, 0, 0, time.UTC)
	prevNow := googleOAuthNow
	googleOAuthNow = func() time.Time { return now }
	t.Cleanup(func() { googleOAuthNow = prevNow })

	tokenAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Fatalf("path = %s, want /token", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm(): %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q, want refresh_token", got)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-456" {
			t.Fatalf("refresh_token = %q, want refresh-456", got)
		}
		if got := r.Form.Get("client_id"); got != "refresh-client-id" {
			t.Fatalf("client_id = %q, want refresh-client-id", got)
		}
		if got := r.Form.Get("client_secret"); got != "refresh-client-secret" {
			t.Fatalf("client_secret = %q, want refresh-client-secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"refreshed-access","expires_in":900}`)
	}))
	defer tokenAPI.Close()
	t.Setenv("GORMES_GOOGLE_OAUTH_TOKEN_ENDPOINT", tokenAPI.URL+"/token")

	credsPath := ProviderTokenPath("google-gemini-cli")
	if err := os.MkdirAll(filepath.Dir(credsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(auth dir): %v", err)
	}
	expiredAt := now.Add(-2 * time.Minute).UnixMilli()
	if err := os.WriteFile(credsPath, []byte(`{
  "refresh": "refresh-456",
  "access": "expired-access",
  "expires": `+strconv.FormatInt(expiredAt, 10)+`
}`), 0o600); err != nil {
		t.Fatalf("WriteFile(google_oauth.json): %v", err)
	}

	got, err := loadProviderToken("google-gemini-cli")
	if err != nil {
		t.Fatalf("loadProviderToken(): %v", err)
	}
	if got != "refreshed-access" {
		t.Fatalf("token = %q, want refreshed-access", got)
	}

	raw, err := os.ReadFile(credsPath)
	if err != nil {
		t.Fatalf("ReadFile(google_oauth.json): %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("Unmarshal(google_oauth.json): %v", err)
	}
	if got := stored["access"]; got != "refreshed-access" {
		t.Fatalf("stored access = %#v, want refreshed-access", got)
	}
	if got := stored["refresh"]; got != "refresh-456" {
		t.Fatalf("stored refresh = %#v, want refresh-456", got)
	}
}
