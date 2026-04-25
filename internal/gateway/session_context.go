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
	GuildID  string
	// ParentChatID preserves the containing channel/forum for threaded sources.
	ParentChatID string
	// MessageID identifies the triggering platform message when the adapter
	// explicitly supplies source metadata for it.
	MessageID string
}

// SessionContext is the deterministic per-turn prompt block the gateway
// injects so the agent knows where the turn came from and which delivery
// targets are available.
type SessionContext struct {
	Source             SessionSource
	SessionKey         string
	SessionID          string
	RequestedSessionID string
	ResumePath         []string
	ResumeStatus       string
	ConnectedPlatforms []string
}

type resolvedSession struct {
	SessionID          string
	RequestedSessionID string
	ResumePath         []string
	ResumeStatus       string
}

func sessionSourceFromInbound(ev InboundEvent) SessionSource {
	platform := strings.ToLower(strings.TrimSpace(ev.Platform))
	chatType := "dm"
	if strings.TrimSpace(ev.ThreadID) != "" {
		chatType = "thread"
	}
	messageID := strings.TrimSpace(ev.MessageID)
	if messageID == "" && platform == "discord" {
		messageID = strings.TrimSpace(ev.MsgID)
	}
	return SessionSource{
		Platform:     platform,
		ChatID:       strings.TrimSpace(ev.ChatID),
		ChatName:     strings.TrimSpace(ev.ChatName),
		ChatType:     chatType,
		UserID:       strings.TrimSpace(ev.UserID),
		UserName:     strings.TrimSpace(ev.UserName),
		ThreadID:     strings.TrimSpace(ev.ThreadID),
		GuildID:      strings.TrimSpace(ev.GuildID),
		ParentChatID: strings.TrimSpace(ev.ParentChatID),
		MessageID:    messageID,
	}
}

func resolveSessionID(ctx context.Context, smap session.Map, chatKey string) (string, error) {
	resolved, err := resolveSession(ctx, smap, chatKey)
	return resolved.SessionID, err
}

func resolveSession(ctx context.Context, smap session.Map, chatKey string) (resolvedSession, error) {
	key := strings.TrimSpace(chatKey)
	if key == "" || smap == nil {
		return resolvedSession{SessionID: key}, nil
	}
	stored, err := smap.Get(ctx, key)
	if err != nil {
		return resolvedSession{SessionID: key}, err
	}
	if stored = strings.TrimSpace(stored); stored != "" {
		resolved := resolvedSession{SessionID: stored}
		resolver, ok := smap.(session.LineageResolver)
		if !ok {
			return resolved, nil
		}
		lineage, err := resolver.ResolveLineageTip(ctx, stored)
		if err != nil {
			resolved.RequestedSessionID = stored
			resolved.ResumeStatus = session.LineageStatusError
			return resolved, err
		}
		status := strings.TrimSpace(lineage.Status)
		if status == "" {
			status = session.LineageStatusOK
		}
		resolved.ResumePath = cleanResumePath(lineage.Path)
		if status != session.LineageStatusOK {
			if status != session.LineageStatusMissing || len(resolved.ResumePath) > 1 {
				resolved.RequestedSessionID = stored
				resolved.ResumeStatus = status
			}
			if resolved.ResumeStatus == "" {
				resolved.ResumePath = nil
			}
			return resolved, nil
		}
		if live := strings.TrimSpace(lineage.LiveSessionID); live != "" {
			resolved.SessionID = live
		}
		if resolved.SessionID == stored {
			resolved.ResumePath = nil
		} else {
			resolved.RequestedSessionID = stored
		}
		return resolved, nil
	}
	return resolvedSession{SessionID: key}, nil
}

func cleanResumePath(path []string) []string {
	out := make([]string, 0, len(path))
	for _, id := range path {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
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
	if guildID := strings.TrimSpace(source.GuildID); guildID != "" {
		lines = append(lines, "**Guild ID:** `"+guildID+"`")
	}
	if parentChatID := strings.TrimSpace(source.ParentChatID); parentChatID != "" {
		lines = append(lines, "**Parent Chat ID:** `"+parentChatID+"`")
	}
	if threadID := strings.TrimSpace(source.ThreadID); threadID != "" {
		lines = append(lines, "**Thread ID:** `"+threadID+"`")
	}
	if messageID := strings.TrimSpace(source.MessageID); messageID != "" {
		lines = append(lines, "**Message ID:** `"+messageID+"`")
	}
	if key := strings.TrimSpace(ctx.SessionKey); key != "" {
		lines = append(lines, "**Session Key:** `"+key+"`")
	}
	if sessionID := strings.TrimSpace(ctx.SessionID); sessionID != "" {
		lines = append(lines, "**Session ID:** `"+sessionID+"`")
	}
	if requested := strings.TrimSpace(ctx.RequestedSessionID); requested != "" {
		lines = append(lines, "**Requested Session ID:** `"+requested+"`")
	}
	if len(ctx.ResumePath) > 1 {
		parts := make([]string, 0, len(ctx.ResumePath))
		for _, id := range ctx.ResumePath {
			if id = strings.TrimSpace(id); id != "" {
				parts = append(parts, "`"+id+"`")
			}
		}
		if len(parts) > 1 {
			lines = append(lines, "**Resume Continuation:** "+strings.Join(parts, " -> "))
		}
	}
	if status := strings.TrimSpace(ctx.ResumeStatus); status != "" && status != session.LineageStatusOK {
		lines = append(lines, "**Resume Continuation Status:** `"+status+"`")
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
	return strings.Join(lines, "\n")
}
