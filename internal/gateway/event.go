// Package gateway is the channel-agnostic messaging chassis for Gormes.
// Individual adapters translate SDK-specific traffic into InboundEvent and
// implement the Channel interface plus any capability sub-interfaces they
// support. The manager owns cross-channel mechanics like command
// normalization, session-map persistence, and outbound routing.
package gateway

import "strings"

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

	// Attachments carries platform-normalized inbound media references. The
	// shared gateway keeps this as metadata; adapters own platform-specific
	// download and fallback behavior.
	Attachments []Attachment
}

// ChatKey returns the internal/session map key shape for this event.
func (e InboundEvent) ChatKey() string {
	return e.Platform + ":" + e.ChatID
}

// SubmitText returns the text sent to the kernel for submit events, including
// deterministic attachment references when a channel supplied inbound media.
func (e InboundEvent) SubmitText() string {
	text := strings.TrimSpace(e.Text)
	if len(e.Attachments) == 0 {
		return text
	}

	lines := make([]string, 0, len(e.Attachments)+3)
	if text != "" {
		lines = append(lines, text, "")
	}
	lines = append(lines, "Attachments:")
	for _, att := range e.Attachments {
		if line := att.submitLine(); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Attachment is the channel-neutral media descriptor attached to an inbound
// event. SourceID preserves the platform-side media identifier so failures can
// still be diagnosed even when URL resolution fails.
type Attachment struct {
	Kind      string `json:"kind"`
	URL       string `json:"url,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	FileName  string `json:"fileName,omitempty"`
	SourceID  string `json:"sourceId,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (a Attachment) submitLine() string {
	kind := strings.TrimSpace(a.Kind)
	if kind == "" {
		kind = "attachment"
	}
	label := kind
	if fileName := strings.TrimSpace(a.FileName); fileName != "" {
		label += " " + fileName
	}

	target := strings.TrimSpace(a.URL)
	if target == "" {
		target = strings.TrimSpace(a.SourceID)
	}
	if target == "" {
		return ""
	}

	var meta []string
	if mediaType := strings.TrimSpace(a.MediaType); mediaType != "" {
		meta = append(meta, "mediaType="+mediaType)
	}
	if sourceID := strings.TrimSpace(a.SourceID); sourceID != "" {
		meta = append(meta, "sourceId="+sourceID)
	}
	if errText := strings.TrimSpace(a.Error); errText != "" {
		meta = append(meta, "error="+errText)
	}

	line := "- " + label + ": " + target
	if len(meta) > 0 {
		line += " (" + strings.Join(meta, ", ") + ")"
	}
	return line
}
