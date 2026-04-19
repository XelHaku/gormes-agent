// Package telemetry derives per-session counters from SSE events. In Phase 1
// there is no DB and no external exporter; the kernel reads Snapshot() each
// frame and the TUI renders the numbers in the sidebar.
package telemetry

import "time"

// Telemetry is the interface for per-session metrics collection.
type Telemetry interface {
	SetModel(m string)
	StartTurn()
	Tick(tokensOut int)
	FinishTurn(latency time.Duration)
	SetTokensIn(n int)
	Snapshot() Snapshot
}

var _ Telemetry = (*telemetry)(nil)

type Snapshot struct {
	Model          string
	TokensInTotal  int
	TokensOutTotal int
	LatencyMsLast  int
	TokensPerSec   float64
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

func (t *telemetry) FinishTurn(latency time.Duration) {
	t.snap.LatencyMsLast = int(latency / time.Millisecond)
}

func (t *telemetry) SetTokensIn(n int) { t.snap.TokensInTotal += n }

func (t *telemetry) Snapshot() Snapshot { return t.snap }
