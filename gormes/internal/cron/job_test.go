package cron

import (
	"testing"
)

func TestValidateSchedule_AcceptsStandardCron(t *testing.T) {
	for _, expr := range []string{
		"0 8 * * *",
		"*/5 * * * *",
		"0 0 1 * *",
		"@daily",
		"@every 30m",
	} {
		if err := ValidateSchedule(expr); err != nil {
			t.Errorf("ValidateSchedule(%q) = %v, want nil", expr, err)
		}
	}
}

func TestValidateSchedule_RejectsGarbage(t *testing.T) {
	for _, expr := range []string{
		"",
		"not a cron expression",
		"* * * *",    // too few fields
		"99 * * * *", // minute out of range
		"@unknown",
	} {
		if err := ValidateSchedule(expr); err == nil {
			t.Errorf("ValidateSchedule(%q) = nil, want error", expr)
		}
	}
}

func TestJob_NewGeneratesID(t *testing.T) {
	j := NewJob("morning", "0 8 * * *", "status prompt")
	if j.ID == "" {
		t.Error("NewJob did not populate ID")
	}
	if j.Name != "morning" || j.Schedule != "0 8 * * *" || j.Prompt != "status prompt" {
		t.Errorf("NewJob fields = %+v, want name/sched/prompt", j)
	}
	if j.CreatedAt == 0 {
		t.Error("NewJob did not set CreatedAt")
	}
	if j.Paused {
		t.Error("NewJob must default to Paused=false (active)")
	}
}
