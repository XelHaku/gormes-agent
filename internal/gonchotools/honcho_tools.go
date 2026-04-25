package gonchotools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/goncho"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
)

// RegisterHonchoTools adds the Honcho-compatible tool surface backed by the
// in-binary Goncho service.
func RegisterHonchoTools(reg *tools.Registry, svc *goncho.Service) {
	if reg == nil {
		panic("tools: nil registry")
	}
	if svc == nil {
		panic("tools: nil goncho service")
	}
	reg.MustRegister(&HonchoProfileTool{Service: svc})
	reg.MustRegister(&HonchoSearchTool{Service: svc})
	reg.MustRegister(&HonchoContextTool{Service: svc})
	reg.MustRegister(&HonchoReasoningTool{Service: svc})
	reg.MustRegister(&HonchoConcludeTool{Service: svc})
}

type HonchoProfileTool struct {
	Service *goncho.Service
}

func (*HonchoProfileTool) Name() string { return "honcho_profile" }
func (*HonchoProfileTool) Description() string {
	return "Read or update the peer card for a peer. Pass card to update; omit it to read."
}
func (*HonchoProfileTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string","description":"peer identity"},"target":{"type":"string","description":"optional observed peer for a directional card from peer's perspective"},"card":{"type":"array","items":{"type":"string"},"description":"optional replacement peer card"}},"required":["peer"]}`)
}
func (*HonchoProfileTool) Timeout() time.Duration { return 5 * time.Second }
func (t *HonchoProfileTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Peer   string   `json:"peer"`
		Target string   `json:"target"`
		Card   []string `json:"card"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("honcho_profile: invalid args: %w", err)
	}
	if strings.TrimSpace(in.Peer) == "" {
		return nil, fmt.Errorf("honcho_profile: peer is required")
	}
	if in.Card != nil {
		var err error
		if strings.TrimSpace(in.Target) == "" {
			err = t.Service.SetProfile(ctx, in.Peer, in.Card)
		} else {
			err = t.Service.SetProfileForTarget(ctx, in.Peer, in.Target, in.Card)
		}
		if err != nil {
			return nil, err
		}
	}
	var out goncho.ProfileResult
	var err error
	if strings.TrimSpace(in.Target) == "" {
		out, err = t.Service.Profile(ctx, in.Peer)
	} else {
		out, err = t.Service.ProfileForTarget(ctx, in.Peer, in.Target)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

type HonchoSearchTool struct {
	Service *goncho.Service
}

func (*HonchoSearchTool) Name() string { return "honcho_search" }
func (*HonchoSearchTool) Description() string {
	return "Search stored Goncho memory for a peer. Returns raw retrieval results without LLM synthesis."
}
func (*HonchoSearchTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string"},"query":{"type":"string"},"max_tokens":{"type":"integer"},"session_key":{"type":"string"},"scope":{"type":"string"},"sources":{"type":"array","items":{"type":"string"}}},"required":["peer","query"]}`)
}
func (*HonchoSearchTool) Timeout() time.Duration { return 5 * time.Second }
func (t *HonchoSearchTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in goncho.SearchParams
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("honcho_search: invalid args: %w", err)
	}
	out, err := t.Service.Search(ctx, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

type HonchoContextTool struct {
	Service *goncho.Service
}

func (*HonchoContextTool) Name() string { return "honcho_context" }
func (*HonchoContextTool) Description() string {
	return "Build a structured Goncho context block: peer card, representation, conclusions, and recent messages."
}
func (*HonchoContextTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string"},"query":{"type":"string"},"max_tokens":{"type":"integer"},"session_key":{"type":"string"},"scope":{"type":"string"},"sources":{"type":"array","items":{"type":"string"}},"peer_target":{"type":"string"},"peer_perspective":{"type":"string"},"limit_to_session":{"type":"boolean"},"search_top_k":{"type":"integer"},"search_max_distance":{"type":"number"},"include_most_frequent":{"type":"boolean"},"max_conclusions":{"type":"integer"}},"required":["peer"]}`)
}
func (*HonchoContextTool) Timeout() time.Duration { return 5 * time.Second }
func (t *HonchoContextTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in goncho.ContextParams
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("honcho_context: invalid args: %w", err)
	}
	out, err := t.Service.Context(ctx, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

type HonchoReasoningTool struct {
	Service *goncho.Service
}

func (*HonchoReasoningTool) Name() string { return "honcho_reasoning" }
func (*HonchoReasoningTool) Description() string {
	return "Answer a query from Goncho context. This slice uses deterministic synthesis from stored context."
}
func (*HonchoReasoningTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string"},"query":{"type":"string"},"reasoning_level":{"type":"string"},"max_tokens":{"type":"integer"},"session_key":{"type":"string"}},"required":["peer","query"]}`)
}
func (*HonchoReasoningTool) Timeout() time.Duration { return 5 * time.Second }
func (t *HonchoReasoningTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Peer           string `json:"peer"`
		Query          string `json:"query"`
		ReasoningLevel string `json:"reasoning_level"`
		MaxTokens      int    `json:"max_tokens"`
		SessionKey     string `json:"session_key"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("honcho_reasoning: invalid args: %w", err)
	}
	contextResult, err := t.Service.Context(ctx, goncho.ContextParams{
		Peer:       in.Peer,
		Query:      in.Query,
		MaxTokens:  in.MaxTokens,
		SessionKey: in.SessionKey,
	})
	if err != nil {
		return nil, err
	}
	answer := deterministicReasoningAnswer(in.Query, contextResult)
	out := struct {
		WorkspaceID    string `json:"workspace_id"`
		Peer           string `json:"peer"`
		ReasoningLevel string `json:"reasoning_level"`
		Answer         string `json:"answer"`
	}{
		WorkspaceID:    contextResult.WorkspaceID,
		Peer:           contextResult.Peer,
		ReasoningLevel: in.ReasoningLevel,
		Answer:         answer,
	}
	return json.Marshal(out)
}

type HonchoConcludeTool struct {
	Service *goncho.Service
}

func (*HonchoConcludeTool) Name() string { return "honcho_conclude" }
func (*HonchoConcludeTool) Description() string {
	return "Create or delete a Goncho conclusion for a peer."
}
func (*HonchoConcludeTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string"},"conclusion":{"type":"string"},"delete_id":{"type":"integer"},"session_key":{"type":"string"}},"required":["peer"]}`)
}
func (*HonchoConcludeTool) Timeout() time.Duration { return 5 * time.Second }
func (t *HonchoConcludeTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in goncho.ConcludeParams
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("honcho_conclude: invalid args: %w", err)
	}
	out, err := t.Service.Conclude(ctx, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func deterministicReasoningAnswer(query string, ctx goncho.ContextResult) string {
	var b strings.Builder
	if strings.TrimSpace(query) != "" {
		b.WriteString("Query: ")
		b.WriteString(strings.TrimSpace(query))
		b.WriteString("\n\n")
	}
	b.WriteString(ctx.Representation)
	if len(ctx.RecentMessages) > 0 {
		b.WriteString("\n\nRecent session evidence:")
		for _, msg := range ctx.RecentMessages {
			b.WriteString("\n- ")
			b.WriteString(msg.Role)
			b.WriteString(": ")
			b.WriteString(msg.Content)
		}
	}
	return b.String()
}
