package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

// DurableWorkerRSSWatchdogReason is the machine-readable policy vocabulary
// emitted before the RSS watchdog is wired into the durable worker loop.
type DurableWorkerRSSWatchdogReason string

const (
	DurableWorkerRSSWatchdogDisabled    DurableWorkerRSSWatchdogReason = "rss_watchdog_disabled"
	DurableWorkerRSSThresholdExceeded   DurableWorkerRSSWatchdogReason = "rss_threshold_exceeded"
	DurableWorkerRSSWatchdogUnavailable DurableWorkerRSSWatchdogReason = "rss_watchdog_unavailable"
	DurableWorkerRSSDrainStarted        DurableWorkerRSSWatchdogReason = "rss_drain_started"
)

const durableWorkerBytesPerMiB = 1024 * 1024
const defaultDurableWorkerWatchdogStableRunAfter = 5 * time.Minute

type DurableWorkerWatchdogRestartReason string

const (
	DurableWorkerStableWatchdogRestart DurableWorkerWatchdogRestartReason = "stable_watchdog_restart"
	DurableWorkerWatchdogRestartCrash  DurableWorkerWatchdogRestartReason = "watchdog_restart"
)

// DurableWorkerRSSReader reads RSS in bytes. Tests inject deterministic readers;
// runtime integration can later supply process-specific measurements.
type DurableWorkerRSSReader func() (uint64, error)

// DurableWorkerClock supplies the observation timestamp for policy evidence.
type DurableWorkerClock func() time.Time

// DurableWorkerRSSWatchdogPolicy is a value-only RSS watchdog configuration.
type DurableWorkerRSSWatchdogPolicy struct {
	MaxRSSMB int64
}

// DurableWorkerRSSWatchdogDecision is the pure policy result.
type DurableWorkerRSSWatchdogDecision struct {
	Reason       DurableWorkerRSSWatchdogReason
	RequestDrain bool
	Evidence     DurableWorkerRSSWatchdogEvidence
}

// DurableWorkerRSSWatchdogEvidence is suitable for later ledger/status wiring.
type DurableWorkerRSSWatchdogEvidence struct {
	Reason     DurableWorkerRSSWatchdogReason `json:"reason"`
	ObservedMB int64                          `json:"observed_mb,omitempty"`
	MaxMB      int64                          `json:"max_mb,omitempty"`
	CheckedAt  time.Time                      `json:"checked_at,omitempty"`
	ErrorText  string                         `json:"error,omitempty"`
}

// DurableWorkerRSSWatchdogEvent is an auditable RSS watchdog observation.
type DurableWorkerRSSWatchdogEvent struct {
	JobID     string
	WorkerID  string
	Reason    DurableWorkerRSSWatchdogReason
	Evidence  DurableWorkerRSSWatchdogEvidence
	CreatedAt time.Time
}

// DurableWorkerWatchdogRestartPolicy classifies supervised watchdog exits.
type DurableWorkerWatchdogRestartPolicy struct {
	StableRunAfter time.Duration
}

// DurableWorkerWatchdogRestartInput is the value-only restart observation.
type DurableWorkerWatchdogRestartInput struct {
	StartedAt          time.Time
	ExitedAt           time.Time
	PreviousCrashCount int
	WatchdogExit       bool
}

// DurableWorkerWatchdogRestartDecision is the crash-count policy result.
type DurableWorkerWatchdogRestartDecision struct {
	Reason     DurableWorkerWatchdogRestartReason
	CrashCount int
}

// Check classifies the RSS watchdog policy without touching worker runtime state.
func (p DurableWorkerRSSWatchdogPolicy) Check(readRSS DurableWorkerRSSReader, now DurableWorkerClock) DurableWorkerRSSWatchdogDecision {
	if p.MaxRSSMB <= 0 {
		return DurableWorkerRSSWatchdogDecision{
			Reason: DurableWorkerRSSWatchdogDisabled,
			Evidence: DurableWorkerRSSWatchdogEvidence{
				Reason: DurableWorkerRSSWatchdogDisabled,
			},
		}
	}
	if readRSS == nil {
		return DurableWorkerRSSWatchdogDecision{
			Reason: DurableWorkerRSSWatchdogUnavailable,
			Evidence: DurableWorkerRSSWatchdogEvidence{
				Reason:    DurableWorkerRSSWatchdogUnavailable,
				CheckedAt: durableWorkerRSSNow(now),
				ErrorText: "rss reader is nil",
			},
		}
	}
	rssBytes, err := readRSS()
	if err != nil {
		return DurableWorkerRSSWatchdogDecision{
			Reason: DurableWorkerRSSWatchdogUnavailable,
			Evidence: DurableWorkerRSSWatchdogEvidence{
				Reason:    DurableWorkerRSSWatchdogUnavailable,
				CheckedAt: durableWorkerRSSNow(now),
				ErrorText: err.Error(),
			},
		}
	}
	observedMB := durableWorkerRSSBytesToMB(rssBytes)
	if observedMB >= p.MaxRSSMB {
		return DurableWorkerRSSWatchdogDecision{
			Reason:       DurableWorkerRSSThresholdExceeded,
			RequestDrain: true,
			Evidence: DurableWorkerRSSWatchdogEvidence{
				Reason:     DurableWorkerRSSThresholdExceeded,
				ObservedMB: observedMB,
				MaxMB:      p.MaxRSSMB,
				CheckedAt:  durableWorkerRSSNow(now),
			},
		}
	}
	return DurableWorkerRSSWatchdogDecision{}
}

// Classify resets watchdog restart accounting after a stable run.
func (p DurableWorkerWatchdogRestartPolicy) Classify(input DurableWorkerWatchdogRestartInput) DurableWorkerWatchdogRestartDecision {
	if input.WatchdogExit && !input.StartedAt.IsZero() && !input.ExitedAt.IsZero() && input.ExitedAt.Sub(input.StartedAt) >= p.stableRunAfter() {
		return DurableWorkerWatchdogRestartDecision{
			Reason:     DurableWorkerStableWatchdogRestart,
			CrashCount: 1,
		}
	}
	return DurableWorkerWatchdogRestartDecision{
		Reason:     DurableWorkerWatchdogRestartCrash,
		CrashCount: input.PreviousCrashCount + 1,
	}
}

func durableWorkerRSSBytesToMB(bytes uint64) int64 {
	mb := bytes / durableWorkerBytesPerMiB
	if bytes%durableWorkerBytesPerMiB >= durableWorkerBytesPerMiB/2 {
		mb++
	}
	return int64(mb)
}

func durableWorkerRSSNow(now DurableWorkerClock) time.Time {
	if now != nil {
		return now().UTC()
	}
	return time.Now().UTC()
}

func (p DurableWorkerWatchdogRestartPolicy) stableRunAfter() time.Duration {
	if p.StableRunAfter > 0 {
		return p.StableRunAfter
	}
	return defaultDurableWorkerWatchdogStableRunAfter
}

// DurableWorkerRSSDrain coordinates graceful RSS drains across concurrent
// DurableWorker.RunOne calls that share the same worker process.
type DurableWorkerRSSDrain struct {
	mu       sync.Mutex
	active   map[string]chan durableWorkerRSSDrainAbort
	draining bool
	abort    durableWorkerRSSDrainAbort
}

type durableWorkerRSSDrainAbort struct {
	Reason   DurableWorkerRSSWatchdogReason
	Evidence DurableWorkerRSSWatchdogEvidence
}

type durableWorkerRSSDrainRegistration struct {
	Abort      <-chan durableWorkerRSSDrainAbort
	unregister func()
}

func NewDurableWorkerRSSDrain() *DurableWorkerRSSDrain {
	return &DurableWorkerRSSDrain{}
}

func (d *DurableWorkerRSSDrain) Register(jobID, workerID string) durableWorkerRSSDrainRegistration {
	if d == nil {
		return durableWorkerRSSDrainRegistration{}
	}
	ch := make(chan durableWorkerRSSDrainAbort, 1)
	key := durableWorkerRSSDrainKey(jobID, workerID)

	d.mu.Lock()
	if d.active == nil {
		d.active = make(map[string]chan durableWorkerRSSDrainAbort)
	}
	d.active[key] = ch
	if d.draining {
		ch <- d.abort
	}
	d.mu.Unlock()

	return durableWorkerRSSDrainRegistration{
		Abort: ch,
		unregister: func() {
			d.mu.Lock()
			delete(d.active, key)
			d.mu.Unlock()
		},
	}
}

func (r durableWorkerRSSDrainRegistration) Unregister() {
	if r.unregister != nil {
		r.unregister()
	}
}

func (d *DurableWorkerRSSDrain) Start(reason DurableWorkerRSSWatchdogReason, evidence DurableWorkerRSSWatchdogEvidence) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.draining {
		return false
	}
	d.draining = true
	d.abort = durableWorkerRSSDrainAbort{
		Reason:   reason,
		Evidence: evidence,
	}
	for _, ch := range d.active {
		select {
		case ch <- d.abort:
		default:
		}
	}
	return true
}

func durableWorkerRSSDrainKey(jobID, workerID string) string {
	return strings.TrimSpace(workerID) + "\x00" + strings.TrimSpace(jobID)
}

func (l *DurableLedger) RecordWorkerRSSWatchdogEvent(ctx context.Context, event DurableWorkerRSSWatchdogEvent) error {
	if l == nil || l.db == nil {
		return errors.New("subagent: durable ledger is nil")
	}
	workerID := strings.TrimSpace(event.WorkerID)
	if workerID == "" {
		return errors.New("subagent: durable worker id is empty")
	}
	reason := event.Reason
	if reason == "" {
		reason = event.Evidence.Reason
	}
	if reason == "" {
		return errors.New("subagent: durable worker RSS watchdog reason is empty")
	}
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = durableTime(durableNow())
	}
	evidence := event.Evidence
	if evidence.Reason == "" {
		evidence.Reason = reason
	}
	if evidence.CheckedAt.IsZero() {
		evidence.CheckedAt = createdAt.UTC()
	}
	payload := map[string]any{
		"type":      string(reason),
		"job_id":    strings.TrimSpace(event.JobID),
		"worker_id": workerID,
		"reason":    string(reason),
		"evidence":  evidence,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = l.db.ExecContext(ctx, `
		INSERT INTO durable_worker_events
			(type, worker_id, reason, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		string(reason), workerID, string(reason), string(raw), createdAt.UTC().UnixNano())
	return err
}
