package gateway

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// HomeChannel is the platform home destination for proactive notifications.
// It records both the chat target and the operator who most recently claimed
// that chat with /sethome.
type HomeChannel struct {
	Platform      string
	ChatID        string
	ChatName      string
	ThreadID      string
	SetByUserID   string
	SetByUserName string
}

// HomeChannels stores home-channel ownership in memory for the current
// gateway process. Persistence can layer on top of this stable contract later.
type HomeChannels struct {
	mu    sync.RWMutex
	homes map[string]HomeChannel
}

// NewHomeChannels returns an empty in-memory home-channel directory.
func NewHomeChannels() *HomeChannels {
	return &HomeChannels{homes: map[string]HomeChannel{}}
}

// SetFromInbound marks the inbound chat as the platform home channel.
func (h *HomeChannels) SetFromInbound(ev InboundEvent) (HomeChannel, bool) {
	if h == nil {
		return HomeChannel{}, false
	}
	platform := normalizePlatform(ev.Platform)
	chatID := strings.TrimSpace(ev.ChatID)
	if platform == "" || chatID == "" {
		return HomeChannel{}, false
	}

	home := HomeChannel{
		Platform:      platform,
		ChatID:        chatID,
		ChatName:      strings.TrimSpace(ev.ChatName),
		ThreadID:      strings.TrimSpace(ev.ThreadID),
		SetByUserID:   strings.TrimSpace(ev.UserID),
		SetByUserName: strings.TrimSpace(ev.UserName),
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.homes[platform] = home
	return home, true
}

// Lookup returns the configured home channel for a platform.
func (h *HomeChannels) Lookup(platform string) (HomeChannel, bool) {
	if h == nil {
		return HomeChannel{}, false
	}
	platform = normalizePlatform(platform)
	if platform == "" {
		return HomeChannel{}, false
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	home, ok := h.homes[platform]
	return home, ok
}

// Snapshot returns a deterministic copy of configured home channels.
func (h *HomeChannels) Snapshot() []HomeChannel {
	if h == nil {
		return nil
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]HomeChannel, 0, len(h.homes))
	for _, home := range h.homes {
		out = append(out, home)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Platform < out[j].Platform
	})
	return out
}

func homeChannelDisplayName(home HomeChannel) string {
	name := strings.TrimSpace(home.ChatName)
	if name != "" {
		return name
	}
	return strings.TrimSpace(home.ChatID)
}

func homeChannelSetMessage(home HomeChannel) string {
	return fmt.Sprintf(
		"Home channel set to **%s** (ID: %s).\nCron jobs and cross-platform messages will be delivered here.",
		homeChannelDisplayName(home),
		home.ChatID,
	)
}
