package hermes

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestResolveBedrockAuth_BearerTokenWins(t *testing.T) {
	evidence := ResolveBedrockAuth(map[string]string{
		"AWS_BEARER_TOKEN_BEDROCK": "bearer-token-test-secret",
		"AWS_ACCESS_KEY_ID":        "AKIA_TEST",
		"AWS_SECRET_ACCESS_KEY":    "secret-test-value",
	})

	if evidence.Source != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Fatalf("Source = %q, want AWS_BEARER_TOKEN_BEDROCK", evidence.Source)
	}
	if evidence.State != "present" {
		t.Fatalf("State = %q, want present", evidence.State)
	}
	if got := evidence.String(); got == "" || containsAnyString(got, "bearer-token-test-secret", "AKIA_TEST", "secret-test-value") {
		t.Fatalf("Evidence.String() leaked credential material: %q", got)
	}
}

func TestResolveBedrockAuth_StaticKeyPairRequiresSecret(t *testing.T) {
	present := ResolveBedrockAuth(map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKIA_TEST",
		"AWS_SECRET_ACCESS_KEY": "secret-test-value",
	})
	if present.Source != "AWS_ACCESS_KEY_ID" {
		t.Fatalf("present Source = %q, want AWS_ACCESS_KEY_ID", present.Source)
	}
	if present.State != "present" {
		t.Fatalf("present State = %q, want present", present.State)
	}

	missingSecret := ResolveBedrockAuth(map[string]string{
		"AWS_ACCESS_KEY_ID": "AKIA_TEST",
	})
	if missingSecret.Source != "AWS_ACCESS_KEY_ID" {
		t.Fatalf("missing Source = %q, want AWS_ACCESS_KEY_ID", missingSecret.Source)
	}
	if missingSecret.State != "missing" {
		t.Fatalf("missing State = %q, want missing", missingSecret.State)
	}
}

func TestResolveBedrockAuth_ProfileAndContainerSources(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		source string
	}{
		{
			name:   "profile",
			env:    map[string]string{"AWS_PROFILE": "default"},
			source: "AWS_PROFILE",
		},
		{
			name:   "container",
			env:    map[string]string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/v2/credentials/task"},
			source: "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
		},
		{
			name:   "web_identity",
			env:    map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": "/var/run/secrets/eks-token"},
			source: "AWS_WEB_IDENTITY_TOKEN_FILE",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := ResolveBedrockAuth(tt.env)
			if evidence.Source != tt.source {
				t.Fatalf("Source = %q, want %s", evidence.Source, tt.source)
			}
			if evidence.State != "present" {
				t.Fatalf("State = %q, want present", evidence.State)
			}
		})
	}
}

func TestResolveBedrockRegionPriority(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "aws_region_wins",
			env:  map[string]string{"AWS_REGION": "eu-west-1", "AWS_DEFAULT_REGION": "us-west-2"},
			want: "eu-west-1",
		},
		{
			name: "default_region_fallback",
			env:  map[string]string{"AWS_DEFAULT_REGION": "ap-northeast-1"},
			want: "ap-northeast-1",
		},
		{
			name: "hard_default",
			env:  map[string]string{},
			want: "us-east-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveBedrockRegion(tt.env); got != tt.want {
				t.Fatalf("ResolveBedrockRegion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSignBedrockRequest_DeterministicHeaders(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"text":"hello"}]}]}`)
	req, err := http.NewRequest(http.MethodPost, "https://bedrock-runtime.us-east-1.amazonaws.com/model/x/converse", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	now := time.Date(2026, 4, 26, 22, 45, 56, 0, time.UTC)
	creds := StaticAWSCredentials{
		AccessKeyID:     "AKIA_TEST",
		SecretAccessKey: "secret-test-value",
		SessionToken:    "session-token-test-value",
		Region:          "us-east-1",
	}

	if err := SignBedrockRequest(req, creds, now); err != nil {
		t.Fatalf("SignBedrockRequest() error = %v", err)
	}

	wantHashBytes := sha256.Sum256(body)
	wantHash := hex.EncodeToString(wantHashBytes[:])
	if got := req.Header.Get("X-Amz-Date"); got != "20260426T224556Z" {
		t.Fatalf("X-Amz-Date = %q, want 20260426T224556Z", got)
	}
	if got := req.Header.Get("X-Amz-Content-Sha256"); got != wantHash {
		t.Fatalf("X-Amz-Content-Sha256 = %q, want %q", got, wantHash)
	}
	auth := req.Header.Get("Authorization")
	wantPrefix := "AWS4-HMAC-SHA256 Credential=AKIA_TEST/20260426/us-east-1/bedrock/aws4_request"
	if !strings.HasPrefix(auth, wantPrefix) {
		t.Fatalf("Authorization = %q, want prefix %q", auth, wantPrefix)
	}
	if !strings.Contains(auth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-security-token") {
		t.Fatalf("Authorization missing deterministic signed headers: %q", auth)
	}
	if got := req.Header.Get("X-Amz-Security-Token"); got != "session-token-test-value" {
		t.Fatalf("X-Amz-Security-Token = %q, want session token", got)
	}
	afterSign, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read signed body: %v", err)
	}
	if !bytes.Equal(afterSign, body) {
		t.Fatalf("signed request body changed: got %q want %q", afterSign, body)
	}
}

func TestBedrockAuthEvidence_RedactsSecrets(t *testing.T) {
	const (
		accessKey    = "AKIA_TEST_REDACT_ME"
		secretKey    = "secret-test-value-redact-me"
		sessionToken = "session-token-test-value-redact-me"
		bearerToken  = "bearer-token-test-secret-redact-me"
	)
	evidence := ResolveBedrockAuth(map[string]string{
		"AWS_BEARER_TOKEN_BEDROCK": bearerToken,
		"AWS_ACCESS_KEY_ID":        accessKey,
		"AWS_SECRET_ACCESS_KEY":    secretKey,
	})
	signErr := SignBedrockRequest(nil, StaticAWSCredentials{
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		SessionToken:    sessionToken,
		Region:          "us-east-1",
	}, time.Date(2026, 4, 26, 22, 45, 56, 0, time.UTC))
	if signErr == nil {
		t.Fatal("SignBedrockRequest(nil) error = nil, want redacted error")
	}

	texts := map[string]string{
		"Evidence.String": evidence.String(),
		"Evidence.Error":  evidence.Error(),
		"sign error":      signErr.Error(),
	}
	for label, text := range texts {
		if containsAnyString(text, accessKey, secretKey, sessionToken, bearerToken) {
			t.Fatalf("%s leaked credential material: %q", label, text)
		}
	}
}

func containsAnyString(s string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
