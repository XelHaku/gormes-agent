package kernel

import (
	"errors"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
)

// ErrResetDuringTurn is returned by Kernel.ResetSession when the kernel is
// not in a resettable phase (PhaseIdle or PhaseFailed). Preserves the
// Zero-Leak Invariant: in-flight turns are never truncated by reset.
var ErrResetDuringTurn = errors.New("kernel: cannot reset session during active turn")

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
	Seq            uint64
	Phase          Phase
	DraftText      string
	History        []hermes.Message
	Telemetry      telemetry.Snapshot
	ProviderStatus hermes.ProviderStatus
	RetryStatus    RetryStatus
	StatusText     string
	SessionID      string
	Model          string
	LastError      string
	SoulEvents     []SoulEntry
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
	// PlatformEventResetSession clears k.history, k.sessionID, and
	// k.lastError. Valid from PhaseIdle and PhaseFailed; rejected with
	// ErrResetDuringTurn via the event's ack channel otherwise.
	PlatformEventResetSession
)

type PlatformEvent struct {
	Kind PlatformEventKind
	Text string
	// SessionID, when non-empty, overrides k.sessionID for this turn
	// only. Used by the Phase 2.D cron executor so each cron fire has
	// an isolated "cron:<job_id>:<unix_ts>" session. A non-cron event
	// leaves this empty and inherits k.sessionID as before. The
	// override is per-event — the kernel's resident sessionID is NOT
	// mutated; after the turn completes, the next non-cron event uses
	// whatever k.sessionID was before.
	SessionID string
	// SessionContext, when non-empty, is injected as the first system
	// message for this turn. Gateway frontends use it to describe the
	// current source chat and delivery options without mutating the
	// kernel's long-lived config.
	SessionContext string
	// CronJobID, when non-empty, causes the kernel to set cron=1 in the
	// AppendUserTurn payload, marking the persisted turn row as a cron
	// turn. The extractor (T3) uses this to skip cron turns during
	// entity extraction so agent-generated cron outputs don't corrupt
	// user representations. Opaque to the kernel — just passed through
	// to the store.Command payload.
	CronJobID string
	// ack is an unexported synchronous result channel used by
	// ResetSession. External callers constructing PlatformEvents for
	// Submit() cannot set this field, which is the desired API — the
	// synchronous ResetSession path is the only one that needs it.
	ack chan error
}
