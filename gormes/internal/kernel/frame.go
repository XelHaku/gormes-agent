package kernel

import (
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
)

// Phase is the kernel state-machine phase. Transitions happen only on the
// Run goroutine, serialised by the select loop.
type Phase int

const (
	PhaseIdle Phase = iota
	PhaseConnecting
	PhaseStreaming
	PhaseFinalizing
	PhaseCancelling
	PhaseFailed
	// PhaseReconnecting is the TDD seed for Phase-1.5 Route-B resilience
	// (spec §9.2 of 2026-04-18-gormes-frontend-adapter-design.md). No
	// transitions to this state exist yet — the future reconnect plan
	// flips reconnect_test.go from Skip to real pass by wiring this up.
	PhaseReconnecting
)

func (p Phase) String() string {
	return [...]string{"Idle", "Connecting", "Streaming", "Finalizing", "Cancelling", "Failed", "Reconnecting"}[p]
}

// RenderFrame is the only TUI input. The TUI never assembles assistant text
// from raw provider events; it renders this frame, full stop.
type RenderFrame struct {
	Seq        uint64
	Phase      Phase
	DraftText  string
	History    []hermes.Message
	Telemetry  telemetry.Snapshot
	StatusText string
	SessionID  string
	Model      string
	LastError  string
	SoulEvents []SoulEntry
}

type SoulEntry struct {
	At   time.Time
	Text string
}

// Mailbox capacities and timings. See spec §7.8 for the authoritative table.
const (
	RenderMailboxCap        = 1
	PlatformEventMailboxCap = 16

	FlushInterval    = 16 * time.Millisecond
	StoreAckDeadline = 250 * time.Millisecond
	ShutdownBudget   = 2 * time.Second
	SoulBufferSize   = 10
)

type PlatformEventKind int

const (
	PlatformEventSubmit PlatformEventKind = iota
	PlatformEventCancel
	PlatformEventQuit
)

type PlatformEvent struct {
	Kind PlatformEventKind
	Text string
}
