package hermes

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ErrorClass int

const (
	ClassUnknown ErrorClass = iota
	ClassRetryable
	ClassFatal
)

func (c ErrorClass) String() string {
	switch c {
	case ClassRetryable:
		return "retryable"
	case ClassFatal:
		return "fatal"
	}
	return "unknown"
}

type HTTPError struct {
	Status     int
	Body       string
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return http.StatusText(e.Status) + ": " + e.Body
}

const maxRetryAfterHint = 16 * time.Second

func newHTTPError(status int, body string, header http.Header) *HTTPError {
	return &HTTPError{
		Status:     status,
		Body:       body,
		RetryAfter: parseRetryAfterHint(header.Get("Retry-After"), body, time.Now()),
	}
}

type ProviderErrorKind string

const (
	ProviderErrorUnknown      ProviderErrorKind = "unknown"
	ProviderErrorRateLimit    ProviderErrorKind = "rate_limit"
	ProviderErrorAuth         ProviderErrorKind = "auth"
	ProviderErrorContext      ProviderErrorKind = "context"
	ProviderErrorRetryable    ProviderErrorKind = "retryable"
	ProviderErrorNonRetryable ProviderErrorKind = "non_retryable"
)

func (k ProviderErrorKind) String() string {
	if k == "" {
		return string(ProviderErrorUnknown)
	}
	return string(k)
}

type ProviderErrorClassification struct {
	Kind                   ProviderErrorKind
	Class                  ErrorClass
	Status                 int
	Message                string
	Retryable              bool
	ShouldCompress         bool
	ShouldRotateCredential bool
	ShouldFallback         bool
}

// Classify inspects an error produced anywhere in the hermes pipeline and
// categorises it so the kernel can decide whether to retry or abort.
func Classify(err error) ErrorClass {
	return ClassifyProviderError(err).Class
}

// ClassifyProviderError returns a structured provider-error envelope for
// status reporting and future recovery decisions. Retry-After hint parsing is
// owned by a separate 4.H slice, so this function only classifies failures.
func ClassifyProviderError(err error) ProviderErrorClassification {
	if err == nil {
		return providerError(ProviderErrorUnknown, ClassUnknown, 0, "", false)
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		message, combined, code := providerHTTPErrorText(httpErr)

		if httpErr.Status == http.StatusTooManyRequests ||
			containsAny(combined, rateLimitPatterns) ||
			isRateLimitCode(code) {
			out := providerError(ProviderErrorRateLimit, ClassRetryable, httpErr.Status, message, true)
			out.ShouldRotateCredential = true
			out.ShouldFallback = true
			return out
		}
		if httpErr.Status == http.StatusUnauthorized ||
			httpErr.Status == http.StatusForbidden ||
			containsAny(combined, authPatterns) {
			out := providerError(ProviderErrorAuth, ClassFatal, httpErr.Status, message, false)
			out.ShouldRotateCredential = true
			out.ShouldFallback = true
			return out
		}
		if httpErr.Status == http.StatusRequestEntityTooLarge ||
			containsAny(combined, contextPatterns) ||
			isContextCode(code) {
			out := providerError(ProviderErrorContext, ClassFatal, httpErr.Status, message, false)
			out.ShouldCompress = true
			return out
		}
		switch httpErr.Status {
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return providerError(ProviderErrorRetryable, ClassRetryable, httpErr.Status, message, true)
		}
		if httpErr.Status == 529 {
			return providerError(ProviderErrorRetryable, ClassRetryable, httpErr.Status, message, true)
		}
		if httpErr.Status >= 400 && httpErr.Status < 500 {
			return providerError(ProviderErrorNonRetryable, ClassFatal, httpErr.Status, message, false)
		}
		return providerError(ProviderErrorUnknown, ClassUnknown, httpErr.Status, message, false)
	}
	message := err.Error()
	combined := strings.ToLower(message)
	if containsAny(combined, rateLimitPatterns) {
		out := providerError(ProviderErrorRateLimit, ClassRetryable, 0, message, true)
		out.ShouldRotateCredential = true
		out.ShouldFallback = true
		return out
	}
	if containsAny(combined, authPatterns) {
		out := providerError(ProviderErrorAuth, ClassFatal, 0, message, false)
		out.ShouldRotateCredential = true
		out.ShouldFallback = true
		return out
	}
	if containsAny(combined, contextPatterns) {
		out := providerError(ProviderErrorContext, ClassFatal, 0, message, false)
		out.ShouldCompress = true
		return out
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return providerError(ProviderErrorRetryable, ClassRetryable, 0, message, true)
	}
	return providerError(ProviderErrorUnknown, ClassUnknown, 0, message, false)
}

func providerError(kind ProviderErrorKind, class ErrorClass, status int, message string, retryable bool) ProviderErrorClassification {
	return ProviderErrorClassification{
		Kind:      kind,
		Class:     class,
		Status:    status,
		Message:   message,
		Retryable: retryable,
	}
}

var rateLimitPatterns = []string{
	"rate limit",
	"rate_limit",
	"too many requests",
	"throttled",
	"throttlingexception",
	"servicequotaexceededexception",
	"resource_exhausted",
	"requests per minute",
	"tokens per minute",
	"try again in",
	"retry after",
	"rate increased too quickly",
	"too many concurrent requests",
}

var authPatterns = []string{
	"invalid api key",
	"invalid_api_key",
	"authentication",
	"unauthorized",
	"forbidden",
	"invalid token",
	"token expired",
	"token revoked",
	"access denied",
}

var contextPatterns = []string{
	"context length",
	"context size",
	"maximum context",
	"token limit",
	"too many tokens",
	"reduce the length",
	"exceeds the limit",
	"context window",
	"prompt is too long",
	"prompt exceeds max length",
	"maximum number of tokens",
	"exceeds the max_model_len",
	"max_model_len",
	"prompt length",
	"input is too long",
	"maximum model length",
	"context length exceeded",
	"slot context",
	"n_ctx_slot",
	"超过最大长度",
	"上下文长度",
	"max input token",
	"input token",
	"exceeds the maximum number of input tokens",
}

func containsAny(s string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func isRateLimitCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "rate_limit", "rate_limit_exceeded", "resource_exhausted", "throttled", "throttlingexception":
		return true
	}
	return false
}

func isContextCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "context_length_exceeded", "context_overflow", "max_tokens_exceeded":
		return true
	}
	return false
}

func isUnsupportedTemperatureError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		_, combined, code := providerHTTPErrorText(httpErr)
		return isUnsupportedTemperatureSignal(combined, code)
	}
	return isUnsupportedTemperatureSignal(strings.ToLower(err.Error()), "")
}

var unsupportedTemperaturePatterns = []string{
	"unsupported parameter",
	"unsupported_parameter",
	"not supported",
	"does not support",
	"unknown parameter",
	"unrecognized request argument",
}

func isUnsupportedTemperatureSignal(combined, code string) bool {
	combined = strings.ToLower(combined)
	if !strings.Contains(combined, "temperature") {
		return false
	}
	if isUnsupportedParameterCode(code) {
		return true
	}
	return containsAny(combined, unsupportedTemperaturePatterns)
}

func isUnsupportedParameterCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "unsupported_parameter", "unknown_parameter":
		return true
	}
	return false
}

func providerHTTPErrorText(err *HTTPError) (message, combined, code string) {
	body := strings.TrimSpace(err.Body)
	message = body
	parts := []string{body}
	if body != "" {
		var decoded any
		if json.Unmarshal([]byte(body), &decoded) == nil {
			extractedMessage, extractedCode, extractedRaw := providerBodySignals(decoded)
			if extractedMessage != "" {
				message = extractedMessage
				parts = append(parts, extractedMessage)
			}
			if extractedCode != "" {
				code = extractedCode
				parts = append(parts, extractedCode)
			}
			if extractedRaw != "" {
				parts = append(parts, extractedRaw)
			}
		}
	}
	combined = strings.ToLower(strings.Join(parts, " "))
	return message, combined, code
}

func providerBodySignals(v any) (message, code, raw string) {
	obj, ok := v.(map[string]any)
	if !ok {
		return "", "", ""
	}
	if errObj, ok := obj["error"].(map[string]any); ok {
		message = stringField(errObj["message"])
		code = firstStringField(errObj, "code", "type")
		if metadata, ok := errObj["metadata"].(map[string]any); ok {
			raw = stringField(metadata["raw"])
			if raw != "" {
				if rawMessage, rawCode := providerRawSignals(raw); rawMessage != "" || rawCode != "" {
					raw = strings.TrimSpace(raw + " " + rawMessage + " " + rawCode)
				}
			}
		}
		return message, code, raw
	}
	message = stringField(obj["message"])
	code = firstStringField(obj, "code", "error_code", "type")
	return message, code, ""
}

func providerRawSignals(raw string) (message, code string) {
	var decoded any
	if json.Unmarshal([]byte(raw), &decoded) != nil {
		return "", ""
	}
	message, code, _ = providerBodySignals(decoded)
	return message, code
}

func firstStringField(obj map[string]any, names ...string) string {
	for _, name := range names {
		if s := stringField(obj[name]); s != "" {
			return s
		}
	}
	return ""
}

func stringField(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	default:
		return ""
	}
}

func parseRetryAfterHint(headerValue, body string, now time.Time) time.Duration {
	if d := parseRetryAfterHeader(headerValue, now); d > 0 {
		return capRetryAfterHint(d)
	}
	if d := parseRetryAfterBody(body); d > 0 {
		return capRetryAfterHint(d)
	}
	return 0
}

func parseRetryAfterHeader(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}

func parseRetryAfterBody(body string) time.Duration {
	body = strings.TrimSpace(body)
	if body == "" {
		return 0
	}
	var decoded any
	if json.Unmarshal([]byte(body), &decoded) != nil {
		return 0
	}
	return retryAfterFromValue(decoded)
}

func retryAfterFromValue(v any) time.Duration {
	switch x := v.(type) {
	case map[string]any:
		if d := retryAfterDuration(x["retry_after"]); d > 0 {
			return d
		}
		if d := retryAfterDuration(x["retryAfter"]); d > 0 {
			return d
		}
		for _, child := range x {
			if d := retryAfterFromValue(child); d > 0 {
				return d
			}
		}
	case []any:
		for _, child := range x {
			if d := retryAfterFromValue(child); d > 0 {
				return d
			}
		}
	}
	return 0
}

func retryAfterDuration(v any) time.Duration {
	switch x := v.(type) {
	case float64:
		if x <= 0 {
			return 0
		}
		return time.Duration(x * float64(time.Second))
	case json.Number:
		seconds, err := strconv.ParseFloat(x.String(), 64)
		if err != nil || seconds <= 0 {
			return 0
		}
		return time.Duration(seconds * float64(time.Second))
	case string:
		seconds, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil || seconds <= 0 {
			return 0
		}
		return time.Duration(seconds * float64(time.Second))
	default:
		return 0
	}
}

func capRetryAfterHint(d time.Duration) time.Duration {
	if d > maxRetryAfterHint {
		return maxRetryAfterHint
	}
	return d
}
