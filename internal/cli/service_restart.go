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

const (
	DefaultServiceRestartPollTimeout  = 10 * time.Second
	DefaultServiceRestartPollInterval = 500 * time.Millisecond
)

type ServiceActiveStatus string

const (
	ServiceActiveStatusActive     ServiceActiveStatus = "active"
	ServiceActiveStatusInactive   ServiceActiveStatus = "inactive"
	ServiceActiveStatusActivating ServiceActiveStatus = "activating"
	ServiceActiveStatusFailed     ServiceActiveStatus = "failed"
	ServiceActiveStatusUnknown    ServiceActiveStatus = "unknown"
)

type ServiceRestartPollOutcome string

const (
	ServiceRestartPollRestarted           ServiceRestartPollOutcome = "restarted"
	ServiceRestartPollTimeout             ServiceRestartPollOutcome = "restart_timeout"
	ServiceRestartPollManagerUnavailable  ServiceRestartPollOutcome = "service_manager_unavailable"
	ServiceRestartPollCrashedAfterRestart ServiceRestartPollOutcome = "crashed_after_restart"
)

type ServiceRestartPollEvidenceKind string

const (
	ServiceRestartPollActiveEvidence              ServiceRestartPollEvidenceKind = "service_active"
	ServiceRestartPollCooldownEvidence            ServiceRestartPollEvidenceKind = "restart_delay_cooldown"
	ServiceRestartPollTimeoutEvidence             ServiceRestartPollEvidenceKind = "service_restart_timeout"
	ServiceRestartPollManagerUnavailableEvidence  ServiceRestartPollEvidenceKind = "service_manager_unavailable"
	ServiceRestartPollCrashedAfterRestartEvidence ServiceRestartPollEvidenceKind = "crashed_after_restart"
	ServiceRestartPollRetryEvidence               ServiceRestartPollEvidenceKind = "service_restart_retry"
)

type ServiceActiveStatusCheck struct {
	Status      ServiceActiveStatus
	Unavailable bool
	Raw         string
	Detail      string
}

type ServiceActiveStatusRunner interface {
	ServiceActiveStatus(service string) (ServiceActiveStatusCheck, error)
}

type ServiceRestartPollClock interface {
	Now() time.Time
	Sleep(time.Duration)
}

type ServiceRestartPollOptions struct {
	Service      string
	Runner       ServiceActiveStatusRunner
	Clock        ServiceRestartPollClock
	RestartDelay ServiceRestartDelayReport
	BaseTimeout  time.Duration
	PollInterval time.Duration
}

type ServiceRestartPollReport struct {
	Outcome      ServiceRestartPollOutcome
	Service      string
	Timeout      time.Duration
	PollInterval time.Duration
	Attempts     int
	StartedAt    time.Time
	FinishedAt   time.Time
	RestartDelay ServiceRestartDelayReport
	Evidence     []ServiceRestartPollEvidence
}

type ServiceRestartPollEvidence struct {
	Kind   ServiceRestartPollEvidenceKind
	Status ServiceActiveStatus
	Raw    string
	Detail string
}

var systemdDurationTokenRE = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)([A-Za-z]*)$`)

func PollServiceRestartActive(options ServiceRestartPollOptions) ServiceRestartPollReport {
	clock := options.Clock
	if clock == nil {
		clock = realServiceRestartPollClock{}
	}

	service := strings.TrimSpace(options.Service)
	if service == "" {
		service = "service"
	}

	baseTimeout := options.BaseTimeout
	if baseTimeout <= 0 {
		baseTimeout = DefaultServiceRestartPollTimeout
	}
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = DefaultServiceRestartPollInterval
	}
	timeout := serviceRestartPollTimeout(baseTimeout, options.RestartDelay)

	report := ServiceRestartPollReport{
		Service:      service,
		Timeout:      timeout,
		PollInterval: pollInterval,
		StartedAt:    clock.Now(),
		RestartDelay: options.RestartDelay,
	}
	if options.RestartDelay.Delay > 0 {
		report.Evidence = append(report.Evidence, ServiceRestartPollEvidence{
			Kind: ServiceRestartPollCooldownEvidence,
			Detail: fmt.Sprintf(
				"polling service active status for %s to cover restart delay %s plus transition slack %s",
				timeout, options.RestartDelay.Delay, baseTimeout,
			),
		})
	}

	if options.Runner == nil {
		return finishServiceRestartPoll(report, clock, ServiceRestartPollManagerUnavailable, ServiceRestartPollEvidence{
			Kind:   ServiceRestartPollManagerUnavailableEvidence,
			Detail: "service manager unavailable; no active-status runner was provided",
		})
	}

	deadline := report.StartedAt.Add(timeout)
	for {
		check, err := options.Runner.ServiceActiveStatus(service)
		report.Attempts++
		check.Status = normalizeServiceActiveStatus(check.Status)

		if err != nil {
			return finishServiceRestartPoll(report, clock, ServiceRestartPollManagerUnavailable, ServiceRestartPollEvidence{
				Kind:   ServiceRestartPollManagerUnavailableEvidence,
				Status: check.Status,
				Raw:    check.Raw,
				Detail: fmt.Sprintf("service manager unavailable while checking %s: %v", service, err),
			})
		}
		if check.Unavailable {
			detail := check.Detail
			if detail == "" {
				detail = fmt.Sprintf("service manager unavailable while checking %s active status", service)
			}
			return finishServiceRestartPoll(report, clock, ServiceRestartPollManagerUnavailable, ServiceRestartPollEvidence{
				Kind:   ServiceRestartPollManagerUnavailableEvidence,
				Status: check.Status,
				Raw:    check.Raw,
				Detail: detail,
			})
		}

		switch check.Status {
		case ServiceActiveStatusActive:
			return finishServiceRestartPoll(report, clock, ServiceRestartPollRestarted, ServiceRestartPollEvidence{
				Kind:   ServiceRestartPollActiveEvidence,
				Status: check.Status,
				Raw:    check.Raw,
				Detail: fmt.Sprintf("service %s reported active after %d poll attempts over %s", service, report.Attempts, clock.Now().Sub(report.StartedAt)),
			})
		case ServiceActiveStatusFailed:
			return finishServiceRestartPoll(report, clock, ServiceRestartPollCrashedAfterRestart,
				ServiceRestartPollEvidence{
					Kind:   ServiceRestartPollCrashedAfterRestartEvidence,
					Status: check.Status,
					Raw:    check.Raw,
					Detail: fmt.Sprintf("service %s reported failed after restart; not reporting it as restarted", service),
				},
				ServiceRestartPollEvidence{
					Kind:   ServiceRestartPollRetryEvidence,
					Status: check.Status,
					Raw:    check.Raw,
					Detail: fmt.Sprintf("retry service restart for %s or inspect service logs before declaring recovery", service),
				},
			)
		}

		now := clock.Now()
		if !now.Before(deadline) {
			return finishServiceRestartPoll(report, clock, ServiceRestartPollTimeout, ServiceRestartPollEvidence{
				Kind:   ServiceRestartPollTimeoutEvidence,
				Status: check.Status,
				Raw:    check.Raw,
				Detail: fmt.Sprintf("timed out after %s waiting for %s to report active; last status was %s", timeout, service, check.Status),
			})
		}

		sleep := pollInterval
		if remaining := deadline.Sub(now); remaining < sleep {
			sleep = remaining
		}
		if sleep <= 0 {
			return finishServiceRestartPoll(report, clock, ServiceRestartPollTimeout, ServiceRestartPollEvidence{
				Kind:   ServiceRestartPollTimeoutEvidence,
				Status: check.Status,
				Raw:    check.Raw,
				Detail: fmt.Sprintf("timed out after %s waiting for %s to report active; last status was %s", timeout, service, check.Status),
			})
		}
		clock.Sleep(sleep)
	}
}

func serviceRestartPollTimeout(baseTimeout time.Duration, restartDelay ServiceRestartDelayReport) time.Duration {
	timeout := baseTimeout
	if restartDelay.Delay > 0 {
		withRestartDelay := restartDelay.Delay + baseTimeout
		if withRestartDelay > timeout {
			timeout = withRestartDelay
		}
	}
	return timeout
}

func normalizeServiceActiveStatus(status ServiceActiveStatus) ServiceActiveStatus {
	switch status {
	case ServiceActiveStatusActive, ServiceActiveStatusInactive, ServiceActiveStatusActivating, ServiceActiveStatusFailed:
		return status
	default:
		return ServiceActiveStatusUnknown
	}
}

func finishServiceRestartPoll(report ServiceRestartPollReport, clock ServiceRestartPollClock, outcome ServiceRestartPollOutcome, evidence ...ServiceRestartPollEvidence) ServiceRestartPollReport {
	report.Outcome = outcome
	report.FinishedAt = clock.Now()
	report.Evidence = append(report.Evidence, evidence...)
	return report
}

type realServiceRestartPollClock struct{}

func (realServiceRestartPollClock) Now() time.Time {
	return time.Now()
}

func (realServiceRestartPollClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

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
