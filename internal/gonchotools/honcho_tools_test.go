package gonchotools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/goncho"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
)

func TestHonchoTools_RegisterExpectedNames(t *testing.T) {
	reg, _, cleanup := newTestHonchoRegistry(t)
	defer cleanup()

	for _, name := range []string{
		"honcho_profile",
		"honcho_search",
		"honcho_context",
		"honcho_reasoning",
		"honcho_conclude",
	} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("%s not registered", name)
		}
	}
}

func TestHonchoSearchTool_SchemaExposesOptionalScopeAndSources(t *testing.T) {
	tool := &HonchoSearchTool{}
	if got := tool.Name(); got != "honcho_search" {
		t.Fatalf("Name() = %q, want honcho_search", got)
	}

	assertScopeAndSourcesSchema(t, tool.Schema(), []string{"peer", "query"})
}

func TestHonchoContextTool_SchemaExposesOptionalScopeAndSources(t *testing.T) {
	tool := &HonchoContextTool{}
	if got := tool.Name(); got != "honcho_context" {
		t.Fatalf("Name() = %q, want honcho_context", got)
	}

	assertScopeAndSourcesSchema(t, tool.Schema(), []string{"peer"})
}

func TestHonchoContextTool_SchemaExposesOptionalRepresentationOptions(t *testing.T) {
	tool := &HonchoContextTool{}

	assertOptionalSchemaProperties(t, tool.Schema(), []string{"peer"}, map[string]string{
		"peer_target":           "string",
		"peer_perspective":      "string",
		"limit_to_session":      "boolean",
		"search_top_k":          "integer",
		"search_max_distance":   "number",
		"include_most_frequent": "boolean",
		"max_conclusions":       "integer",
	})
}

func TestHonchoSearchTool_OmittedScopeSourcesPreservesSameChatDefault(t *testing.T) {
	reg, svc, cleanup := newTestHonchoRegistry(t)
	defer cleanup()

	ctx := context.Background()
	seedScopedConclusions(t, ctx, svc)

	output := executeHonchoTool(t, reg, "honcho_search", json.RawMessage(`{
		"peer":"telegram:6586915095",
		"query":"codename",
		"session_key":"telegram:6586915095"
	}`))
	if !strings.Contains(string(output), "same-chat codename orchid") {
		t.Fatalf("search output missing same-chat result: %s", output)
	}
	if strings.Contains(string(output), "other-chat codename orchid") {
		t.Fatalf("search output leaked other chat result: %s", output)
	}
}

func TestHonchoContextTool_OmittedScopeSourcesPreservesSameChatDefault(t *testing.T) {
	reg, svc, cleanup := newTestHonchoRegistry(t)
	defer cleanup()

	ctx := context.Background()
	seedScopedConclusions(t, ctx, svc)

	output := executeHonchoTool(t, reg, "honcho_context", json.RawMessage(`{
		"peer":"telegram:6586915095",
		"query":"codename",
		"session_key":"telegram:6586915095"
	}`))
	if !strings.Contains(string(output), "same-chat codename orchid") {
		t.Fatalf("context output missing same-chat result: %s", output)
	}
	if strings.Contains(string(output), "other-chat codename orchid") {
		t.Fatalf("context output leaked other chat result: %s", output)
	}
}

func TestHonchoProfileTool_UsesService(t *testing.T) {
	reg, svc, cleanup := newTestHonchoRegistry(t)
	defer cleanup()

	ctx := context.Background()
	if err := svc.SetProfile(ctx, "telegram:6586915095", []string{"Blind", "Prefers exact outputs"}); err != nil {
		t.Fatal(err)
	}

	exec := tools.NewInProcessToolExecutor(reg)
	ch, err := exec.Execute(ctx, tools.ToolRequest{
		ToolName: "honcho_profile",
		Input:    json.RawMessage(`{"peer":"telegram:6586915095"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	var outputs []tools.ToolEvent
	for ev := range ch {
		outputs = append(outputs, ev)
	}
	if len(outputs) != 3 {
		t.Fatalf("event count = %d, want 3", len(outputs))
	}
	if !strings.Contains(string(outputs[1].Output), `"Prefers exact outputs"`) {
		t.Fatalf("profile output = %s", outputs[1].Output)
	}
}

func TestHonchoReasoningTool_ReturnsDeterministicAnswer(t *testing.T) {
	reg, svc, cleanup := newTestHonchoRegistry(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := svc.Conclude(ctx, goncho.ConcludeParams{
		Peer:       "telegram:6586915095",
		Conclusion: "The user prefers exact evidence-first reports.",
		SessionKey: "telegram:6586915095",
	}); err != nil {
		t.Fatal(err)
	}

	exec := tools.NewInProcessToolExecutor(reg)
	ch, err := exec.Execute(ctx, tools.ToolRequest{
		ToolName: "honcho_reasoning",
		Input: json.RawMessage(`{
			"peer":"telegram:6586915095",
			"query":"How should I answer?",
			"reasoning_level":"low",
			"session_key":"telegram:6586915095"
		}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	var outputs []tools.ToolEvent
	for ev := range ch {
		outputs = append(outputs, ev)
	}
	if len(outputs) != 3 {
		t.Fatalf("event count = %d, want 3", len(outputs))
	}
	if !strings.Contains(string(outputs[1].Output), `"answer"`) {
		t.Fatalf("reasoning output = %s", outputs[1].Output)
	}
	if !strings.Contains(string(outputs[1].Output), `exact evidence-first reports`) {
		t.Fatalf("reasoning output missing conclusion: %s", outputs[1].Output)
	}
}

func assertScopeAndSourcesSchema(t *testing.T, raw json.RawMessage, wantRequired []string) {
	t.Helper()

	var schema struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Items *struct {
				Type string `json:"type"`
			} `json:"items,omitempty"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}

	scope, ok := schema.Properties["scope"]
	if !ok {
		t.Fatalf("schema properties missing scope: %s", raw)
	}
	if scope.Type != "string" {
		t.Fatalf("scope type = %q, want string", scope.Type)
	}

	sources, ok := schema.Properties["sources"]
	if !ok {
		t.Fatalf("schema properties missing sources: %s", raw)
	}
	if sources.Type != "array" {
		t.Fatalf("sources type = %q, want array", sources.Type)
	}
	if sources.Items == nil || sources.Items.Type != "string" {
		t.Fatalf("sources items = %+v, want string items", sources.Items)
	}

	required := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = true
	}
	for _, name := range []string{"scope", "sources"} {
		if required[name] {
			t.Fatalf("%s is required in schema: %s", name, raw)
		}
	}
	if len(schema.Required) != len(wantRequired) {
		t.Fatalf("required = %v, want %v", schema.Required, wantRequired)
	}
	for _, name := range wantRequired {
		if !required[name] {
			t.Fatalf("required = %v, missing %s", schema.Required, name)
		}
	}
}

func assertOptionalSchemaProperties(t *testing.T, raw json.RawMessage, wantRequired []string, wantTypes map[string]string) {
	t.Helper()

	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}

	required := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = true
	}
	if len(schema.Required) != len(wantRequired) {
		t.Fatalf("required = %v, want %v", schema.Required, wantRequired)
	}
	for _, name := range wantRequired {
		if !required[name] {
			t.Fatalf("required = %v, missing %s", schema.Required, name)
		}
	}

	for name, wantType := range wantTypes {
		prop, ok := schema.Properties[name]
		if !ok {
			t.Fatalf("schema properties missing %s: %s", name, raw)
		}
		if prop.Type != wantType {
			t.Fatalf("%s type = %q, want %q", name, prop.Type, wantType)
		}
		if required[name] {
			t.Fatalf("%s is required in schema: %s", name, raw)
		}
	}
}

func seedScopedConclusions(t *testing.T, ctx context.Context, svc *goncho.Service) {
	t.Helper()

	for _, item := range []struct {
		sessionKey string
		conclusion string
	}{
		{
			sessionKey: "telegram:6586915095",
			conclusion: "same-chat codename orchid",
		},
		{
			sessionKey: "discord:channel-9",
			conclusion: "other-chat codename orchid",
		},
	} {
		if _, err := svc.Conclude(ctx, goncho.ConcludeParams{
			Peer:       "telegram:6586915095",
			Conclusion: item.conclusion,
			SessionKey: item.sessionKey,
		}); err != nil {
			t.Fatalf("seed conclusion %q: %v", item.conclusion, err)
		}
	}
}

func executeHonchoTool(t *testing.T, reg *tools.Registry, toolName string, input json.RawMessage) json.RawMessage {
	t.Helper()

	ch, err := tools.NewInProcessToolExecutor(reg).Execute(context.Background(), tools.ToolRequest{
		ToolName: toolName,
		Input:    input,
	})
	if err != nil {
		t.Fatal(err)
	}

	var outputs []tools.ToolEvent
	for ev := range ch {
		outputs = append(outputs, ev)
	}
	if len(outputs) != 3 {
		t.Fatalf("event count = %d, want 3", len(outputs))
	}
	if outputs[1].Type != "output" {
		t.Fatalf("second event = %s, want output", outputs[1].Type)
	}
	return outputs[1].Output
}

func newTestHonchoRegistry(t *testing.T) (*tools.Registry, *goncho.Service, func()) {
	t.Helper()

	store, err := memory.OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}

	reg := tools.NewRegistry()
	svc := goncho.NewService(store.DB(), goncho.Config{
		WorkspaceID:    "default",
		ObserverPeerID: "gormes",
		RecentMessages: 4,
	}, nil)
	RegisterHonchoTools(reg, svc)

	return reg, svc, func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}
