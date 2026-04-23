package config

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	googleOAuthAuthEndpointEnv     = "GORMES_GOOGLE_OAUTH_AUTH_ENDPOINT"
	googleOAuthTokenEndpointEnv    = "GORMES_GOOGLE_OAUTH_TOKEN_ENDPOINT"
	googleOAuthUserinfoEndpointEnv = "GORMES_GOOGLE_OAUTH_USERINFO_ENDPOINT"
	googleOAuthClientIDEnv         = "HERMES_GEMINI_CLIENT_ID"
	googleOAuthClientSecretEnv     = "HERMES_GEMINI_CLIENT_SECRET"

	defaultGoogleOAuthAuthEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultGoogleOAuthTokenEndpoint    = "https://oauth2.googleapis.com/token"
	defaultGoogleOAuthUserinfoEndpoint = "https://www.googleapis.com/oauth2/v1/userinfo?alt=json"
	defaultGoogleOAuthRedirectHost     = "127.0.0.1"
	defaultGoogleOAuthRedirectPort     = 8085
	googleOAuthCallbackPath            = "/oauth2callback"
	googleOAuthScopes                  = "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"
	googleOAuthRefreshSkew             = 60 * time.Second
	googleOAuthHTTPTimeout             = 20 * time.Second
	googleOAuthCallbackTimeout         = 5 * time.Minute

	googlePublicClientIDProjectNum = "681255809395"
	googlePublicClientIDHash       = "oo8ft2oprdrnp9e3aqf6av3hmdib135j"
	googlePublicClientSecretSuffix = "4uHgMPm-1o7Sk-geV6Cu5clXFsxl"
)

var (
	googleOAuthNow         = time.Now
	googleOAuthOpenBrowser = openBrowserURL
)

var (
	defaultGoogleOAuthClientID = googlePublicClientIDProjectNum + "-" + googlePublicClientIDHash + ".apps.googleusercontent.com"
	defaultGoogleOAuthSecret   = "GOCSPX-" + googlePublicClientSecretSuffix
)

type GoogleOAuthLoginOptions struct {
	OpenBrowser bool
	NotifyURL   func(string)
}

type GoogleOAuthLoginResult struct {
	Email string
	Path  string
}

type googleOAuthCredentials struct {
	Refresh string `json:"refresh,omitempty"`
	Access  string `json:"access,omitempty"`
	Expires int64  `json:"expires,omitempty"`
	Email   string `json:"email,omitempty"`
}

type googleOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type googleOAuthCallbackResult struct {
	Code string
	Err  error
}

func (c googleOAuthCredentials) needsRefresh(now time.Time) bool {
	if strings.TrimSpace(c.Refresh) == "" {
		return false
	}
	if strings.TrimSpace(c.Access) == "" || c.Expires == 0 {
		return true
	}
	return !now.Add(googleOAuthRefreshSkew).Before(time.UnixMilli(c.Expires))
}

func LoginGoogleOAuth(ctx context.Context, opts GoogleOAuthLoginOptions) (GoogleOAuthLoginResult, error) {
	clientID, _ := googleOAuthClientCredentials()
	if clientID == "" {
		return GoogleOAuthLoginResult{}, fmt.Errorf("google OAuth client ID is not configured")
	}

	verifier, challenge, err := generateGoogleOAuthPKCE()
	if err != nil {
		return GoogleOAuthLoginResult{}, err
	}
	state, err := googleOAuthRandomString(24)
	if err != nil {
		return GoogleOAuthLoginResult{}, err
	}

	redirectURI, waitForCode, shutdown, err := startGoogleOAuthCallbackServer(state)
	if err != nil {
		return GoogleOAuthLoginResult{}, err
	}
	defer shutdown()

	authURL := buildGoogleOAuthAuthURL(clientID, redirectURI, challenge, state)
	if opts.NotifyURL != nil {
		opts.NotifyURL(authURL)
	}
	if opts.OpenBrowser {
		_ = googleOAuthOpenBrowser(authURL)
	}

	waitCtx, cancel := context.WithTimeout(ctx, googleOAuthCallbackTimeout)
	defer cancel()
	code, err := waitForCode(waitCtx)
	if err != nil {
		return GoogleOAuthLoginResult{}, err
	}

	tokenResp, err := exchangeGoogleOAuthCode(ctx, code, verifier, redirectURI)
	if err != nil {
		return GoogleOAuthLoginResult{}, err
	}
	creds := googleOAuthCredentials{
		Refresh: tokenResp.RefreshToken,
		Access:  tokenResp.AccessToken,
		Expires: googleOAuthExpiryUnixMilli(tokenResp.ExpiresIn),
		Email:   fetchGoogleOAuthEmail(ctx, tokenResp.AccessToken),
	}

	path := ProviderTokenPath("google-gemini-cli")
	if err := saveGoogleOAuthCredentials(path, creds); err != nil {
		return GoogleOAuthLoginResult{}, err
	}
	return GoogleOAuthLoginResult{Email: creds.Email, Path: path}, nil
}

func loadGoogleOAuthProviderToken(path string, tokenFile providerTokenFile) (string, error) {
	creds := googleOAuthCredentials{
		Refresh: strings.TrimSpace(tokenFile.Refresh),
		Access:  firstNonEmpty(tokenFile.Access, tokenFile.AccessToken, tokenFile.APIKey, tokenFile.Token),
		Expires: tokenFile.Expires,
		Email:   strings.TrimSpace(tokenFile.Email),
	}
	if creds.Refresh == "" {
		return creds.Access, nil
	}
	if !creds.needsRefresh(googleOAuthNow()) {
		return creds.Access, nil
	}

	refreshed, err := refreshGoogleOAuthCredentials(context.Background(), creds)
	if err != nil {
		return "", err
	}
	if err := saveGoogleOAuthCredentials(path, refreshed); err != nil {
		return "", err
	}
	return refreshed.Access, nil
}

func refreshGoogleOAuthCredentials(ctx context.Context, creds googleOAuthCredentials) (googleOAuthCredentials, error) {
	tokenResp, err := refreshGoogleOAuthAccessToken(ctx, googleOAuthRefreshToken(creds.Refresh))
	if err != nil {
		return googleOAuthCredentials{}, err
	}
	if tokenResp.RefreshToken == "" {
		tokenResp.RefreshToken = creds.Refresh
	}
	creds.Access = tokenResp.AccessToken
	creds.Refresh = tokenResp.RefreshToken
	creds.Expires = googleOAuthExpiryUnixMilli(tokenResp.ExpiresIn)
	return creds, nil
}

func saveGoogleOAuthCredentials(path string, creds googleOAuthCredentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	body, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	body = append(body, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", path, err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

func buildGoogleOAuthAuthURL(clientID, redirectURI, challenge, state string) string {
	values := url.Values{}
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("response_type", "code")
	values.Set("scope", googleOAuthScopes)
	values.Set("state", state)
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("access_type", "offline")
	values.Set("prompt", "consent")
	return googleOAuthAuthEndpoint() + "?" + values.Encode()
}

func exchangeGoogleOAuthCode(ctx context.Context, code, verifier, redirectURI string) (googleOAuthTokenResponse, error) {
	clientID, clientSecret := googleOAuthClientCredentials()
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("code_verifier", verifier)
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURI)
	if clientSecret != "" {
		values.Set("client_secret", clientSecret)
	}
	return googleOAuthTokenRequest(ctx, values)
}

func refreshGoogleOAuthAccessToken(ctx context.Context, refreshToken string) (googleOAuthTokenResponse, error) {
	clientID, clientSecret := googleOAuthClientCredentials()
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)
	values.Set("client_id", clientID)
	if clientSecret != "" {
		values.Set("client_secret", clientSecret)
	}
	return googleOAuthTokenRequest(ctx, values)
}

func googleOAuthTokenRequest(ctx context.Context, values url.Values) (googleOAuthTokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, googleOAuthHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleOAuthTokenEndpoint(), strings.NewReader(values.Encode()))
	if err != nil {
		return googleOAuthTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return googleOAuthTokenResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return googleOAuthTokenResponse{}, err
	}
	if resp.StatusCode >= 300 {
		return googleOAuthTokenResponse{}, fmt.Errorf("google OAuth token endpoint returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp googleOAuthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return googleOAuthTokenResponse{}, fmt.Errorf("decode token response: %w", err)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return googleOAuthTokenResponse{}, fmt.Errorf("google OAuth token response missing access_token")
	}
	return tokenResp, nil
}

func fetchGoogleOAuthEmail(ctx context.Context, accessToken string) string {
	if strings.TrimSpace(accessToken) == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, googleOAuthHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleOAuthUserinfoEndpoint(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ""
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.Email)
}

func startGoogleOAuthCallbackServer(state string) (string, func(context.Context) (string, error), func(), error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(defaultGoogleOAuthRedirectHost, fmt.Sprintf("%d", defaultGoogleOAuthRedirectPort)))
	if err != nil {
		listener, err = net.Listen("tcp", net.JoinHostPort(defaultGoogleOAuthRedirectHost, "0"))
		if err != nil {
			return "", nil, nil, err
		}
	}

	resultCh := make(chan googleOAuthCallbackResult, 1)
	var once sync.Once
	sendResult := func(result googleOAuthCallbackResult) {
		once.Do(func() {
			resultCh <- result
		})
	}

	mux := http.NewServeMux()
	mux.HandleFunc(googleOAuthCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("state") != state:
			http.Error(w, "Google OAuth state mismatch", http.StatusBadRequest)
			sendResult(googleOAuthCallbackResult{Err: fmt.Errorf("google OAuth state mismatch")})
		case strings.TrimSpace(query.Get("error")) != "":
			http.Error(w, "Google OAuth authorization failed", http.StatusBadRequest)
			sendResult(googleOAuthCallbackResult{Err: fmt.Errorf("google OAuth authorization failed: %s", query.Get("error"))})
		case strings.TrimSpace(query.Get("code")) == "":
			http.Error(w, "Google OAuth callback missing code", http.StatusBadRequest)
			sendResult(googleOAuthCallbackResult{Err: fmt.Errorf("google OAuth callback missing code")})
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, "<html><body><h1>Signed in.</h1><p>You can return to Gormes.</p></body></html>")
			sendResult(googleOAuthCallbackResult{Code: query.Get("code")})
		}
	})

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()

	addr := listener.Addr().(*net.TCPAddr)
	redirectURI := fmt.Sprintf("http://%s:%d%s", defaultGoogleOAuthRedirectHost, addr.Port, googleOAuthCallbackPath)
	waitForCode := func(ctx context.Context) (string, error) {
		select {
		case result := <-resultCh:
			return result.Code, result.Err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		_ = listener.Close()
	}
	return redirectURI, waitForCode, shutdown, nil
}

func generateGoogleOAuthPKCE() (string, string, error) {
	verifier, err := googleOAuthRandomString(64)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func googleOAuthRandomString(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func googleOAuthRefreshToken(packed string) string {
	packed = strings.TrimSpace(packed)
	if packed == "" {
		return ""
	}
	if head, _, ok := strings.Cut(packed, "|"); ok {
		return head
	}
	return packed
}

func googleOAuthExpiryUnixMilli(expiresInSeconds int64) int64 {
	if expiresInSeconds < 60 {
		expiresInSeconds = 60
	}
	return googleOAuthNow().Add(time.Duration(expiresInSeconds) * time.Second).UnixMilli()
}

func googleOAuthAuthEndpoint() string {
	return firstNonEmpty(strings.TrimSpace(os.Getenv(googleOAuthAuthEndpointEnv)), defaultGoogleOAuthAuthEndpoint)
}

func googleOAuthTokenEndpoint() string {
	return firstNonEmpty(strings.TrimSpace(os.Getenv(googleOAuthTokenEndpointEnv)), defaultGoogleOAuthTokenEndpoint)
}

func googleOAuthUserinfoEndpoint() string {
	return firstNonEmpty(strings.TrimSpace(os.Getenv(googleOAuthUserinfoEndpointEnv)), defaultGoogleOAuthUserinfoEndpoint)
}

func googleOAuthClientID() string {
	return firstNonEmpty(strings.TrimSpace(os.Getenv(googleOAuthClientIDEnv)), defaultGoogleOAuthClientID)
}

func googleOAuthClientSecret() string {
	return firstNonEmpty(strings.TrimSpace(os.Getenv(googleOAuthClientSecretEnv)), defaultGoogleOAuthSecret)
}

func googleOAuthClientCredentials() (string, string) {
	return googleOAuthClientID(), googleOAuthClientSecret()
}

func openBrowserURL(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}
