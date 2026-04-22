package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

var templateToken = regexp.MustCompile(`\{([a-zA-Z0-9_.]+)\}`)

// RouteConfig is the subset of webhook route config needed to turn a payload
// into a prompt plus a gateway delivery target.
type RouteConfig struct {
	Prompt       string
	Deliver      string
	DeliverExtra map[string]any
}

// PromptDelivery is the typed bridge result used by future webhook or generic
// trigger ingress code.
type PromptDelivery struct {
	SessionChatID string
	Prompt        string
	Target        gateway.DeliveryTarget
	HasTarget     bool
}

// BuildPromptDelivery renders the prompt template, derives the deterministic
// webhook session key, and resolves a gateway delivery target when the route
// points at a gateway-backed platform.
func BuildPromptDelivery(routeName, deliveryID, eventType string, route RouteConfig, payload map[string]any) (PromptDelivery, error) {
	routeName = trim(routeName)
	deliveryID = trim(deliveryID)
	if routeName == "" {
		return PromptDelivery{}, errors.New("webhook: route name is required")
	}
	if deliveryID == "" {
		return PromptDelivery{}, errors.New("webhook: delivery ID is required")
	}
	if payload == nil {
		payload = map[string]any{}
	}

	prompt := renderPrompt(route.Prompt, payload, eventType, routeName)
	result := PromptDelivery{
		SessionChatID: sessionChatID(routeName, deliveryID),
		Prompt:        prompt,
	}

	renderedExtra := renderDeliveryExtra(route.DeliverExtra, payload)
	target, hasTarget, err := resolveTarget(route.Deliver, renderedExtra)
	if err != nil {
		return PromptDelivery{}, err
	}
	result.Target = target
	result.HasTarget = hasTarget
	return result, nil
}

func renderPrompt(template string, payload map[string]any, eventType, routeName string) string {
	if trim(template) == "" {
		truncated := mustJSON(payload, 4000)
		return "Webhook event '" + trim(eventType) + "' on route '" + routeName + "':\n\n```json\n" + truncated + "\n```"
	}

	return templateToken.ReplaceAllStringFunc(template, func(token string) string {
		key := token[1 : len(token)-1]
		if key == "__raw__" {
			return mustJSON(payload, 4000)
		}
		value, ok := resolvePayloadValue(payload, key)
		if !ok {
			return token
		}
		switch typed := value.(type) {
		case map[string]any, []any:
			return mustJSON(typed, 2000)
		default:
			return fmt.Sprint(typed)
		}
	})
}

func renderDeliveryExtra(extra map[string]any, payload map[string]any) map[string]any {
	if len(extra) == 0 {
		return nil
	}
	rendered := make(map[string]any, len(extra))
	for key, value := range extra {
		if text, ok := value.(string); ok {
			rendered[key] = renderPrompt(text, payload, "", "")
			continue
		}
		rendered[key] = value
	}
	return rendered
}

func resolveTarget(deliver string, extra map[string]any) (gateway.DeliveryTarget, bool, error) {
	deliver = strings.ToLower(trim(deliver))
	switch deliver {
	case "", "log", "github_comment":
		return gateway.DeliveryTarget{}, false, nil
	}

	chatID := stringValue(extra, "chat_id")
	threadID := threadIDFromExtra(extra)

	if threadID != "" && chatID == "" {
		return gateway.DeliveryTarget{}, false, errors.New("webhook: threaded delivery requires chat_id")
	}

	raw := deliver
	if chatID != "" {
		raw += ":" + chatID
	}
	if threadID != "" {
		raw += ":" + threadID
	}

	target, err := gateway.ParseDeliveryTarget(raw, nil)
	if err != nil {
		return gateway.DeliveryTarget{}, false, err
	}
	return target, true, nil
}

func resolvePayloadValue(payload map[string]any, dotted string) (any, bool) {
	current := any(payload)
	for _, part := range strings.Split(dotted, ".") {
		next, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = next[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringValue(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok {
		return ""
	}
	return trim(fmt.Sprint(raw))
}

func threadIDFromExtra(extra map[string]any) string {
	threadID := stringValue(extra, "thread_id")
	if threadID != "" {
		return threadID
	}
	return stringValue(extra, "message_thread_id")
}

func sessionChatID(routeName, deliveryID string) string {
	return "webhook:" + routeName + ":" + deliveryID
}

func mustJSON(value any, limit int) string {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	out := string(b)
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}

func trim(value string) string {
	return strings.TrimSpace(value)
}
