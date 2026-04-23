package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

func TestAuthLoginCommand_GoogleGeminiCLIRequiresRiskAcceptance(t *testing.T) {
	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"auth", "login", "google-gemini-cli", "--no-browser"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want acceptance error")
	}
	if !strings.Contains(err.Error(), "accept-google-oauth-risk") {
		t.Fatalf("error = %v, want risk-acceptance guidance", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestAuthLoginCommand_GoogleGeminiCLICompletesBrowserFlow(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dataHome, "config"))
	t.Setenv("HERMES_GEMINI_CLIENT_ID", "test-client-id")
	t.Setenv("HERMES_GEMINI_CLIENT_SECRET", "test-client-secret")

	oauthAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(): %v", err)
			}
			if got := r.Form.Get("grant_type"); got != "authorization_code" {
				t.Fatalf("grant_type = %q, want authorization_code", got)
			}
			if got := r.Form.Get("client_id"); got != "test-client-id" {
				t.Fatalf("client_id = %q, want test-client-id", got)
			}
			if got := r.Form.Get("client_secret"); got != "test-client-secret" {
				t.Fatalf("client_secret = %q, want test-client-secret", got)
			}
			if got := r.Form.Get("code"); got != "test-code" {
				t.Fatalf("code = %q, want test-code", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"access-123","refresh_token":"refresh-123","expires_in":3600}`)
		case "/userinfo":
			if got := r.Header.Get("Authorization"); got != "Bearer access-123" {
				t.Fatalf("Authorization = %q, want Bearer access-123", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"email":"person@example.com"}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer oauthAPI.Close()

	t.Setenv("GORMES_GOOGLE_OAUTH_AUTH_ENDPOINT", "https://accounts.example.test/o/oauth2/v2/auth")
	t.Setenv("GORMES_GOOGLE_OAUTH_TOKEN_ENDPOINT", oauthAPI.URL+"/token")
	t.Setenv("GORMES_GOOGLE_OAUTH_USERINFO_ENDPOINT", oauthAPI.URL+"/userinfo")

	cmd := newRootCommand()
	stdout := &lockedBuffer{}
	var stderr bytes.Buffer
	cmd.SetOut(stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"auth", "login", "google-gemini-cli", "--accept-google-oauth-risk", "--no-browser"})

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Execute()
	}()

	authURL := waitForAuthURL(t, stdout, &stderr, errCh)
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("Parse(authURL): %v", err)
	}
	redirectURI := parsed.Query().Get("redirect_uri")
	state := parsed.Query().Get("state")
	if redirectURI == "" || state == "" {
		t.Fatalf("auth URL missing redirect/state: %s", authURL)
	}
	callbackURL := redirectURI + "?state=" + url.QueryEscape(state) + "&code=test-code"
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d, want 200", resp.StatusCode)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Execute(): %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for auth login command\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}

	if !strings.Contains(stdout.String(), "person@example.com") {
		t.Fatalf("stdout = %q, want signed-in email", stdout.String())
	}

	raw, err := os.ReadFile(config.ProviderTokenPath("google-gemini-cli"))
	if err != nil {
		t.Fatalf("ReadFile(google_oauth.json): %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("Unmarshal(google_oauth.json): %v", err)
	}
	if got := stored["refresh"]; got != "refresh-123" {
		t.Fatalf("refresh = %#v, want refresh-123", got)
	}
	if got := stored["access"]; got != "access-123" {
		t.Fatalf("access = %#v, want access-123", got)
	}
	if got := stored["email"]; got != "person@example.com" {
		t.Fatalf("email = %#v, want person@example.com", got)
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var authURLPattern = regexp.MustCompile(`https://\S+`)

func waitForAuthURL(t *testing.T, stdout *lockedBuffer, stderr *bytes.Buffer, errCh <-chan error) string {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if url := authURLPattern.FindString(stdout.String()); url != "" {
			return url
		}
		select {
		case err := <-errCh:
			t.Fatalf("Execute(): %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for auth URL\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	return ""
}
