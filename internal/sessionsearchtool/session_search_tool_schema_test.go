package sessionsearchtool

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
)

func TestSessionSearchToolSchema_Descriptor(t *testing.T) {
	tool := &SessionSearchTool{}
	var _ tools.Tool = tool

	if got := tool.Name(); got != "session_search" {
		t.Fatalf("Name() = %q, want session_search", got)
	}
	if tool.Description() == "" {
		t.Fatal("Description() is empty")
	}
	if got := tool.Timeout(); got != 5*time.Second {
		t.Fatalf("Timeout() = %v, want 5s", got)
	}

	var schema struct {
		Type       string `json:"type"`
		Required   []string
		Properties map[string]struct {
			Type    string   `json:"type"`
			Enum    []string `json:"enum,omitempty"`
			Default any      `json:"default,omitempty"`
			Items   *struct {
				Type string `json:"type"`
			} `json:"items,omitempty"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("Schema() invalid JSON: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("schema type = %q, want object", schema.Type)
	}
	for _, name := range []string{"query", "scope", "sources", "mode", "limit", "current_session_id"} {
		if _, ok := schema.Properties[name]; !ok {
			t.Fatalf("schema properties missing %q: %s", name, tool.Schema())
		}
		if slices.Contains(schema.Required, name) {
			t.Fatalf("%q must be optional in schema required=%v", name, schema.Required)
		}
	}
	if got := schema.Properties["query"].Type; got != "string" {
		t.Fatalf("query type = %q, want string", got)
	}
	if got := schema.Properties["scope"].Enum; !slices.Equal(got, []string{"same-chat", "user"}) {
		t.Fatalf("scope enum = %v, want same-chat,user", got)
	}
	if got := schema.Properties["scope"].Default; got != "same-chat" {
		t.Fatalf("scope default = %#v, want same-chat", got)
	}
	if schema.Properties["sources"].Items == nil || schema.Properties["sources"].Items.Type != "string" {
		t.Fatalf("sources items = %#v, want string array", schema.Properties["sources"].Items)
	}
	if got := schema.Properties["mode"].Enum; !slices.Equal(got, []string{"default", "recent", "search"}) {
		t.Fatalf("mode enum = %v, want default,recent,search", got)
	}
	if got := schema.Properties["mode"].Default; got != "default" {
		t.Fatalf("mode default = %#v, want default", got)
	}
	if got := schema.Properties["limit"].Type; got != "integer" {
		t.Fatalf("limit type = %q, want integer", got)
	}
	if got := schema.Properties["current_session_id"].Type; got != "string" {
		t.Fatalf("current_session_id type = %q, want string", got)
	}
}

func TestSessionSearchToolSchema_DefaultArgs(t *testing.T) {
	args, evidence := ValidateSessionSearchArgs(json.RawMessage(`{"query":"orchid","limit":4,"current_session_id":"sess-1"}`))
	if evidence != nil {
		t.Fatalf("ValidateSessionSearchArgs returned evidence: %+v", evidence)
	}
	if args.Query != "orchid" {
		t.Fatalf("Query = %q, want orchid", args.Query)
	}
	if args.Scope != "same-chat" {
		t.Fatalf("Scope = %q, want same-chat", args.Scope)
	}
	if args.Mode != "default" {
		t.Fatalf("Mode = %q, want default", args.Mode)
	}
	if len(args.Sources) != 0 {
		t.Fatalf("Sources = %#v, want omitted sources normalized to empty allowlist", args.Sources)
	}
	if args.Limit != 4 {
		t.Fatalf("Limit = %d, want provided limit", args.Limit)
	}
	if args.CurrentSessionID != "sess-1" {
		t.Fatalf("CurrentSessionID = %q, want sess-1", args.CurrentSessionID)
	}
}

func TestSessionSearchToolSchema_RejectsUnsafeScope(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantField string
	}{
		{name: "unknown scope", raw: `{"scope":"global"}`, wantField: "scope"},
		{name: "unknown mode", raw: `{"mode":"archive"}`, wantField: "mode"},
		{name: "negative limit", raw: `{"limit":-1}`, wantField: "limit"},
		{name: "non-string source", raw: `{"sources":["discord",7]}`, wantField: "sources"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, evidence := ValidateSessionSearchArgs(json.RawMessage(tt.raw))
			if evidence == nil {
				t.Fatal("ValidateSessionSearchArgs evidence = nil, want invalid-args evidence")
			}
			if evidence.Status != "session_search_invalid_args" {
				t.Fatalf("Evidence.Status = %q, want session_search_invalid_args", evidence.Status)
			}
			if evidence.Field != tt.wantField {
				t.Fatalf("Evidence.Field = %q, want %q", evidence.Field, tt.wantField)
			}
			if evidence.Reason == "" {
				t.Fatal("Evidence.Reason is empty")
			}

			out, err := (&SessionSearchTool{}).Execute(context.Background(), json.RawMessage(tt.raw))
			if err != nil {
				t.Fatalf("Execute returned error for degraded invalid args: %v", err)
			}
			var payload struct {
				Success  bool                    `json:"success"`
				Evidence []SessionSearchEvidence `json:"evidence"`
			}
			if err := json.Unmarshal(out, &payload); err != nil {
				t.Fatalf("Execute output invalid JSON: %s: %v", out, err)
			}
			if payload.Success {
				t.Fatalf("Execute success = true for invalid args output %s", out)
			}
			if len(payload.Evidence) != 1 || payload.Evidence[0].Status != "session_search_invalid_args" {
				t.Fatalf("Execute evidence = %+v, want session_search_invalid_args", payload.Evidence)
			}
		})
	}
}

func TestSessionSearchToolSchema_NotRegisteredGlobally(t *testing.T) {
	reg := tools.NewRegistry()
	if _, ok := reg.Get("session_search"); ok {
		t.Fatal("new registry unexpectedly contains session_search")
	}

	for _, tool := range []tools.Tool{&tools.EchoTool{}, &tools.NowTool{}, &tools.RandIntTool{}} {
		if tool.Name() == "session_search" {
			t.Fatalf("builtin tool list unexpectedly contains %T as session_search", tool)
		}
	}

	if err := reg.Register(&SessionSearchTool{}); err != nil {
		t.Fatalf("local Register(SessionSearchTool): %v", err)
	}
	if _, ok := reg.Get("session_search"); !ok {
		t.Fatal("local registry should contain explicitly registered session_search")
	}
}
