package acp

import "context"

const ProtocolVersion = 1

type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonCancelled StopReason = "cancelled"
)

type Implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type TextContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type SessionUpdate struct {
	SessionUpdate string           `json:"sessionUpdate"`
	Content       TextContentBlock `json:"content"`
}

type PromptResult struct {
	StopReason StopReason `json:"stopReason"`
}

type Session interface {
	Prompt(ctx context.Context, prompt []ContentBlock, send func(SessionUpdate)) (PromptResult, error)
	Cancel()
	Close() error
}
