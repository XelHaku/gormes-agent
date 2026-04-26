package cli

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Sentinel errors returned by NormalizeWebhookURL. Callers should branch on
// these via errors.Is so wrapping context can be added without breaking
// classification.
var (
	ErrWebhookURLEmpty             = errors.New("webhook URL is empty")
	ErrWebhookURLBadScheme         = errors.New("webhook URL must use http or https scheme")
	ErrWebhookURLUserInfoForbidden = errors.New("webhook URL must not embed userinfo credentials")
	ErrWebhookURLParseFailed       = errors.New("webhook URL is not parsable")
)

// NormalizeWebhookURL canonicalizes an operator-supplied webhook URL without
// touching the network. It trims surrounding whitespace, requires an http or
// https scheme, lowercases the host, strips trailing slashes from the path
// (including reducing a "/" root to no path at all), preserves the query and
// fragment, and refuses URLs that embed userinfo credentials. The returned
// error is one of the sentinel values declared above so callers can branch
// on the typed failure mode instead of inspecting strings.
func NormalizeWebhookURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrWebhookURLEmpty
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrWebhookURLParseFailed, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrWebhookURLBadScheme
	}
	u.Scheme = scheme

	if u.User != nil {
		return "", ErrWebhookURLUserInfoForbidden
	}

	u.Host = strings.ToLower(u.Host)

	for strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	for strings.HasSuffix(u.RawPath, "/") {
		u.RawPath = strings.TrimSuffix(u.RawPath, "/")
	}

	return u.String(), nil
}
