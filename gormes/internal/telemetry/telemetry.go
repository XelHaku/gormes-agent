// Package telemetry derives per-session counters from SSE events. In Phase 1
// there is no DB and no external exporter; the kernel reads Snapshot() each
// frame and the TUI renders the numbers in the sidebar.
package telemetry

import "time"

type TurnStatus string

const (
	TurnStatusCompleted TurnStatus = "completed"
	TurnStatusFailed    TurnStatus = "failed"
	TurnStatusCancelled TurnStatus = "cancelled"
)

type ToolStatus string

const (
	ToolStatusCompleted ToolStatus = "completed"
	ToolStatusFailed    ToolStatus = "failed"
	ToolStatusCancelled ToolStatus = "cancelled"
)

// Telemetry is the interface for per-session metrics collection.
type Telemetry interface {
	SetModel(m string)
	StartTurn()
	Tick(tokensOut int)
	FinishTurn(latency time.Duration, status TurnStatus)
	RecordToolCall(status ToolStatus)
	SetTokensIn(n int)
	Snapshot() Snapshot
}

var _ Telemetry = (*telemetry)(nil)

type Snapshot struct {
	Model              string
	TokensInTotal      int
	TokensOutTotal     int
	LatencyMsLast      int
	TokensPerSec       float64
	TurnsTotal         int
	TurnsCompleted     int
	TurnsFailed        int
	TurnsCancelled     int
	ToolCallsTotal     int
	ToolCallsFailed    int
	ToolCallsCancelled int
	LastTurnStatus     TurnStatus
}

// telemetry is NOT goroutine-safe. The kernel holds it on its single owner
// goroutine; no other goroutine touches it. If that invariant changes, add
// a mutex or an atomic snapshot read.
type telemetry struct {
	snap       Snapshot
	turnStart  time.Time
	turnTokens int
	ema        float64
}

func New() Telemetry { return &telemetry{} }

func (t *telemetry) SetModel(m string) { t.snap.Model = m }

func (t *telemetry) StartTurn() {
	t.turnStart = time.Now()
	t.turnTokens = 0
	t.snap.TurnsTotal++
}

func (t *telemetry) Tick(tokensOut int) {
	delta := tokensOut - t.turnTokens
	if delta < 0 {
		delta = 0
	}
	t.turnTokens = tokensOut
	t.snap.TokensOutTotal += delta
	if el := time.Since(t.turnStart).Seconds(); el > 0 {
		tps := float64(t.turnTokens) / el
		const alpha = 0.2
		t.ema = alpha*tps + (1-alpha)*t.ema
		t.snap.TokensPerSec = t.ema
	}
}

func (t *telemetry) FinishTurn(latency time.Duration, status TurnStatus) {
	t.snap.LatencyMsLast = int(latency / time.Millisecond)
	t.snap.LastTurnStatus = status
	t.bumpTurnStatus(status)
}

func (t *telemetry) bumpTurnStatus(status TurnStatus) {
	switch status {
	case TurnStatusCompleted:
		t.snap.TurnsCompleted++
	case TurnStatusFailed:
		t.snap.TurnsFailed++
	case TurnStatusCancelled:
		t.snap.TurnsCancelled++
	}
}

func (t *telemetry) RecordToolCall(status ToolStatus) {
	t.snap.ToolCallsTotal++
	t.bumpToolStatus(status)
}

func (t *telemetry) bumpToolStatus(status ToolStatus) {
	switch status {
	case ToolStatusFailed:
		t.snap.ToolCallsFailed++
	case ToolStatusCancelled:
		t.snap.ToolCallsCancelled++
	}
}

func (t *telemetry) SetTokensIn(n int) { t.snap.TokensInTotal += n }

func (t *telemetry) Snapshot() Snapshot { return t.snap }
