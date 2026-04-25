package tools

import (
	"testing"
	"time"
)

type fakeProcessNotificationClock struct {
	now time.Time
}

func newFakeProcessNotificationClock() *fakeProcessNotificationClock {
	return &fakeProcessNotificationClock{now: time.Date(2026, 4, 25, 15, 0, 0, 0, time.UTC)}
}

func (c *fakeProcessNotificationClock) Now() time.Time {
	return c.now
}

func (c *fakeProcessNotificationClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func TestProcessNotificationNormalizeMutualExclusion(t *testing.T) {
	req := ProcessNotificationRequest{
		Background:       true,
		NotifyOnComplete: true,
		WatchPatterns:    []string{"ERROR", "DONE"},
	}

	normalized := NormalizeProcessNotificationRequest(req)

	if !normalized.NotifyOnComplete {
		t.Fatal("NotifyOnComplete = false, want true")
	}
	if len(normalized.WatchPatterns) != 0 {
		t.Fatalf("WatchPatterns = %v, want dropped", normalized.WatchPatterns)
	}
	if len(normalized.Evidence) != 1 {
		t.Fatalf("len(Evidence) = %d, want 1", len(normalized.Evidence))
	}
	if normalized.Evidence[0].Status != "watch_patterns_disabled" {
		t.Fatalf("Evidence[0].Status = %q, want watch_patterns_disabled", normalized.Evidence[0].Status)
	}
	if normalized.Evidence[0].Message == "" {
		t.Fatal("Evidence[0].Message is empty, want operator-readable conflict note")
	}

	foreground := NormalizeProcessNotificationRequest(ProcessNotificationRequest{
		Background:       false,
		NotifyOnComplete: true,
		WatchPatterns:    []string{"ERROR"},
	})
	if len(foreground.WatchPatterns) != 1 || foreground.WatchPatterns[0] != "ERROR" {
		t.Fatalf("foreground WatchPatterns = %v, want preserved", foreground.WatchPatterns)
	}
	if len(foreground.Evidence) != 0 {
		t.Fatalf("foreground Evidence = %v, want none", foreground.Evidence)
	}

	watchOnly := NormalizeProcessNotificationRequest(ProcessNotificationRequest{
		Background:    true,
		WatchPatterns: []string{"READY"},
	})
	if len(watchOnly.WatchPatterns) != 1 || watchOnly.WatchPatterns[0] != "READY" {
		t.Fatalf("watch-only WatchPatterns = %v, want preserved", watchOnly.WatchPatterns)
	}
	if watchOnly.NotifyOnComplete {
		t.Fatal("watch-only NotifyOnComplete = true, want false")
	}
}

func TestProcessNotificationWatchPatternThrottle(t *testing.T) {
	clock := newFakeProcessNotificationClock()
	policy := NewProcessNotificationPolicy(clock.Now)
	session := ProcessNotificationSession{
		ID:            "proc_one",
		Command:       "tail -f app.log",
		WatchPatterns: []string{"ERROR"},
	}

	events := policy.CheckWatchPatterns(&session, "INFO ok\nERROR first\n")
	if len(events) != 1 {
		t.Fatalf("first events len = %d, want 1", len(events))
	}
	if events[0].Type != "watch_match" {
		t.Fatalf("first event Type = %q, want watch_match", events[0].Type)
	}
	if events[0].Pattern != "ERROR" {
		t.Fatalf("first event Pattern = %q, want ERROR", events[0].Pattern)
	}

	events = policy.CheckWatchPatterns(&session, "ERROR second\nERROR third\n")
	if len(events) != 0 {
		t.Fatalf("cooldown events = %#v, want none", events)
	}
	if session.watchConsecutiveStrikes != 1 {
		t.Fatalf("watchConsecutiveStrikes = %d, want one strike for the window", session.watchConsecutiveStrikes)
	}
	if session.watchSuppressed != 2 {
		t.Fatalf("watchSuppressed = %d, want two suppressed matched lines", session.watchSuppressed)
	}

	clock.Advance(ProcessWatchMinInterval)
	events = policy.CheckWatchPatterns(&session, "ERROR fourth\n")
	if len(events) != 1 {
		t.Fatalf("post-cooldown events len = %d, want 1", len(events))
	}
	if events[0].Suppressed != 2 {
		t.Fatalf("post-cooldown Suppressed = %d, want 2", events[0].Suppressed)
	}
}

func TestProcessNotificationThreeStrikePromotion(t *testing.T) {
	clock := newFakeProcessNotificationClock()
	policy := NewProcessNotificationPolicy(clock.Now)
	session := ProcessNotificationSession{
		ID:            "proc_strikes",
		Command:       "tail -f noisy.log",
		WatchPatterns: []string{"E"},
	}

	var disabled []ProcessNotificationEvent
	for strike := 0; strike < ProcessWatchStrikeLimit; strike++ {
		events := policy.CheckWatchPatterns(&session, "E emitted\n")
		if strike < ProcessWatchStrikeLimit-1 && len(events) != 1 {
			t.Fatalf("strike %d emit events len = %d, want 1", strike, len(events))
		}
		disabled = policy.CheckWatchPatterns(&session, "E dropped\n")
		if strike < ProcessWatchStrikeLimit-1 && len(disabled) != 0 {
			t.Fatalf("strike %d drop events = %#v, want none before promotion", strike, disabled)
		}
		clock.Advance(ProcessWatchMinInterval)
	}

	if !session.watchDisabled {
		t.Fatal("watchDisabled = false, want true")
	}
	if len(session.WatchPatterns) != 0 {
		t.Fatalf("WatchPatterns = %v, want disabled", session.WatchPatterns)
	}
	if !session.NotifyOnComplete {
		t.Fatal("NotifyOnComplete = false, want promotion to completion notification")
	}
	if len(disabled) != 1 {
		t.Fatalf("disabled events len = %d, want one summary", len(disabled))
	}
	if disabled[0].Type != "watch_disabled" {
		t.Fatalf("disabled Type = %q, want watch_disabled", disabled[0].Type)
	}
	if disabled[0].Message == "" {
		t.Fatal("disabled Message is empty, want operator-readable summary")
	}
	events := policy.CheckWatchPatterns(&session, "E still noisy\n")
	if len(events) != 0 {
		t.Fatalf("post-disable events = %#v, want no duplicate summaries", events)
	}
}

func TestProcessNotificationSuppressAfterExit(t *testing.T) {
	clock := newFakeProcessNotificationClock()
	policy := NewProcessNotificationPolicy(clock.Now)
	session := ProcessNotificationSession{
		ID:            "proc_done",
		Command:       "make test",
		WatchPatterns: []string{"PASS"},
		Exited:        true,
	}

	events := policy.CheckWatchPatterns(&session, "PASS late buffered output\n")
	if len(events) != 0 {
		t.Fatalf("events after exit = %#v, want none", events)
	}
	if session.watchHits != 0 {
		t.Fatalf("watchHits = %d, want 0", session.watchHits)
	}
}

func TestProcessNotificationGlobalOverflowTripAndRelease(t *testing.T) {
	clock := newFakeProcessNotificationClock()
	policy := NewProcessNotificationPolicy(clock.Now)

	watchMatches := 0
	overflowTripped := 0
	for i := 0; i < ProcessWatchGlobalMaxPerWindow+1; i++ {
		session := ProcessNotificationSession{
			ID:            "proc_global",
			Command:       "tail -f shared.log",
			WatchPatterns: []string{"E"},
		}
		events := policy.CheckWatchPatterns(&session, "E hit\n")
		for _, event := range events {
			switch event.Type {
			case "watch_match":
				watchMatches++
			case "watch_overflow_tripped":
				overflowTripped++
			}
		}
	}
	if watchMatches != ProcessWatchGlobalMaxPerWindow {
		t.Fatalf("watchMatches = %d, want %d", watchMatches, ProcessWatchGlobalMaxPerWindow)
	}
	if overflowTripped != 1 {
		t.Fatalf("overflowTripped = %d, want 1", overflowTripped)
	}

	suppressed := ProcessNotificationSession{
		ID:            "proc_suppressed",
		Command:       "tail -f shared.log",
		WatchPatterns: []string{"E"},
	}
	events := policy.CheckWatchPatterns(&suppressed, "E suppressed\n")
	if len(events) != 0 {
		t.Fatalf("events while global breaker tripped = %#v, want none", events)
	}

	clock.Advance(ProcessWatchGlobalCooldown)
	released := ProcessNotificationSession{
		ID:            "proc_released",
		Command:       "tail -f shared.log",
		WatchPatterns: []string{"E"},
	}
	events = policy.CheckWatchPatterns(&released, "E released\n")
	gotRelease := false
	gotAdmit := false
	for _, event := range events {
		switch event.Type {
		case "watch_overflow_released":
			gotRelease = true
			if event.Suppressed < 1 {
				t.Fatalf("release Suppressed = %d, want at least 1", event.Suppressed)
			}
		case "watch_match":
			gotAdmit = true
		}
	}
	if !gotRelease {
		t.Fatalf("events after cooldown = %#v, missing release summary", events)
	}
	if !gotAdmit {
		t.Fatalf("events after cooldown = %#v, missing admitted watch_match", events)
	}
}
