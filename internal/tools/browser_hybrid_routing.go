package tools

import (
	"net/netip"
	"net/url"
	"strings"
)

const (
	defaultBrowserTaskID        = "default"
	localBrowserSidecarSuffix   = "::local"
	privateBrowserSidecarReason = "private_url_local_sidecar"
)

// BrowserRoute is the pure pre-navigation routing decision for a browser URL.
type BrowserRoute struct {
	SessionKey string
	ForceLocal bool
	Reason     string
}

// IsPrivateBrowserHost reports whether host is a local, private, or LAN-style
// browser target that should stay off a cloud browser provider.
func IsPrivateBrowserHost(host string) bool {
	hostname := normalizeBrowserHost(host)
	if hostname == "" {
		return false
	}
	if hostname == "localhost" ||
		strings.HasSuffix(hostname, ".local") ||
		strings.HasSuffix(hostname, ".lan") ||
		strings.HasSuffix(hostname, ".internal") {
		return true
	}

	addr, err := netip.ParseAddr(hostname)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	if addr.IsLoopback() {
		return true
	}
	return addr.Is4() && (addr.IsPrivate() || addr.IsLinkLocalUnicast())
}

// RouteBrowserNavigation selects the session key for an initial browser
// navigation without starting a browser or consulting runtime configuration.
func RouteBrowserNavigation(taskID, rawURL string, cloudConfigured, autoLocalForPrivateURLs, cdpOverride, camofoxMode bool) BrowserRoute {
	sessionKey := normalizeBrowserTaskID(taskID)
	route := BrowserRoute{SessionKey: sessionKey}
	if !cloudConfigured || !autoLocalForPrivateURLs || cdpOverride || camofoxMode {
		return route
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || !IsPrivateBrowserHost(parsed.Hostname()) {
		return route
	}

	return BrowserRoute{
		SessionKey: sessionKey + localBrowserSidecarSuffix,
		ForceLocal: true,
		Reason:     privateBrowserSidecarReason,
	}
}

func normalizeBrowserTaskID(taskID string) string {
	if taskID == "" {
		return defaultBrowserTaskID
	}
	return taskID
}

func normalizeBrowserHost(host string) string {
	hostname := strings.TrimSpace(host)
	if strings.HasPrefix(hostname, "[") {
		if closeBracket := strings.Index(hostname, "]"); closeBracket > 0 {
			hostname = hostname[1:closeBracket]
		}
	} else if strings.Count(hostname, ":") == 1 {
		if colon := strings.LastIndex(hostname, ":"); colon > 0 {
			hostname = hostname[:colon]
		}
	}
	hostname = strings.ToLower(hostname)
	return strings.TrimSuffix(hostname, ".")
}
