package gateway

import (
	"context"
	"sort"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

// SessionSource describes the gateway-facing origin of a turn.
type SessionSource struct {
	Platform string
	ChatID   string
	ChatName string
	ChatType string
	UserID   string
	UserName string
	ThreadID string
}

// SessionContext is the deterministic per-turn prompt block the gateway
// injects so the agent knows where the turn came from and which delivery
// targets are available.
type SessionContext struct {
	Source             SessionSource
	SessionKey         string
	SessionID          string
	ConnectedPlatforms []string
	// ResumePending, when non-nil, carries a drain-timeout resume flag
	// captured by the previous restart/shutdown cycle. If the flag passes
	// render gating (running agent + preserved session_id), the rendered
	// prompt prepends a reason-aware "Resume Pending" header so the next
	// turn picks up where the interrupted turn left off.
	ResumePending *ResumePending
}

func sessionSourceFromInbound(ev InboundEvent) SessionSource {
	chatType := "dm"
	if strings.TrimSpace(ev.ThreadID) != "" {
		chatType = "thread"
	}
	return SessionSource{
		Platform: strings.ToLower(strings.TrimSpace(ev.Platform)),
		ChatID:   strings.TrimSpace(ev.ChatID),
		ChatName: strings.TrimSpace(ev.ChatName),
		ChatType: chatType,
		UserID:   strings.TrimSpace(ev.UserID),
		UserName: strings.TrimSpace(ev.UserName),
		ThreadID: strings.TrimSpace(ev.ThreadID),
	}
}

func resolveSessionID(ctx context.Context, smap session.Map, chatKey string) (string, error) {
	key := strings.TrimSpace(chatKey)
	if key == "" || smap == nil {
		return key, nil
	}
	stored, err := smap.Get(ctx, key)
	if err != nil {
		return key, err
	}
	if stored = strings.TrimSpace(stored); stored != "" {
		return stored, nil
	}
	return key, nil
}

// BuildSessionContextPrompt renders the gateway's per-turn session metadata as
// a stable system block. Ordering is deterministic so prompt caching and tests
// stay predictable.
func BuildSessionContextPrompt(ctx SessionContext) string {
	source := ctx.Source
	lines := []string{
		"## Current Session Context",
		"",
	}

	platform := strings.ToLower(strings.TrimSpace(source.Platform))
	chatID := strings.TrimSpace(source.ChatID)
	switch {
	case platform == "" && chatID == "":
		lines = append(lines, "**Source:** unknown")
	case chatID == "":
		lines = append(lines, "**Source:** "+platform)
	default:
		lines = append(lines, "**Source:** "+platform+" chat `"+chatID+"`")
	}
	if userID := strings.TrimSpace(source.UserID); userID != "" {
		lines = append(lines, "**User ID:** `"+userID+"`")
	}
	if threadID := strings.TrimSpace(source.ThreadID); threadID != "" {
		lines = append(lines, "**Thread ID:** `"+threadID+"`")
	}
	if key := strings.TrimSpace(ctx.SessionKey); key != "" {
		lines = append(lines, "**Session Key:** `"+key+"`")
	}
	if sessionID := strings.TrimSpace(ctx.SessionID); sessionID != "" {
		lines = append(lines, "**Session ID:** `"+sessionID+"`")
	}

	targets := []string{"`origin`", "`local`"}
	if len(ctx.ConnectedPlatforms) > 0 {
		seen := make(map[string]struct{}, len(ctx.ConnectedPlatforms))
		platforms := make([]string, 0, len(ctx.ConnectedPlatforms))
		for _, name := range ctx.ConnectedPlatforms {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			platforms = append(platforms, name)
		}
		sort.Strings(platforms)
		for _, name := range platforms {
			targets = append(targets, "`"+name+"`")
		}
	}
	lines = append(lines, "**Delivery Targets:** "+strings.Join(targets, ", "))
	rendered := strings.Join(lines, "\n")
	if ctx.ResumePending != nil {
		if note := BuildResumeNote(*ctx.ResumePending); note != "" {
			rendered = note + "\n\n" + rendered
		}
	}
	return rendered
}
