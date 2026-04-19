// Package gormes re-exports the stable Phase-1 public surface for external
// consumers. Every actual definition lives in an internal/ package; this file
// is purely type aliases so "import .../gormes/pkg/gormes" works as a single
// stable entry point across refactors.
package gormes

import (
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/pybridge"
)

// Hermes wire surface — everything Gormes needs to speak HTTP+SSE to a
// Hermes-compatible api_server.
type (
	Client         = hermes.Client
	Stream         = hermes.Stream
	RunEventStream = hermes.RunEventStream
	ChatRequest    = hermes.ChatRequest
	Message        = hermes.Message
	Event          = hermes.Event
	EventKind      = hermes.EventKind
	RunEvent       = hermes.RunEvent
	RunEventType   = hermes.RunEventType
	ErrorClass     = hermes.ErrorClass
	HTTPError      = hermes.HTTPError
)

// Kernel surface — the RenderFrame the TUI consumes plus the PlatformEvent
// it emits. External TUIs (future Bubble Tea alternatives, web UIs, etc.)
// can re-implement a UI by importing only these.
type (
	RenderFrame       = kernel.RenderFrame
	Phase             = kernel.Phase
	SoulEntry         = kernel.SoulEntry
	PlatformEvent     = kernel.PlatformEvent
	PlatformEventKind = kernel.PlatformEventKind
)

// Runtime seam — Phase-5 interface definitions, present in Phase 1 so
// downstream integrators can write conforming runtimes ahead of time.
type (
	Runtime    = pybridge.Runtime
	Invocation = pybridge.Invocation
)
