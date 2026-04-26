package hermes

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	BedrockAuthStatePresent = "present"
	BedrockAuthStateMissing = "missing"

	BedrockAuthStatusCredentialsMissing = "bedrock_credentials_missing"
	BedrockAuthStatusBearerSelected     = "bedrock_bearer_selected"
	BedrockAuthStatusProfileSelected    = "bedrock_profile_selected"
	BedrockAuthStatusStaticKeySelected  = "bedrock_static_key_selected"
	BedrockAuthStatusSigV4Unavailable   = "bedrock_sigv4_unavailable"
)

type BedrockAuthEvidence struct {
	Source string
	State  string
	Status string
}

type StaticAWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
}

func (e BedrockAuthEvidence) String() string {
	return fmt.Sprintf("bedrock auth source=%s state=%s status=%s", e.Source, e.State, e.Status)
}

func (e BedrockAuthEvidence) Error() string {
	return e.String()
}

func ResolveBedrockAuth(env map[string]string) BedrockAuthEvidence {
	if strings.TrimSpace(env["AWS_BEARER_TOKEN_BEDROCK"]) != "" {
		return BedrockAuthEvidence{
			Source: "AWS_BEARER_TOKEN_BEDROCK",
			State:  BedrockAuthStatePresent,
			Status: BedrockAuthStatusBearerSelected,
		}
	}
	accessKey := strings.TrimSpace(env["AWS_ACCESS_KEY_ID"])
	secretKey := strings.TrimSpace(env["AWS_SECRET_ACCESS_KEY"])
	if accessKey != "" && secretKey != "" {
		return BedrockAuthEvidence{
			Source: "AWS_ACCESS_KEY_ID",
			State:  BedrockAuthStatePresent,
			Status: BedrockAuthStatusStaticKeySelected,
		}
	}
	if accessKey != "" || secretKey != "" {
		return BedrockAuthEvidence{
			Source: "AWS_ACCESS_KEY_ID",
			State:  BedrockAuthStateMissing,
			Status: BedrockAuthStatusCredentialsMissing,
		}
	}
	if strings.TrimSpace(env["AWS_PROFILE"]) != "" {
		return BedrockAuthEvidence{
			Source: "AWS_PROFILE",
			State:  BedrockAuthStatePresent,
			Status: BedrockAuthStatusProfileSelected,
		}
	}
	if strings.TrimSpace(env["AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"]) != "" {
		return BedrockAuthEvidence{
			Source: "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
			State:  BedrockAuthStatePresent,
			Status: BedrockAuthStatusSigV4Unavailable,
		}
	}
	if strings.TrimSpace(env["AWS_WEB_IDENTITY_TOKEN_FILE"]) != "" {
		return BedrockAuthEvidence{
			Source: "AWS_WEB_IDENTITY_TOKEN_FILE",
			State:  BedrockAuthStatePresent,
			Status: BedrockAuthStatusSigV4Unavailable,
		}
	}
	return BedrockAuthEvidence{State: BedrockAuthStateMissing, Status: BedrockAuthStatusCredentialsMissing}
}

func ResolveBedrockRegion(env map[string]string) string {
	if region := strings.TrimSpace(env["AWS_REGION"]); region != "" {
		return region
	}
	if region := strings.TrimSpace(env["AWS_DEFAULT_REGION"]); region != "" {
		return region
	}
	return "us-east-1"
}

func SignBedrockRequest(req *http.Request, creds StaticAWSCredentials, now time.Time) error {
	if req == nil {
		return errors.New("bedrock sigv4 request is nil")
	}
	accessKey := strings.TrimSpace(creds.AccessKeyID)
	secretKey := strings.TrimSpace(creds.SecretAccessKey)
	if accessKey == "" || secretKey == "" {
		return errors.New("bedrock sigv4 static credentials are incomplete")
	}
	region := strings.TrimSpace(creds.Region)
	if region == "" {
		region = bedrockRegionFromHost(req.URL.Host)
	}
	if region == "" {
		region = "us-east-1"
	}

	body, err := bedrockRequestBody(req)
	if err != nil {
		return err
	}
	payloadHash := bedrockSHA256Hex(body)
	amzDate := now.UTC().Format("20060102T150405Z")
	date := now.UTC().Format("20060102")

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if token := strings.TrimSpace(creds.SessionToken); token != "" {
		req.Header.Set("X-Amz-Security-Token", token)
	}

	scope := strings.Join([]string{date, region, "bedrock", "aws4_request"}, "/")
	canonicalRequest, signedHeaders := bedrockCanonicalRequest(req, payloadHash)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		bedrockSHA256Hex([]byte(canonicalRequest)),
	}, "\n")
	signingKey := bedrockSigningKey(secretKey, date, region, "bedrock")
	signature := hex.EncodeToString(bedrockHMAC(signingKey, stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey,
		scope,
		signedHeaders,
		signature,
	))
	return nil
}

func bedrockRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, errors.New("bedrock sigv4 request body could not be read")
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	return body, nil
}

func bedrockCanonicalRequest(req *http.Request, payloadHash string) (string, string) {
	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	if req.Header.Get("X-Amz-Security-Token") != "" {
		signedHeaders = append(signedHeaders, "x-amz-security-token")
	}
	sort.Strings(signedHeaders)

	headerValues := map[string]string{
		"host":                 bedrockCanonicalHost(req),
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           req.Header.Get("X-Amz-Date"),
	}
	if token := req.Header.Get("X-Amz-Security-Token"); token != "" {
		headerValues["x-amz-security-token"] = token
	}

	var canonicalHeaders strings.Builder
	for _, name := range signedHeaders {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(strings.Join(strings.Fields(headerValues[name]), " "))
		canonicalHeaders.WriteByte('\n')
	}
	signedHeadersText := strings.Join(signedHeaders, ";")
	return strings.Join([]string{
		req.Method,
		bedrockCanonicalURI(req.URL),
		bedrockCanonicalQuery(req.URL),
		canonicalHeaders.String(),
		signedHeadersText,
		payloadHash,
	}, "\n"), signedHeadersText
}

func bedrockCanonicalHost(req *http.Request) string {
	if req.Host != "" {
		return req.Host
	}
	if req.URL != nil {
		return req.URL.Host
	}
	return ""
}

func bedrockCanonicalURI(u *url.URL) string {
	if u == nil {
		return "/"
	}
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func bedrockCanonicalQuery(u *url.URL) string {
	if u == nil || u.RawQuery == "" {
		return ""
	}
	values := u.Query()
	pairs := make([]string, 0)
	for key, rawValues := range values {
		sort.Strings(rawValues)
		for _, value := range rawValues {
			pairs = append(pairs, bedrockURIEncode(key)+"="+bedrockURIEncode(value))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

func bedrockURIEncode(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	return strings.ReplaceAll(escaped, "%7E", "~")
}

func bedrockSHA256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func bedrockSigningKey(secret, date, region, service string) []byte {
	kDate := bedrockHMAC([]byte("AWS4"+secret), date)
	kRegion := bedrockHMAC(kDate, region)
	kService := bedrockHMAC(kRegion, service)
	return bedrockHMAC(kService, "aws4_request")
}

func bedrockHMAC(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func bedrockRegionFromHost(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) >= 3 && parts[0] == "bedrock-runtime" {
		return parts[1]
	}
	return ""
}
