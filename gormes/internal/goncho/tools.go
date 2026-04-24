package goncho

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

// RegisterTools adds the Honcho-compatible tool surface backed by the
// in-binary Goncho service. Keep this package-side so internal/tools remains
// persistence-free for kernel build-isolation.
func RegisterTools(reg *tools.Registry, svc *Service) {
	if reg == nil {
		panic("goncho: nil registry")
	}
	if svc == nil {
		panic("goncho: nil service")
	}
	reg.MustRegisterEntry(tools.ToolEntry{Tool: &ProfileTool{Service: svc}, Toolset: "honcho"})
	reg.MustRegisterEntry(tools.ToolEntry{Tool: &SearchTool{Service: svc}, Toolset: "honcho"})
	reg.MustRegisterEntry(tools.ToolEntry{Tool: &ContextTool{Service: svc}, Toolset: "honcho"})
	reg.MustRegisterEntry(tools.ToolEntry{Tool: &ReasoningTool{Service: svc}, Toolset: "honcho"})
	reg.MustRegisterEntry(tools.ToolEntry{Tool: &ConcludeTool{Service: svc}, Toolset: "honcho"})
}

type ProfileTool struct {
	Service *Service
}

func (*ProfileTool) Name() string { return "honcho_profile" }
func (*ProfileTool) Description() string {
	return "Read or update the peer card for a peer. Pass card to update; omit it to read."
}
func (*ProfileTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string","description":"peer identity"},"card":{"type":"array","items":{"type":"string"},"description":"optional replacement peer card"}},"required":["peer"]}`)
}
func (*ProfileTool) Timeout() time.Duration { return 5 * time.Second }
func (t *ProfileTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Peer string   `json:"peer"`
		Card []string `json:"card"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("honcho_profile: invalid args: %w", err)
	}
	if strings.TrimSpace(in.Peer) == "" {
		return nil, fmt.Errorf("honcho_profile: peer is required")
	}
	if in.Card != nil {
		if err := t.Service.SetProfile(ctx, in.Peer, in.Card); err != nil {
			return nil, err
		}
	}
	out, err := t.Service.Profile(ctx, in.Peer)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

type SearchTool struct {
	Service *Service
}

func (*SearchTool) Name() string { return "honcho_search" }
func (*SearchTool) Description() string {
	return "Search stored Goncho memory for a peer. Returns raw retrieval results without LLM synthesis."
}
func (*SearchTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string"},"query":{"type":"string"},"max_tokens":{"type":"integer"},"session_key":{"type":"string"}},"required":["peer","query"]}`)
}
func (*SearchTool) Timeout() time.Duration { return 5 * time.Second }
func (t *SearchTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in SearchParams
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("honcho_search: invalid args: %w", err)
	}
	out, err := t.Service.Search(ctx, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

type ContextTool struct {
	Service *Service
}

func (*ContextTool) Name() string { return "honcho_context" }
func (*ContextTool) Description() string {
	return "Build a structured Goncho context block: peer card, representation, conclusions, and recent messages."
}
func (*ContextTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string"},"query":{"type":"string"},"max_tokens":{"type":"integer"},"session_key":{"type":"string"}},"required":["peer"]}`)
}
func (*ContextTool) Timeout() time.Duration { return 5 * time.Second }
func (t *ContextTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in ContextParams
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("honcho_context: invalid args: %w", err)
	}
	out, err := t.Service.Context(ctx, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

type ReasoningTool struct {
	Service *Service
}

func (*ReasoningTool) Name() string { return "honcho_reasoning" }
func (*ReasoningTool) Description() string {
	return "Answer a query from Goncho context. This slice uses deterministic synthesis from stored context."
}
func (*ReasoningTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string"},"query":{"type":"string"},"reasoning_level":{"type":"string"},"max_tokens":{"type":"integer"},"session_key":{"type":"string"}},"required":["peer","query"]}`)
}
func (*ReasoningTool) Timeout() time.Duration { return 5 * time.Second }
func (t *ReasoningTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
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
	contextResult, err := t.Service.Context(ctx, ContextParams{
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

type ConcludeTool struct {
	Service *Service
}

func (*ConcludeTool) Name() string { return "honcho_conclude" }
func (*ConcludeTool) Description() string {
	return "Create or delete a Goncho conclusion for a peer."
}
func (*ConcludeTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"peer":{"type":"string"},"conclusion":{"type":"string"},"delete_id":{"type":"integer"},"session_key":{"type":"string"}},"required":["peer"]}`)
}
func (*ConcludeTool) Timeout() time.Duration { return 5 * time.Second }
func (t *ConcludeTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in ConcludeParams
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("honcho_conclude: invalid args: %w", err)
	}
	out, err := t.Service.Conclude(ctx, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func deterministicReasoningAnswer(query string, ctx ContextResult) string {
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
