// Package gateway is the channel-agnostic messaging chassis for Gormes.
// Individual adapters translate SDK-specific traffic into InboundEvent and
// implement the Channel interface plus any capability sub-interfaces they
// support. The manager owns cross-channel mechanics like command
// normalization, session-map persistence, and outbound routing.
package gateway

// EventKind is the normalized command kind on an inbound message.
type EventKind int

const (
	// EventUnknown is an unrecognized slash command.
	EventUnknown EventKind = iota
	// EventSubmit carries user text for kernel.PlatformEventSubmit.
	EventSubmit
	// EventCancel maps to kernel.PlatformEventCancel.
	EventCancel
	// EventReset maps to kernel.PlatformEventResetSession.
	EventReset
	// EventStart is the help or welcome command.
	EventStart
)

// String returns the stable log/test representation of an EventKind.
func (k EventKind) String() string {
	switch k {
	case EventSubmit:
		return "submit"
	case EventCancel:
		return "cancel"
	case EventReset:
		return "reset"
	case EventStart:
		return "start"
	default:
		return "unknown"
	}
}

// InboundEvent is the platform-neutral form every channel emits into the
// shared gateway manager.
type InboundEvent struct {
	Platform string
	ChatID   string
	ChatName string
	UserID   string
	UserName string
	ThreadID string
	MsgID    string
	Kind     EventKind
	Text     string
}

// ChatKey returns the internal/session map key shape for this event.
func (e InboundEvent) ChatKey() string {
	return e.Platform + ":" + e.ChatID
}
