package gateway

import (
	"context"
	"fmt"
	"strings"
)

// UnauthorizedDMBehavior controls the shared response contract for direct
// messages from users that did not pass gateway authorization.
type UnauthorizedDMBehavior string

const (
	UnauthorizedDMDeny   UnauthorizedDMBehavior = "deny"
	UnauthorizedDMPair   UnauthorizedDMBehavior = "pair"
	UnauthorizedDMIgnore UnauthorizedDMBehavior = "ignore"
)

// UnauthorizedDMDenialText is intentionally terse so it does not disclose
// configured chats, sessions, or pairing state.
const UnauthorizedDMDenialText = "Access denied."

// UnauthorizedDMPolicy carries the shared gateway policy dependencies for
// unknown direct-message senders.
type UnauthorizedDMPolicy struct {
	Behavior     UnauthorizedDMBehavior
	PairingStore *PairingStore
}

// UnauthorizedDMDecision reports the policy outcome without exposing
// authorized-session state.
type UnauthorizedDMDecision struct {
	Handled       bool
	StartAgent    bool
	ReplySent     bool
	PairingStatus PairingCodeStatus
}

// NormalizeUnauthorizedDMBehavior returns a supported unauthorized-DM mode.
// The open-gateway default mirrors upstream Hermes: unknown DMs are offered a
// pairing code unless the operator configures a quieter mode.
func NormalizeUnauthorizedDMBehavior(behavior UnauthorizedDMBehavior) UnauthorizedDMBehavior {
	switch UnauthorizedDMBehavior(strings.ToLower(strings.TrimSpace(string(behavior)))) {
	case UnauthorizedDMDeny:
		return UnauthorizedDMDeny
	case UnauthorizedDMIgnore:
		return UnauthorizedDMIgnore
	case UnauthorizedDMPair:
		return UnauthorizedDMPair
	default:
		return UnauthorizedDMPair
	}
}

// HandleUnauthorizedDM applies the shared unauthorized-direct-message policy.
// Callers invoke this only after normal authorization fails; the function never
// starts an agent turn.
func HandleUnauthorizedDM(ctx context.Context, ch Channel, ev InboundEvent, policy UnauthorizedDMPolicy) (UnauthorizedDMDecision, error) {
	decision := UnauthorizedDMDecision{Handled: true}
	if !ev.IsDirectMessage() {
		return decision, nil
	}

	switch NormalizeUnauthorizedDMBehavior(policy.Behavior) {
	case UnauthorizedDMDeny:
		if policy.PairingStore != nil {
			result, err := policy.PairingStore.GeneratePairingCode(ctx, PairingCodeRequestFromInbound(ev, true))
			if err != nil {
				return decision, err
			}
			decision.PairingStatus = result.Status
		}
		return sendUnauthorizedDMReply(ctx, ch, ev.ChatID, UnauthorizedDMDenialText, decision)
	case UnauthorizedDMIgnore:
		return decision, nil
	case UnauthorizedDMPair:
		if policy.PairingStore == nil {
			return decision, nil
		}
		result, err := policy.PairingStore.GeneratePairingCode(ctx, PairingCodeRequestFromInbound(ev, false))
		if err != nil {
			return decision, err
		}
		decision.PairingStatus = result.Status
		if result.Status != PairingCodeIssued {
			return decision, nil
		}
		text := FormatUnauthorizedDMPairingPrompt(ev.Platform, result.Code)
		return sendUnauthorizedDMReply(ctx, ch, ev.ChatID, text, decision)
	default:
		return decision, nil
	}
}

// FormatUnauthorizedDMPairingPrompt returns the bounded public prompt sent for
// pair-mode unauthorized DMs.
func FormatUnauthorizedDMPairingPrompt(platformName, code string) string {
	platformName = strings.TrimSpace(platformName)
	code = strings.TrimSpace(code)
	return fmt.Sprintf("Hi. I don't recognize this DM yet.\n\nPairing code: `%s`\nAsk the operator to run: `gormes pairing approve %s %s`", code, platformName, code)
}

func sendUnauthorizedDMReply(ctx context.Context, ch Channel, chatID, text string, decision UnauthorizedDMDecision) (UnauthorizedDMDecision, error) {
	if ch == nil {
		return decision, nil
	}
	if _, err := ch.Send(ctx, chatID, text); err != nil {
		return decision, err
	}
	decision.ReplySent = true
	return decision, nil
}
