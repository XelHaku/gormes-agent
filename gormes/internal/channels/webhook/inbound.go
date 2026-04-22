package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const InsecureNoAuth = "INSECURE_NO_AUTH"

var (
	ErrPayloadTooLarge  = errors.New("webhook: payload too large")
	ErrInvalidSignature = errors.New("webhook: invalid signature")
	ErrCannotParseBody  = errors.New("webhook: cannot parse body")
)

// InboundRequest is the transport-neutral request envelope a future HTTP
// adapter can hand to the signed ingress contract.
type InboundRequest struct {
	Headers       map[string]string
	Body          []byte
	ContentLength int64
}

// IngressConfig captures the auth and filtering gates enforced before webhook
// traffic becomes a prompt-delivery job.
type IngressConfig struct {
	Secret       string
	Events       []string
	MaxBodyBytes int64
}

// ParsedInbound is the typed payload metadata future runtime wiring can pass
// into prompt rendering, routing, and idempotency layers.
type ParsedInbound struct {
	EventType  string
	DeliveryID string
	Payload    map[string]any
}

// ParseInbound verifies request gates, parses the payload, and extracts stable
// event metadata. The returned bool is false when the route deliberately
// ignores the event because it does not match the configured event filter.
func ParseInbound(req InboundRequest, cfg IngressConfig) (ParsedInbound, bool, error) {
	if exceedsLimit(req, cfg.MaxBodyBytes) {
		return ParsedInbound{}, false, ErrPayloadTooLarge
	}

	if secret := strings.TrimSpace(cfg.Secret); secret != "" && secret != InsecureNoAuth {
		if !ValidateSignature(req.Headers, req.Body, secret) {
			return ParsedInbound{}, false, ErrInvalidSignature
		}
	}

	payload, err := parseBody(req.Body)
	if err != nil {
		return ParsedInbound{}, false, err
	}

	eventType := eventType(req.Headers, payload)
	if !allowsEvent(cfg.Events, eventType) {
		return ParsedInbound{EventType: eventType, Payload: payload}, false, nil
	}

	return ParsedInbound{
		EventType:  eventType,
		DeliveryID: deliveryID(req.Headers, req.Body),
		Payload:    payload,
	}, true, nil
}

// ValidateSignature freezes the upstream webhook auth behavior for GitHub,
// GitLab, and generic HMAC-SHA256 webhook senders.
func ValidateSignature(headers map[string]string, body []byte, secret string) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" || secret == InsecureNoAuth {
		return true
	}

	if got := headerValue(headers, "X-Hub-Signature-256"); got != "" {
		return hmac.Equal([]byte(got), []byte(signGitHub(secret, body)))
	}

	if got := headerValue(headers, "X-Gitlab-Token"); got != "" {
		return hmac.Equal([]byte(got), []byte(secret))
	}

	if got := headerValue(headers, "X-Webhook-Signature"); got != "" {
		return hmac.Equal([]byte(got), []byte(signGeneric(secret, body)))
	}

	return false
}

func exceedsLimit(req InboundRequest, maxBodyBytes int64) bool {
	if maxBodyBytes <= 0 {
		return false
	}
	if req.ContentLength > maxBodyBytes {
		return true
	}
	return int64(len(req.Body)) > maxBodyBytes
}

func parseBody(body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return map[string]any{}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload == nil {
			return map[string]any{}, nil
		}
		return payload, nil
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, ErrCannotParseBody
	}
	payload = make(map[string]any, len(values))
	for key, items := range values {
		if len(items) == 0 {
			payload[key] = ""
			continue
		}
		payload[key] = items[len(items)-1]
	}
	if len(payload) == 0 && strings.TrimSpace(string(body)) != "" {
		return nil, ErrCannotParseBody
	}
	return payload, nil
}

func eventType(headers map[string]string, payload map[string]any) string {
	if event := headerValue(headers, "X-GitHub-Event"); event != "" {
		return event
	}
	if event := headerValue(headers, "X-GitLab-Event"); event != "" {
		return event
	}
	if raw, ok := payload["event_type"]; ok {
		if event := strings.TrimSpace(fmt.Sprint(raw)); event != "" {
			return event
		}
	}
	return "unknown"
}

func allowsEvent(allowed []string, eventType string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, event := range allowed {
		if strings.TrimSpace(event) == eventType {
			return true
		}
	}
	return false
}

func deliveryID(headers map[string]string, body []byte) string {
	if id := headerValue(headers, "X-GitHub-Delivery"); id != "" {
		return id
	}
	if id := headerValue(headers, "X-Request-ID"); id != "" {
		return id
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func headerValue(headers map[string]string, key string) string {
	for name, value := range headers {
		if strings.EqualFold(strings.TrimSpace(name), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func signGitHub(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func signGeneric(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
