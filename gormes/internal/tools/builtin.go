package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"
)

// EchoTool round-trips its input. Proof-of-life for the Tool plumbing.
type EchoTool struct{}

func (*EchoTool) Name() string        { return "echo" }
func (*EchoTool) Description() string { return "Echo the provided text back. Useful for testing tool-call plumbing." }
func (*EchoTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","description":"text to echo"}},"required":["text"]}`)
}
func (*EchoTool) Timeout() time.Duration { return 0 }

func (*EchoTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("echo: invalid args: %w", err)
	}
	if in.Text == "" {
		return nil, errors.New("echo: 'text' is required and must be non-empty")
	}
	out := struct {
		Text string `json:"text"`
	}{Text: in.Text}
	return json.Marshal(out)
}

// NowTool returns the current time in two formats.
type NowTool struct{}

func (*NowTool) Name() string        { return "now" }
func (*NowTool) Description() string { return "Return the current server time as unix seconds and ISO-8601 UTC." }
func (*NowTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}
func (*NowTool) Timeout() time.Duration { return 0 }

func (*NowTool) Execute(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	now := time.Now().UTC()
	out := struct {
		Unix int64  `json:"unix"`
		ISO  string `json:"iso"`
	}{Unix: now.Unix(), ISO: now.Format(time.RFC3339)}
	return json.Marshal(out)
}

// RandIntTool returns a uniformly-random integer in [min, max].
type RandIntTool struct{}

func (*RandIntTool) Name() string        { return "rand_int" }
func (*RandIntTool) Description() string { return "Return a uniformly random integer in [min, max]." }
func (*RandIntTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"min":{"type":"integer"},"max":{"type":"integer"}},"required":["min","max"]}`)
}
func (*RandIntTool) Timeout() time.Duration { return 0 }

func (*RandIntTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Min int `json:"min"`
		Max int `json:"max"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("rand_int: invalid args: %w", err)
	}
	if in.Min > in.Max {
		return nil, fmt.Errorf("rand_int: min (%d) must be <= max (%d)", in.Min, in.Max)
	}
	value := in.Min + rand.Intn(in.Max-in.Min+1)
	out := struct {
		Value int `json:"value"`
	}{Value: value}
	return json.Marshal(out)
}
