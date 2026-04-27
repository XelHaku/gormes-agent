package builderloop

import (
	"context"
	"errors"
	"strings"
)

type BackendFailure struct {
	Status string
	Detail string
}

func ClassifyBackendFailure(err error, stdout, stderr string) BackendFailure {
	detail := backendFailureDetail(err, stdout, stderr)
	status := "backend_failed"
	haystack := strings.ToLower(strings.Join([]string{errorText(err), stdout, stderr}, "\n"))
	switch {
	case backendWasKilled(haystack):
		status = "backend_killed"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(haystack, context.DeadlineExceeded.Error()):
		status = "backend_no_progress"
	case backendHitUsageLimit(haystack):
		status = "backend_usage_limited"
	case strings.Contains(haystack, "reading additional input from stdin"):
		status = "backend_waiting_for_stdin"
	}
	if detail == "" {
		detail = status
	} else {
		detail = status + ": " + detail
	}
	return BackendFailure{Status: status, Detail: detail}
}

func backendFailureDetail(err error, stdout, stderr string) string {
	var parts []string
	if text := errorText(err); text != "" {
		parts = append(parts, text)
	}
	for _, output := range []string{strings.TrimSpace(stderr), strings.TrimSpace(stdout)} {
		if output == "" {
			continue
		}
		alreadyIncluded := false
		for _, part := range parts {
			if strings.Contains(part, output) || strings.Contains(output, part) {
				alreadyIncluded = true
				break
			}
		}
		if !alreadyIncluded {
			parts = append(parts, output)
		}
	}
	return strings.Join(parts, ": ")
}

func backendWasKilled(text string) bool {
	return strings.Contains(text, "signal: killed") ||
		strings.Contains(text, "signal: terminated") ||
		strings.Contains(text, "signal: interrupt") ||
		strings.Contains(text, "killed")
}

func backendHitUsageLimit(text string) bool {
	return strings.Contains(text, "hit your usage limit") ||
		strings.Contains(text, "usage limit")
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
