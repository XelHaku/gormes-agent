package cron

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseCronSchedule_OneShotDuration(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		input   string
		offset  time.Duration
		display string
	}{
		{input: "30m", offset: 30 * time.Minute, display: "30m"},
		{input: "2h", offset: 2 * time.Hour, display: "2h"},
		{input: "1d", offset: 24 * time.Hour, display: "1d"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseCronSchedule(tt.input, now)
			if err != nil {
				t.Fatalf("ParseCronSchedule(%q) error = %v, want nil", tt.input, err)
			}
			if got.Kind != ScheduleKindOnce {
				t.Fatalf("Kind = %q, want %q", got.Kind, ScheduleKindOnce)
			}
			if !got.RunAt.Equal(now.Add(tt.offset)) {
				t.Fatalf("RunAt = %s, want %s", got.RunAt, now.Add(tt.offset))
			}
			if got.Display != tt.display {
				t.Fatalf("Display = %q, want %q", got.Display, tt.display)
			}
		})
	}
}

func TestParseCronSchedule_RecurringInterval(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		input   string
		minutes int
	}{
		{input: "every 30m", minutes: 30},
		{input: "every 2h", minutes: 120},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseCronSchedule(tt.input, now)
			if err != nil {
				t.Fatalf("ParseCronSchedule(%q) error = %v, want nil", tt.input, err)
			}
			if got.Kind != ScheduleKindInterval {
				t.Fatalf("Kind = %q, want %q", got.Kind, ScheduleKindInterval)
			}
			if got.Minutes != tt.minutes {
				t.Fatalf("Minutes = %d, want %d", got.Minutes, tt.minutes)
			}
		})
	}
}

func TestParseCronSchedule_CronExpression(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	got, err := ParseCronSchedule("0 9 * * *", now)
	if err != nil {
		t.Fatalf("ParseCronSchedule(valid cron) error = %v, want nil", err)
	}
	if got.Kind != ScheduleKindCron {
		t.Fatalf("Kind = %q, want %q", got.Kind, ScheduleKindCron)
	}
	if got.Expr != "0 9 * * *" {
		t.Fatalf("Expr = %q, want %q", got.Expr, "0 9 * * *")
	}

	for _, input := range []string{"* * * *", "99 * * * *"} {
		t.Run(input, func(t *testing.T) {
			_, err := ParseCronSchedule(input, now)
			if err == nil {
				t.Fatalf("ParseCronSchedule(%q) error = nil, want typed invalid schedule error", input)
			}
			var parseErr *ScheduleParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("error type = %T, want *ScheduleParseError", err)
			}
			if !strings.Contains(err.Error(), "invalid schedule") {
				t.Fatalf("error = %q, want it to contain invalid schedule", err.Error())
			}
		})
	}
}

func TestParseCronSchedule_ISOTimestamp(t *testing.T) {
	loc := time.FixedZone("anchor", -6*60*60)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, loc)

	aware, err := ParseCronSchedule("2026-04-26T15:30:00-05:00", now)
	if err != nil {
		t.Fatalf("ParseCronSchedule(timezone-aware ISO) error = %v, want nil", err)
	}
	wantAware := time.Date(2026, 4, 26, 15, 30, 0, 0, time.FixedZone("", -5*60*60))
	if aware.Kind != ScheduleKindOnce {
		t.Fatalf("aware Kind = %q, want %q", aware.Kind, ScheduleKindOnce)
	}
	if !aware.RunAt.Equal(wantAware) {
		t.Fatalf("aware RunAt = %s, want %s", aware.RunAt, wantAware)
	}

	naive, err := ParseCronSchedule("2026-04-26T09:30:00", now)
	if err != nil {
		t.Fatalf("ParseCronSchedule(naive ISO) error = %v, want nil", err)
	}
	wantNaive := time.Date(2026, 4, 26, 9, 30, 0, 0, loc)
	if !naive.RunAt.Equal(wantNaive) {
		t.Fatalf("naive RunAt = %s, want %s", naive.RunAt, wantNaive)
	}
	if naive.RunAt.Location() != loc {
		t.Fatalf("naive location = %v, want injected location %v", naive.RunAt.Location(), loc)
	}
}

func TestCronNextRunDecision_OneShotGraceAllowsLateTick(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	late119 := ParsedSchedule{
		Kind:    ScheduleKindOnce,
		RunAt:   now.Add(-119 * time.Second),
		Display: "30m",
		Repeat:  1,
	}
	got := CronNextRunDecision(late119, 0, 0, now)
	if !got.Runnable || !got.ShouldRun || !got.RecoverableOneShot {
		t.Fatalf("119s-late decision = %+v, want runnable recoverable one-shot", got)
	}

	late121 := late119
	late121.RunAt = now.Add(-121 * time.Second)
	got = CronNextRunDecision(late121, 0, 0, now)
	if got.Runnable || got.ShouldRun || got.RecoverableOneShot {
		t.Fatalf("121s-late decision = %+v, want excluded after grace", got)
	}
	if got.Unavailable == nil || got.Unavailable.Code != "oneshot_grace_expired" {
		t.Fatalf("121s-late unavailable = %+v, want oneshot_grace_expired", got.Unavailable)
	}
}

func TestCronNextRunDecision_FiniteRepeatExhaustion(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	parsed := ParsedSchedule{
		Kind:    ScheduleKindInterval,
		Minutes: 30,
		Display: "every 30m",
		Repeat:  1,
	}

	exhausted := CronNextRunDecision(parsed, now.Unix(), 1, now)
	if !exhausted.Exhausted || exhausted.Runnable || exhausted.ShouldRun {
		t.Fatalf("repeat=1 completed=1 decision = %+v, want exhausted and not runnable", exhausted)
	}
	if exhausted.Unavailable == nil || exhausted.Unavailable.Code != "repeat_exhausted" {
		t.Fatalf("exhausted unavailable = %+v, want repeat_exhausted", exhausted.Unavailable)
	}

	parsed.Repeat = 3
	remaining := CronNextRunDecision(parsed, now.Unix(), 2, now)
	if remaining.Exhausted || !remaining.Runnable || remaining.Unavailable != nil {
		t.Fatalf("repeat=3 completed=2 decision = %+v, want runnable and not exhausted", remaining)
	}
}

func TestCronNextRunDecision_RecurringFastForward(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	parsed, err := ParseCronSchedule("every 30m", now)
	if err != nil {
		t.Fatalf("ParseCronSchedule(every 30m) error = %v, want nil", err)
	}

	staleLastRun := now.Add(-3 * time.Hour).Unix()
	got := CronNextRunDecision(parsed, staleLastRun, 0, now)
	if !got.Runnable || got.ShouldRun || !got.FastForwarded {
		t.Fatalf("stale recurring decision = %+v, want runnable fast-forward without immediate run", got)
	}
	want := now.Add(30 * time.Minute)
	if !got.NextRun.Equal(want) {
		t.Fatalf("NextRun = %s, want %s", got.NextRun, want)
	}
	if !got.NextRun.After(now) {
		t.Fatalf("NextRun = %s, want after now %s", got.NextRun, now)
	}
}
