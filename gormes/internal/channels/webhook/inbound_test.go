package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestParseInbound_GitHubSignatureAndJSONPayload(t *testing.T) {
	body := []byte(`{"action":"opened","repository":{"full_name":"org/repo"}}`)
	got, ok, err := ParseInbound(InboundRequest{
		Headers: map[string]string{
			"X-Hub-Signature-256": githubSignature(body, "top-secret"),
			"X-GitHub-Event":      "pull_request",
			"X-GitHub-Delivery":   "gh-123",
		},
		Body:          body,
		ContentLength: int64(len(body)),
	}, IngressConfig{
		Secret:       "top-secret",
		Events:       []string{"pull_request"},
		MaxBodyBytes: 1024,
	})
	if err != nil {
		t.Fatalf("ParseInbound() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseInbound() ok = false, want true")
	}
	if got.EventType != "pull_request" {
		t.Fatalf("EventType = %q, want pull_request", got.EventType)
	}
	if got.DeliveryID != "gh-123" {
		t.Fatalf("DeliveryID = %q, want gh-123", got.DeliveryID)
	}
	if got.Payload["action"] != "opened" {
		t.Fatalf("Payload[action] = %#v, want opened", got.Payload["action"])
	}
}

func TestParseInbound_GitLabTokenParsesFormPayload(t *testing.T) {
	body := []byte("event_type=push&ref=main")
	got, ok, err := ParseInbound(InboundRequest{
		Headers: map[string]string{
			"X-Gitlab-Token": "gitlab-secret",
			"X-Request-ID":   "req-9",
		},
		Body:          body,
		ContentLength: int64(len(body)),
	}, IngressConfig{
		Secret:       "gitlab-secret",
		MaxBodyBytes: 1024,
	})
	if err != nil {
		t.Fatalf("ParseInbound() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseInbound() ok = false, want true")
	}
	if got.EventType != "push" {
		t.Fatalf("EventType = %q, want push", got.EventType)
	}
	if got.DeliveryID != "req-9" {
		t.Fatalf("DeliveryID = %q, want req-9", got.DeliveryID)
	}
	if got.Payload["ref"] != "main" {
		t.Fatalf("Payload[ref] = %#v, want main", got.Payload["ref"])
	}
}

func TestParseInbound_GenericSignatureUsesDeterministicBodyHashFallback(t *testing.T) {
	body := []byte(`{"event_type":"build","status":"queued"}`)
	sum := sha256.Sum256(body)

	got, ok, err := ParseInbound(InboundRequest{
		Headers: map[string]string{
			"X-Webhook-Signature": genericSignature(body, "generic-secret"),
		},
		Body: body,
	}, IngressConfig{
		Secret:       "generic-secret",
		MaxBodyBytes: 1024,
	})
	if err != nil {
		t.Fatalf("ParseInbound() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseInbound() ok = false, want true")
	}
	wantID := "sha256:" + hex.EncodeToString(sum[:])
	if got.DeliveryID != wantID {
		t.Fatalf("DeliveryID = %q, want %q", got.DeliveryID, wantID)
	}
	if got.EventType != "build" {
		t.Fatalf("EventType = %q, want build", got.EventType)
	}
}

func TestParseInbound_IgnoresFilteredEvents(t *testing.T) {
	got, ok, err := ParseInbound(InboundRequest{
		Headers: map[string]string{
			"X-GitHub-Event": "push",
		},
		Body: []byte(`{"ref":"main"}`),
	}, IngressConfig{
		Secret:       InsecureNoAuth,
		Events:       []string{"pull_request"},
		MaxBodyBytes: 1024,
	})
	if err != nil {
		t.Fatalf("ParseInbound() error = %v, want nil", err)
	}
	if ok {
		t.Fatalf("ParseInbound() ok = true, want false with result %+v", got)
	}
}

func TestParseInbound_RejectsOversizedPayloadAndBadSignature(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	if _, ok, err := ParseInbound(InboundRequest{
		Headers: map[string]string{
			"X-Hub-Signature-256": githubSignature(body, "top-secret"),
		},
		Body:          body,
		ContentLength: 9999,
	}, IngressConfig{
		Secret:       "top-secret",
		MaxBodyBytes: 10,
	}); err == nil || ok {
		t.Fatalf("ParseInbound(oversized) = ok:%v err:%v, want non-nil error and ok=false", ok, err)
	}

	if _, ok, err := ParseInbound(InboundRequest{
		Headers: map[string]string{
			"X-Hub-Signature-256": "sha256=deadbeef",
			"X-GitHub-Event":      "pull_request",
		},
		Body: body,
	}, IngressConfig{
		Secret:       "top-secret",
		MaxBodyBytes: 1024,
	}); err == nil || ok {
		t.Fatalf("ParseInbound(bad signature) = ok:%v err:%v, want non-nil error and ok=false", ok, err)
	}
}

func githubSignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func genericSignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
