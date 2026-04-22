package gateway

import (
	"errors"
	"strings"
)

// DeliveryTarget is a parsed --deliver destination.
type DeliveryTarget struct {
	Platform   string
	ChatID     string
	ThreadID   string
	IsOrigin   bool
	IsExplicit bool
}

func (t DeliveryTarget) String() string {
	if t.IsOrigin {
		return "origin"
	}
	platform := strings.ToLower(strings.TrimSpace(t.Platform))
	if platform == "local" || platform == "" {
		return "local"
	}
	if t.ChatID == "" {
		return platform
	}
	if t.ThreadID == "" {
		return platform + ":" + t.ChatID
	}
	return platform + ":" + t.ChatID + ":" + t.ThreadID
}

// ParseDeliveryTarget converts a single --deliver token into a typed target.
// Parsing is syntax-only; runtime availability checks happen later when a
// router binds targets to concrete platform channels.
func ParseDeliveryTarget(raw string, origin *SessionSource) (DeliveryTarget, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DeliveryTarget{}, errors.New("gateway: empty delivery target")
	}
	if strings.EqualFold(trimmed, "origin") {
		if origin == nil {
			return DeliveryTarget{Platform: "local", IsOrigin: true}, nil
		}
		return DeliveryTarget{
			Platform: strings.ToLower(strings.TrimSpace(origin.Platform)),
			ChatID:   strings.TrimSpace(origin.ChatID),
			ThreadID: strings.TrimSpace(origin.ThreadID),
			IsOrigin: true,
		}, nil
	}
	if strings.EqualFold(trimmed, "local") {
		return DeliveryTarget{Platform: "local"}, nil
	}

	parts := strings.Split(trimmed, ":")
	switch len(parts) {
	case 1:
		platform := strings.ToLower(strings.TrimSpace(parts[0]))
		if platform == "" {
			return DeliveryTarget{}, errors.New("gateway: empty delivery platform")
		}
		return DeliveryTarget{Platform: platform}, nil
	case 2:
		platform := strings.ToLower(strings.TrimSpace(parts[0]))
		chatID := strings.TrimSpace(parts[1])
		if platform == "" || chatID == "" {
			return DeliveryTarget{}, errors.New("gateway: invalid explicit delivery target")
		}
		return DeliveryTarget{Platform: platform, ChatID: chatID, IsExplicit: true}, nil
	case 3:
		platform := strings.ToLower(strings.TrimSpace(parts[0]))
		chatID := strings.TrimSpace(parts[1])
		threadID := strings.TrimSpace(parts[2])
		if platform == "" || chatID == "" || threadID == "" {
			return DeliveryTarget{}, errors.New("gateway: invalid threaded delivery target")
		}
		return DeliveryTarget{
			Platform:   platform,
			ChatID:     chatID,
			ThreadID:   threadID,
			IsExplicit: true,
		}, nil
	default:
		return DeliveryTarget{}, errors.New("gateway: invalid delivery target")
	}
}
