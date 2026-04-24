package gateway

import (
	"sort"
	"sync"
	"time"
)

// LifecyclePhase is the coarse channel lifecycle state tracked by the gateway
// manager's in-process status model. Values are stable strings so later
// read-model persistence (gateway_state.json, pairing.json) can marshal them
// without a translation table.
type LifecyclePhase string

const (
	// LifecyclePhaseRegistered means the manager knows about the channel but
	// has not yet started its Run loop.
	LifecyclePhaseRegistered LifecyclePhase = "registered"
	// LifecyclePhaseRunning means the per-channel Run goroutine is active.
	LifecyclePhaseRunning LifecyclePhase = "running"
	// LifecyclePhaseDisconnected means the Run goroutine returned cleanly
	// (nil or context.Canceled).
	LifecyclePhaseDisconnected LifecyclePhase = "disconnected"
	// LifecyclePhaseFailed means the Run goroutine returned a non-cancel
	// error; LastError carries the message.
	LifecyclePhaseFailed LifecyclePhase = "failed"
)

// ChannelStatus is a snapshot of one registered channel's lifecycle state as
// known to the gateway manager.
type ChannelStatus struct {
	Platform    string
	Phase       LifecyclePhase
	LastUpdated time.Time
	LastError   string
}

// StatusModel is the in-process read-model seam that later on-disk status
// persistence will wrap. Channel lifecycle writers call the Mark* methods;
// read consumers (the future `gormes gateway status` command and operator
// surfaces) use Snapshot or Lookup.
type StatusModel struct {
	now func() time.Time

	mu       sync.RWMutex
	channels map[string]ChannelStatus
}

// NewStatusModel returns a StatusModel backed by time.Now.
func NewStatusModel() *StatusModel {
	return NewStatusModelWithClock(nil)
}

// NewStatusModelWithClock lets tests inject a deterministic clock. A nil clock
// falls back to time.Now.
func NewStatusModelWithClock(now func() time.Time) *StatusModel {
	if now == nil {
		now = time.Now
	}
	return &StatusModel{
		now:      now,
		channels: map[string]ChannelStatus{},
	}
}

func (s *StatusModel) mark(platform string, phase LifecyclePhase, err error) {
	if s == nil || platform == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.channels[platform]
	entry.Platform = platform
	entry.Phase = phase
	entry.LastUpdated = s.now()
	if err != nil {
		entry.LastError = err.Error()
	} else {
		entry.LastError = ""
	}
	s.channels[platform] = entry
}

// MarkRegistered records that the manager has learned of the channel.
func (s *StatusModel) MarkRegistered(platform string) {
	s.mark(platform, LifecyclePhaseRegistered, nil)
}

// MarkRunning records that the per-channel Run goroutine has started.
func (s *StatusModel) MarkRunning(platform string) {
	s.mark(platform, LifecyclePhaseRunning, nil)
}

// MarkDisconnected records that the channel's Run returned without a
// non-cancel error. A nil err clears the historical LastError.
func (s *StatusModel) MarkDisconnected(platform string, err error) {
	s.mark(platform, LifecyclePhaseDisconnected, err)
}

// MarkFailed records that the channel's Run returned a non-cancel error.
func (s *StatusModel) MarkFailed(platform string, err error) {
	s.mark(platform, LifecyclePhaseFailed, err)
}

// Lookup returns the status entry for a platform, if any.
func (s *StatusModel) Lookup(platform string) (ChannelStatus, bool) {
	if s == nil {
		return ChannelStatus{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.channels[platform]
	return entry, ok
}

// Snapshot returns a stable, platform-sorted copy of every tracked channel
// status.
func (s *StatusModel) Snapshot() []ChannelStatus {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ChannelStatus, 0, len(s.channels))
	for _, entry := range s.channels {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Platform < out[j].Platform })
	return out
}
