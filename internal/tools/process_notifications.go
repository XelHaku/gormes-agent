package tools

import (
	"strings"
	"time"
)

const (
	// ProcessWatchMinInterval is the per-session minimum spacing for emitted
	// watch-pattern notifications.
	ProcessWatchMinInterval = 15 * time.Second
	// ProcessWatchStrikeLimit is the number of consecutive cooldown windows with
	// suppressed matches before watch patterns are disabled.
	ProcessWatchStrikeLimit = 3
	// ProcessWatchGlobalMaxPerWindow is the maximum number of watch matches
	// emitted across all sessions in ProcessWatchGlobalWindow.
	ProcessWatchGlobalMaxPerWindow = 15
	// ProcessWatchGlobalWindow is the global overflow accounting window.
	ProcessWatchGlobalWindow = 10 * time.Second
	// ProcessWatchGlobalCooldown is the global overflow suppression duration.
	ProcessWatchGlobalCooldown = 30 * time.Second
)

// ProcessNotificationRequest is the process/terminal notification portion of a
// future spawn request.
type ProcessNotificationRequest struct {
	Background       bool
	NotifyOnComplete bool
	WatchPatterns    []string
}

// ProcessNotificationEvidence is an operator-readable policy decision.
type ProcessNotificationEvidence struct {
	Status  string
	Message string
}

// ProcessNotificationPlan is the normalized notification configuration that a
// future process runner can apply before spawning a live process.
type ProcessNotificationPlan struct {
	NotifyOnComplete bool
	WatchPatterns    []string
	Evidence         []ProcessNotificationEvidence
}

// NormalizeProcessNotificationRequest resolves invalid notification flag
// combinations without starting a process.
func NormalizeProcessNotificationRequest(req ProcessNotificationRequest) ProcessNotificationPlan {
	plan := ProcessNotificationPlan{
		NotifyOnComplete: req.NotifyOnComplete,
		WatchPatterns:    append([]string(nil), req.WatchPatterns...),
	}
	if req.Background && req.NotifyOnComplete && len(req.WatchPatterns) > 0 {
		plan.WatchPatterns = nil
		plan.Evidence = append(plan.Evidence, ProcessNotificationEvidence{
			Status:  "watch_patterns_disabled",
			Message: "watch_patterns ignored because notify_on_complete=true; these settings would produce duplicate delayed notifications",
		})
	}
	return plan
}

// ProcessNotificationSession is the notification state for one background
// process session. It intentionally does not own a live process handle.
type ProcessNotificationSession struct {
	ID               string
	SessionKey       string
	Command          string
	NotifyOnComplete bool
	WatchPatterns    []string
	Exited           bool

	watchHits               int
	watchSuppressed         int
	watchDisabled           bool
	watchCooldownUntil      time.Time
	watchStrikeCandidate    bool
	watchConsecutiveStrikes int
}

// ProcessNotificationEvent is a queued notification or degraded-mode summary
// produced by the policy.
type ProcessNotificationEvent struct {
	Type       string
	SessionID  string
	SessionKey string
	Command    string
	Pattern    string
	Output     string
	Suppressed int
	Message    string
}

// ProcessNotificationPolicy applies watch-pattern notification throttles.
type ProcessNotificationPolicy struct {
	now func() time.Time

	globalWindowStart          time.Time
	globalWindowHits           int
	globalTrippedUntil         time.Time
	globalSuppressedDuringTrip int
}

// NewProcessNotificationPolicy returns a process notification policy. Tests can
// pass a fake clock; production callers may pass nil to use time.Now.
func NewProcessNotificationPolicy(now func() time.Time) *ProcessNotificationPolicy {
	if now == nil {
		now = time.Now
	}
	return &ProcessNotificationPolicy{now: now}
}

// CheckWatchPatterns scans new output and returns the notifications admitted by
// the per-session and global flood-control policy.
func (p *ProcessNotificationPolicy) CheckWatchPatterns(session *ProcessNotificationSession, newText string) []ProcessNotificationEvent {
	if p == nil || session == nil || session.Exited || session.watchDisabled || len(session.WatchPatterns) == 0 {
		return nil
	}

	matchedPattern, matchedLines := matchProcessWatchPatterns(session.WatchPatterns, newText)
	if len(matchedLines) == 0 {
		return nil
	}

	now := p.now()
	if !session.watchCooldownUntil.IsZero() && now.Before(session.watchCooldownUntil) {
		session.watchSuppressed += len(matchedLines)
		if !session.watchStrikeCandidate {
			session.watchStrikeCandidate = true
			session.watchConsecutiveStrikes++
			if session.watchConsecutiveStrikes >= ProcessWatchStrikeLimit {
				session.watchDisabled = true
				session.WatchPatterns = nil
				session.NotifyOnComplete = true
				return []ProcessNotificationEvent{{
					Type:       "watch_disabled",
					SessionID:  session.ID,
					SessionKey: session.SessionKey,
					Command:    session.Command,
					Suppressed: session.watchSuppressed,
					Message:    "watch_patterns disabled after consecutive rate-limit windows; promoted to notify_on_complete semantics",
				}}
			}
		}
		return nil
	}

	if !session.watchCooldownUntil.IsZero() && !session.watchStrikeCandidate {
		session.watchConsecutiveStrikes = 0
	}
	session.watchStrikeCandidate = false
	session.watchCooldownUntil = now.Add(ProcessWatchMinInterval)
	session.watchHits++
	suppressed := session.watchSuppressed
	session.watchSuppressed = 0

	admitted, events := p.globalAdmit(now)
	if !admitted {
		return events
	}

	events = append(events, ProcessNotificationEvent{
		Type:       "watch_match",
		SessionID:  session.ID,
		SessionKey: session.SessionKey,
		Command:    session.Command,
		Pattern:    matchedPattern,
		Output:     formatProcessWatchOutput(matchedLines),
		Suppressed: suppressed,
	})
	return events
}

func matchProcessWatchPatterns(patterns []string, text string) (string, []string) {
	var matchedPattern string
	var matchedLines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		for _, pattern := range patterns {
			if pattern == "" {
				continue
			}
			if strings.Contains(line, pattern) {
				matchedLines = append(matchedLines, line)
				if matchedPattern == "" {
					matchedPattern = pattern
				}
				break
			}
		}
	}
	return matchedPattern, matchedLines
}

func formatProcessWatchOutput(lines []string) string {
	if len(lines) > 20 {
		lines = lines[:20]
	}
	out := strings.Join(lines, "\n")
	if len(out) > 2000 {
		out = out[:2000] + "\n...(truncated)"
	}
	return out
}

func (p *ProcessNotificationPolicy) globalAdmit(now time.Time) (bool, []ProcessNotificationEvent) {
	var events []ProcessNotificationEvent
	if !p.globalTrippedUntil.IsZero() && !now.Before(p.globalTrippedUntil) {
		suppressed := p.globalSuppressedDuringTrip
		p.globalTrippedUntil = time.Time{}
		p.globalSuppressedDuringTrip = 0
		p.globalWindowStart = now
		p.globalWindowHits = 0
		if suppressed > 0 {
			events = append(events, ProcessNotificationEvent{
				Type:       "watch_overflow_released",
				Suppressed: suppressed,
				Message:    "watch-pattern notifications resumed after global overflow suppression",
			})
		}
	}

	if !p.globalTrippedUntil.IsZero() && now.Before(p.globalTrippedUntil) {
		p.globalSuppressedDuringTrip++
		return false, events
	}

	if p.globalWindowStart.IsZero() || now.Sub(p.globalWindowStart) >= ProcessWatchGlobalWindow {
		p.globalWindowStart = now
		p.globalWindowHits = 0
	}

	if p.globalWindowHits >= ProcessWatchGlobalMaxPerWindow {
		p.globalTrippedUntil = now.Add(ProcessWatchGlobalCooldown)
		p.globalSuppressedDuringTrip++
		events = append(events, ProcessNotificationEvent{
			Type:    "watch_overflow_tripped",
			Message: "watch-pattern global overflow tripped; suppressing further watch_match events temporarily",
		})
		return false, events
	}

	p.globalWindowHits++
	return true, events
}
