package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// MCPCallResult is the normalized envelope produced by an MCP `tools/call`
// response. Content captures the structured body in the same StructuredContent
// shape used by NormalizeTools/RenderToolCallResult so call sites do not need
// transport-specific decoders. IsError mirrors the protocol's `isError`
// boolean: a true value means the tool reported a failure inside an otherwise
// successful JSON-RPC response (transport-level errors stay separate).
type MCPCallResult struct {
	Content []StructuredContent
	IsError bool
}

// rawToolCallResult mirrors the on-the-wire shape of an MCP tools/call
// response. Content blocks are decoded into a representation-agnostic
// StructuredContent slice via parseMCPCallResult.
type rawToolCallResult struct {
	Content []rawToolCallContent `json:"content"`
	IsError bool                 `json:"isError"`
}

// rawToolCallContent captures the fields the StructuredContent renderer needs
// from each MCP content block. Unknown fields are ignored so SDK extensions
// degrade gracefully instead of failing the parse.
type rawToolCallContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
	Resource *struct {
		URI      string `json:"uri,omitempty"`
		MimeType string `json:"mimeType,omitempty"`
	} `json:"resource,omitempty"`
}

// parseMCPCallResult turns a raw `result` JSON document into a
// transport-free MCPCallResult. Empty bodies are valid: tools that report
// success without content come back with IsError=false and zero Content
// blocks so callers can render them as a no-op.
func parseMCPCallResult(raw json.RawMessage) (MCPCallResult, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return MCPCallResult{}, nil
	}
	var decoded rawToolCallResult
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return MCPCallResult{}, fmt.Errorf("mcp call: parse result: %w", err)
	}
	out := MCPCallResult{IsError: decoded.IsError}
	if len(decoded.Content) == 0 {
		return out, nil
	}
	out.Content = make([]StructuredContent, 0, len(decoded.Content))
	for _, block := range decoded.Content {
		out.Content = append(out.Content, normalizeCallContent(block))
	}
	return out, nil
}

// normalizeCallContent collapses a single content block into the shared
// StructuredContent shape. Unknown kinds keep their type label so callers
// can branch on it, and resource blocks merge their nested `resource.uri`
// into the top-level URI field that RenderToolCallResult inspects.
func normalizeCallContent(block rawToolCallContent) StructuredContent {
	out := StructuredContent{
		Kind:     block.Type,
		Text:     block.Text,
		MimeType: block.MimeType,
		URI:      block.URI,
	}
	if block.Resource != nil {
		if out.URI == "" {
			out.URI = block.Resource.URI
		}
		if out.MimeType == "" {
			out.MimeType = block.Resource.MimeType
		}
	}
	return out
}

// CallTool invokes the named tool over the HTTP transport and decodes the
// response into the shared MCPCallResult shape. Arguments may be nil; the
// MCP server then receives an empty object so providers that require an
// `arguments` field do not see a malformed request.
//
// Transport-level failures (connectivity, auth, timeouts) bubble up as the
// underlying error so callers can branch on errors.Is(ErrAuthRequired) and
// friends. Application-level failures (the server reports `isError: true`)
// surface inside the returned MCPCallResult instead so structured content
// is preserved for the caller.
func (c *HTTPClient) CallTool(ctx context.Context, name string, arguments map[string]any) (MCPCallResult, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	var raw json.RawMessage
	if err := c.call(ctx, "tools/call", params, &raw); err != nil {
		return MCPCallResult{}, err
	}
	return parseMCPCallResult(raw)
}
