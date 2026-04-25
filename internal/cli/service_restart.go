package cli

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultServiceRestartDelay preserves the current hard-restart behavior:
	// absent RestartSec evidence does not add cooldown time to the caller's
	// base transition timeout.
	DefaultServiceRestartDelay = 0

	// DefaultMaxServiceRestartDelay prevents malformed or unexpectedly huge
	// service-manager output from producing an unbounded wait budget.
	DefaultMaxServiceRestartDelay = 5 * time.Minute
)

type ServiceManagerKind string

const (
	ServiceManagerSystemd     ServiceManagerKind = "systemd"
	ServiceManagerUnsupported ServiceManagerKind = "unsupported"
)

type ServiceRestartDelayEvidenceKind string

const (
	RestartDelayDefaulted     ServiceRestartDelayEvidenceKind = "restart_delay_defaulted"
	RestartDelayMalformed     ServiceRestartDelayEvidenceKind = "restart_delay_malformed"
	RestartDelayInfinite      ServiceRestartDelayEvidenceKind = "restart_delay_infinite"
	RestartDelayMissing       ServiceRestartDelayEvidenceKind = "restart_delay_missing"
	RestartDelayUnsupported   ServiceRestartDelayEvidenceKind = "restart_delay_unsupported"
	ServiceManagerUnavailable ServiceRestartDelayEvidenceKind = "service_manager_unavailable"
	RestartDelayBounded       ServiceRestartDelayEvidenceKind = "restart_delay_bounded"
)

type ServiceRestartDelaySource struct {
	Manager      ServiceManagerKind
	Output       string
	Unavailable  bool
	DefaultDelay time.Duration
	MaxDelay     time.Duration
}

type ServiceRestartDelayReport struct {
	Delay    time.Duration
	Evidence []ServiceRestartDelayEvidence
}

type ServiceRestartDelayEvidence struct {
	Kind     ServiceRestartDelayEvidenceKind
	Manager  ServiceManagerKind
	Property string
	Raw      string
	Detail   string
}

var systemdDurationTokenRE = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)([A-Za-z]*)$`)

func ParseServiceRestartDelay(source ServiceRestartDelaySource) ServiceRestartDelayReport {
	defaultDelay := boundedDefaultDelay(source.DefaultDelay, source.MaxDelay)
	maxDelay := restartDelayMax(source.MaxDelay)

	manager := source.Manager
	if manager == "" {
		manager = ServiceManagerSystemd
	}

	if source.Unavailable {
		return defaultRestartDelayReport(defaultDelay, ServiceRestartDelayEvidence{
			Kind:    ServiceManagerUnavailable,
			Manager: manager,
			Raw:     strings.TrimSpace(source.Output),
			Detail:  fmt.Sprintf("service manager unavailable; using default restart delay %s", defaultDelay),
		})
	}

	if manager != ServiceManagerSystemd {
		return defaultRestartDelayReport(defaultDelay, ServiceRestartDelayEvidence{
			Kind:    RestartDelayUnsupported,
			Manager: manager,
			Raw:     strings.TrimSpace(source.Output),
			Detail:  fmt.Sprintf("service manager %q does not expose systemd RestartUSec/RestartSec; using default restart delay %s", manager, defaultDelay),
		})
	}

	property, raw, ok := systemdRestartDelayRaw(source.Output)
	if !ok || strings.TrimSpace(raw) == "" {
		return defaultRestartDelayReport(defaultDelay, ServiceRestartDelayEvidence{
			Kind:     RestartDelayMissing,
			Manager:  manager,
			Property: property,
			Raw:      strings.TrimSpace(source.Output),
			Detail:   fmt.Sprintf("systemd RestartUSec/RestartSec evidence is missing; using default restart delay %s", defaultDelay),
		})
	}

	delay, bounded, status := parseSystemdRestartDelay(raw, property, maxDelay)
	switch status {
	case restartDelayParsed:
		report := ServiceRestartDelayReport{Delay: delay}
		if bounded {
			report.Evidence = append(report.Evidence, ServiceRestartDelayEvidence{
				Kind:     RestartDelayBounded,
				Manager:  manager,
				Property: property,
				Raw:      strings.TrimSpace(raw),
				Detail:   fmt.Sprintf("systemd %s exceeded max restart delay; bounded to %s", property, delay),
			})
		}
		return report
	case restartDelayInfiniteStatus:
		return defaultRestartDelayReport(defaultDelay, ServiceRestartDelayEvidence{
			Kind:     RestartDelayInfinite,
			Manager:  manager,
			Property: property,
			Raw:      strings.TrimSpace(raw),
			Detail:   fmt.Sprintf("systemd %s is infinity; using default restart delay %s", property, defaultDelay),
		})
	default:
		return defaultRestartDelayReport(defaultDelay, ServiceRestartDelayEvidence{
			Kind:     RestartDelayMalformed,
			Manager:  manager,
			Property: property,
			Raw:      strings.TrimSpace(raw),
			Detail:   fmt.Sprintf("could not parse systemd %s value %q; using default restart delay %s", property, strings.TrimSpace(raw), defaultDelay),
		})
	}
}

func boundedDefaultDelay(defaultDelay, maxDelay time.Duration) time.Duration {
	if defaultDelay < 0 {
		return 0
	}
	max := restartDelayMax(maxDelay)
	if defaultDelay > max {
		return max
	}
	return defaultDelay
}

func restartDelayMax(maxDelay time.Duration) time.Duration {
	if maxDelay <= 0 {
		return DefaultMaxServiceRestartDelay
	}
	return maxDelay
}

func defaultRestartDelayReport(delay time.Duration, evidence ServiceRestartDelayEvidence) ServiceRestartDelayReport {
	return ServiceRestartDelayReport{
		Delay: delay,
		Evidence: []ServiceRestartDelayEvidence{
			evidence,
			{
				Kind:     RestartDelayDefaulted,
				Manager:  evidence.Manager,
				Property: evidence.Property,
				Raw:      evidence.Raw,
				Detail:   fmt.Sprintf("using default restart delay %s", delay),
			},
		},
	}
}

func systemdRestartDelayRaw(output string) (property string, raw string, ok bool) {
	const defaultProperty = "RestartUSec"

	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return defaultProperty, "", false
	}

	hasAssignment := false
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		hasAssignment = true
		key = strings.TrimSpace(key)
		if key == "RestartUSec" || key == "RestartSec" {
			return key, strings.TrimSpace(value), true
		}
	}
	if hasAssignment {
		return defaultProperty, "", false
	}
	return defaultProperty, trimmed, true
}

type restartDelayParseStatus int

const (
	restartDelayParsed restartDelayParseStatus = iota
	restartDelayMalformedStatus
	restartDelayInfiniteStatus
)

func parseSystemdRestartDelay(raw, property string, maxDelay time.Duration) (time.Duration, bool, restartDelayParseStatus) {
	trimmed := strings.TrimSpace(raw)
	if strings.EqualFold(trimmed, "infinity") {
		return 0, false, restartDelayInfiniteStatus
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return 0, false, restartDelayMalformedStatus
	}

	totalNanos := 0.0
	maxNanos := float64(maxDelay)
	bounded := false
	for _, part := range parts {
		matches := systemdDurationTokenRE.FindStringSubmatch(part)
		if matches == nil {
			return 0, false, restartDelayMalformedStatus
		}

		value, err := strconv.ParseFloat(matches[1], 64)
		if err != nil || math.IsInf(value, 0) || math.IsNaN(value) || value < 0 {
			return 0, false, restartDelayMalformedStatus
		}
		unit, ok := systemdDurationUnit(matches[2], property)
		if !ok {
			return 0, false, restartDelayMalformedStatus
		}

		totalNanos += value * float64(unit)
		if math.IsInf(totalNanos, 0) || math.IsNaN(totalNanos) {
			return 0, false, restartDelayMalformedStatus
		}
		if totalNanos > maxNanos {
			totalNanos = maxNanos
			bounded = true
			break
		}
	}

	return time.Duration(totalNanos), bounded, restartDelayParsed
}

func systemdDurationUnit(suffix, property string) (time.Duration, bool) {
	switch strings.ToLower(suffix) {
	case "":
		if property == "RestartUSec" {
			return time.Microsecond, true
		}
		return time.Second, true
	case "ns", "nsec":
		return time.Nanosecond, true
	case "us", "usec":
		return time.Microsecond, true
	case "ms", "msec":
		return time.Millisecond, true
	case "s", "sec", "second", "seconds":
		return time.Second, true
	case "m", "min", "minute", "minutes":
		return time.Minute, true
	case "h", "hr", "hour", "hours":
		return time.Hour, true
	default:
		return 0, false
	}
}
