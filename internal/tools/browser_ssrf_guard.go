package tools

import (
	"net/url"
	"strings"
)

const (
	browserSSRFGuardConfigInvalid = "ssrf_guard_config_invalid"
	browserSSRFPrivateURLBlocked  = "private_url_blocked"
)

// BrowserSSRFGuardBool is a normalized bool-like browser safety config value.
type BrowserSSRFGuardBool struct {
	Value    bool
	Evidence string
}

// BrowserSSRFGuardOptions are the pure inputs needed before a browser provider
// receives a navigation URL.
type BrowserSSRFGuardOptions struct {
	CloudConfigured         bool
	AllowPrivateURLs        any
	AutoLocalForPrivateURLs any
	CDPOverride             bool
	CamofoxMode             bool
}

// BrowserSSRFGuardDecision is the pure pre-navigation cloud safety decision.
type BrowserSSRFGuardDecision struct {
	Allowed  bool
	Evidence string
	Route    BrowserRoute
}

// CoerceBrowserSSRFGuardBool normalizes bool-like config values without using
// language truthiness for strings such as "false".
func CoerceBrowserSSRFGuardBool(raw any, fallback bool) BrowserSSRFGuardBool {
	switch value := raw.(type) {
	case nil:
		return BrowserSSRFGuardBool{Value: fallback}
	case bool:
		return BrowserSSRFGuardBool{Value: value}
	case string:
		switch normalizeBrowserSSRFGuardBoolString(value) {
		case "1", "true", "yes", "on":
			return BrowserSSRFGuardBool{Value: true}
		case "0", "false", "no", "off":
			return BrowserSSRFGuardBool{Value: false}
		}
	case int:
		if value == 0 {
			return BrowserSSRFGuardBool{Value: false}
		}
		if value == 1 {
			return BrowserSSRFGuardBool{Value: true}
		}
	}
	return BrowserSSRFGuardBool{Value: fallback, Evidence: browserSSRFGuardConfigInvalid}
}

// CheckBrowserSSRFGuard determines whether rawURL may proceed to its selected
// browser route without starting a browser or resolving DNS.
func CheckBrowserSSRFGuard(taskID, rawURL string, opts BrowserSSRFGuardOptions) BrowserSSRFGuardDecision {
	allowPrivate := CoerceBrowserSSRFGuardBool(opts.AllowPrivateURLs, false)
	autoLocal := CoerceBrowserSSRFGuardBool(opts.AutoLocalForPrivateURLs, true)

	route := RouteBrowserNavigation(
		taskID,
		rawURL,
		opts.CloudConfigured,
		autoLocal.Value,
		opts.CDPOverride,
		opts.CamofoxMode,
	)
	decision := BrowserSSRFGuardDecision{Allowed: true, Route: route}

	if allowPrivate.Evidence != "" {
		decision.Allowed = false
		decision.Evidence = allowPrivate.Evidence
		return decision
	}
	if autoLocal.Evidence != "" {
		decision.Allowed = false
		decision.Evidence = autoLocal.Evidence
		return decision
	}

	if !opts.CloudConfigured || opts.CDPOverride || opts.CamofoxMode || allowPrivate.Value || route.ForceLocal {
		return decision
	}

	parsed, err := url.Parse(rawURL)
	if err == nil && IsPrivateBrowserHost(parsed.Hostname()) {
		decision.Allowed = false
		decision.Evidence = browserSSRFPrivateURLBlocked
	}
	return decision
}

func normalizeBrowserSSRFGuardBoolString(value string) string {
	trimmed := strings.TrimSpace(value)
	for len(trimmed) >= 2 {
		first := trimmed[0]
		last := trimmed[len(trimmed)-1]
		if (first != '"' && first != '\'') || first != last {
			break
		}
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	return strings.ToLower(trimmed)
}
