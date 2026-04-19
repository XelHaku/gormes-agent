package hermes

import (
	"errors"
	"net"
	"net/http"
	"strings"
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
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return http.StatusText(e.Status) + ": " + e.Body
}

// Classify inspects an error produced anywhere in the hermes pipeline and
// categorises it so the kernel can decide whether to retry or abort.
func Classify(err error) ErrorClass {
	if err == nil {
		return ClassUnknown
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.Status {
		case 429, 500, 502, 503, 504:
			return ClassRetryable
		case 401, 403, 404:
			return ClassFatal
		}
		if strings.Contains(strings.ToLower(httpErr.Body), "context length") {
			return ClassFatal
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ClassRetryable
	}
	return ClassUnknown
}
