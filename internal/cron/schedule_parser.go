package cron

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	rc "github.com/robfig/cron/v3"
)

const oneShotGrace = 120 * time.Second

var (
	// ErrInvalidSchedule marks operator schedule strings that cannot be parsed.
	ErrInvalidSchedule = errors.New("invalid schedule")

	durationPattern = regexp.MustCompile(`^(\d+)\s*(m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days)$`)
	isoDatePattern  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(?:[T ].*)?$`)
	cronParser      = rc.NewParser(rc.Minute | rc.Hour | rc.Dom | rc.Month | rc.Dow)
)

type ScheduleKind string

const (
	ScheduleKindOnce     ScheduleKind = "once"
	ScheduleKindInterval ScheduleKind = "interval"
	ScheduleKindCron     ScheduleKind = "cron"
)

// ParsedSchedule is the pure read model for operator cron schedule strings.
type ParsedSchedule struct {
	Kind    ScheduleKind
	Display string
	RunAt   time.Time
	Minutes int
	Expr    string
	// Repeat is a finite repeat limit. Zero means unbounded.
	Repeat int
}

// CronUnavailableEvidence is stable degraded-mode evidence for schedule
// decisions that should skip one job without stopping the scheduler loop.
type CronUnavailableEvidence struct {
	Code    string
	Message string
}

// ScheduleParseError is a typed invalid-schedule error with unavailable
// evidence attached for future cron tool/API envelopes.
type ScheduleParseError struct {
	Input    string
	Evidence CronUnavailableEvidence
	Err      error
}

func (e *ScheduleParseError) Error() string {
	if e == nil {
		return ""
	}
	message := e.Evidence.Message
	if message == "" && e.Err != nil {
		message = e.Err.Error()
	}
	if message == "" {
		message = ErrInvalidSchedule.Error()
	}
	return fmt.Sprintf("cron: invalid schedule %q: %s", e.Input, message)
}

func (e *ScheduleParseError) Unwrap() error {
	return e.Err
}

func (e *ScheduleParseError) Is(target error) bool {
	return target == ErrInvalidSchedule
}

// CronRunDecision reports what a scheduler read path should do for one job.
type CronRunDecision struct {
	Runnable           bool
	ShouldRun          bool
	NextRun            time.Time
	RecoverableOneShot bool
	FastForwarded      bool
	Exhausted          bool
	Unavailable        *CronUnavailableEvidence
}

// ParseCronSchedule parses Hermes-compatible operator schedule strings without
// touching stores, clocks, goroutines, or public cron tool handlers.
func ParseCronSchedule(input string, now time.Time) (ParsedSchedule, error) {
	display := strings.TrimSpace(input)
	if display == "" {
		return ParsedSchedule{}, invalidSchedule(input, "schedule is empty")
	}

	lower := strings.ToLower(display)
	if strings.HasPrefix(lower, "every ") {
		minutes, err := parseDurationMinutes(strings.TrimSpace(display[len("every "):]))
		if err != nil {
			return ParsedSchedule{}, invalidSchedule(input, err.Error())
		}
		return ParsedSchedule{
			Kind:    ScheduleKindInterval,
			Display: display,
			Minutes: minutes,
		}, nil
	}

	if runAt, ok, err := parseISOTimestamp(display, now.Location()); ok || err != nil {
		if err != nil {
			return ParsedSchedule{}, invalidSchedule(input, err.Error())
		}
		return ParsedSchedule{
			Kind:    ScheduleKindOnce,
			Display: display,
			RunAt:   runAt,
			Repeat:  1,
		}, nil
	}

	if minutes, err := parseDurationMinutes(display); err == nil {
		return ParsedSchedule{
			Kind:    ScheduleKindOnce,
			Display: display,
			RunAt:   now.Add(time.Duration(minutes) * time.Minute),
			Repeat:  1,
		}, nil
	}

	if _, err := parseCronExpression(display); err == nil {
		return ParsedSchedule{
			Kind:    ScheduleKindCron,
			Display: display,
			Expr:    display,
		}, nil
	} else if len(strings.Fields(display)) == 5 {
		return ParsedSchedule{}, invalidSchedule(input, err.Error())
	}

	return ParsedSchedule{}, invalidSchedule(input, "use a duration, recurring interval, 5-field cron expression, or ISO timestamp")
}

// CronNextRunDecision evaluates repeat state and next-run recovery without
// mutating cron stores.
func CronNextRunDecision(parsed ParsedSchedule, lastRunUnix int64, repeatCompleted int, now time.Time) CronRunDecision {
	if repeatCompleted < 0 {
		repeatCompleted = 0
	}
	if parsed.Repeat > 0 && repeatCompleted >= parsed.Repeat {
		return CronRunDecision{
			Exhausted: true,
			Unavailable: unavailableEvidence(
				"repeat_exhausted",
				fmt.Sprintf("repeat limit %d exhausted after %d completed runs", parsed.Repeat, repeatCompleted),
			),
		}
	}

	switch parsed.Kind {
	case ScheduleKindOnce:
		return oneShotDecision(parsed, lastRunUnix, now)
	case ScheduleKindInterval:
		return intervalDecision(parsed, lastRunUnix, now)
	case ScheduleKindCron:
		return cronDecision(parsed, lastRunUnix, now)
	default:
		return CronRunDecision{
			Unavailable: unavailableEvidence("invalid_schedule", "schedule kind is invalid"),
		}
	}
}

func parseDurationMinutes(input string) (int, error) {
	match := durationPattern.FindStringSubmatch(strings.ToLower(strings.TrimSpace(input)))
	if match == nil {
		return 0, fmt.Errorf("invalid duration %q", input)
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid duration %q", input)
	}
	switch match[2][0] {
	case 'm':
		return value, nil
	case 'h':
		return value * 60, nil
	case 'd':
		return value * 1440, nil
	default:
		return 0, fmt.Errorf("invalid duration %q", input)
	}
}

func parseISOTimestamp(input string, loc *time.Location) (time.Time, bool, error) {
	if !isoDatePattern.MatchString(input) {
		return time.Time{}, false, nil
	}
	if loc == nil {
		loc = time.UTC
	}

	awareLayouts := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04Z07:00",
	}
	for _, layout := range awareLayouts {
		if ts, err := time.Parse(layout, input); err == nil {
			return ts, true, nil
		}
	}

	naiveLayouts := []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, layout := range naiveLayouts {
		if ts, err := time.ParseInLocation(layout, input, loc); err == nil {
			return ts, true, nil
		}
	}

	return time.Time{}, true, fmt.Errorf("invalid ISO timestamp %q", input)
}

func parseCronExpression(input string) (rc.Schedule, error) {
	if len(strings.Fields(input)) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields")
	}
	return cronParser.Parse(input)
}

func oneShotDecision(parsed ParsedSchedule, lastRunUnix int64, now time.Time) CronRunDecision {
	if parsed.RunAt.IsZero() {
		return CronRunDecision{Unavailable: unavailableEvidence("invalid_schedule", "one-shot run time is missing")}
	}
	if lastRunUnix > 0 {
		return CronRunDecision{Unavailable: unavailableEvidence("oneshot_completed", "one-shot already has a recorded run")}
	}

	decision := CronRunDecision{
		Runnable: true,
		NextRun:  parsed.RunAt,
	}
	if parsed.RunAt.After(now) {
		return decision
	}

	if now.Sub(parsed.RunAt) <= oneShotGrace {
		decision.ShouldRun = true
		decision.RecoverableOneShot = true
		return decision
	}

	return CronRunDecision{
		Unavailable: unavailableEvidence("oneshot_grace_expired", "one-shot run time is outside the 120s grace window"),
	}
}

func intervalDecision(parsed ParsedSchedule, lastRunUnix int64, now time.Time) CronRunDecision {
	if parsed.Minutes <= 0 {
		return CronRunDecision{Unavailable: unavailableEvidence("invalid_schedule", "interval minutes must be positive")}
	}
	period := time.Duration(parsed.Minutes) * time.Minute
	if lastRunUnix <= 0 {
		return CronRunDecision{
			Runnable: true,
			NextRun:  now.Add(period),
		}
	}
	nextRun := time.Unix(lastRunUnix, 0).In(now.Location()).Add(period)
	return recurringDecision(nextRun, now.Add(period), period, now)
}

func cronDecision(parsed ParsedSchedule, lastRunUnix int64, now time.Time) CronRunDecision {
	schedule, err := parseCronExpression(parsed.Expr)
	if err != nil {
		return CronRunDecision{Unavailable: unavailableEvidence("invalid_schedule", err.Error())}
	}

	base := now
	if lastRunUnix > 0 {
		base = time.Unix(lastRunUnix, 0).In(now.Location())
	}
	nextRun := schedule.Next(base)
	fastForwardTo := schedule.Next(now)
	period := cronPeriod(schedule, now)
	return recurringDecision(nextRun, fastForwardTo, period, now)
}

func recurringDecision(nextRun, fastForwardTo time.Time, period time.Duration, now time.Time) CronRunDecision {
	decision := CronRunDecision{
		Runnable: true,
		NextRun:  nextRun,
	}
	if nextRun.After(now) {
		return decision
	}
	if now.Sub(nextRun) > recurringGrace(period) {
		decision.FastForwarded = true
		decision.NextRun = fastForwardTo
		return decision
	}
	decision.ShouldRun = true
	return decision
}

func cronPeriod(schedule rc.Schedule, now time.Time) time.Duration {
	first := schedule.Next(now)
	second := schedule.Next(first)
	return second.Sub(first)
}

func recurringGrace(period time.Duration) time.Duration {
	const (
		minGrace = 120 * time.Second
		maxGrace = 2 * time.Hour
	)
	if period <= 0 {
		return minGrace
	}
	grace := period / 2
	if grace < minGrace {
		return minGrace
	}
	if grace > maxGrace {
		return maxGrace
	}
	return grace
}

func invalidSchedule(input, message string) error {
	return &ScheduleParseError{
		Input: input,
		Evidence: CronUnavailableEvidence{
			Code:    "invalid_schedule",
			Message: message,
		},
		Err: ErrInvalidSchedule,
	}
}

func unavailableEvidence(code, message string) *CronUnavailableEvidence {
	return &CronUnavailableEvidence{
		Code:    code,
		Message: message,
	}
}
