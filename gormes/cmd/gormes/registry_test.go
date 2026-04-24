package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

func TestBuildDefaultRegistryDelegationDisabled(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{}, "", nil, "")
	if _, ok := reg.Get("delegate_task"); ok {
		t.Fatal("delegate_task unexpectedly registered")
	}
	if _, ok := reg.Get("execute_code"); !ok {
		t.Fatal("execute_code not registered")
	}
}

func TestBuildDefaultRegistryRegistersBrowserNavigate(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{}, "", nil, "")

	entry, ok := reg.Entry("browser_navigate")
	if !ok {
		t.Fatal("browser_navigate not registered")
	}
	if entry.Toolset != "browser" {
		t.Fatalf("browser_navigate toolset = %q, want browser", entry.Toolset)
	}
}

func TestBuildDefaultRegistryRegistersExecuteCode(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{}, "", nil, "")

	entry, ok := reg.Entry("execute_code")
	if !ok {
		t.Fatal("execute_code not registered")
	}
	if entry.Toolset != "code_execution" {
		t.Fatalf("execute_code toolset = %q, want code_execution", entry.Toolset)
	}
}

func TestBuildDefaultRegistryRegistersDormantClarify(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{}, "", nil, "")

	entry, ok := reg.Entry("clarify")
	if !ok {
		t.Fatal("clarify not registered")
	}
	if entry.Toolset != "clarify" {
		t.Fatalf("clarify toolset = %q, want clarify", entry.Toolset)
	}
	if containsToolDescriptor(reg.AvailableDescriptors(), "clarify") {
		t.Fatal("clarify unexpectedly available without prompter")
	}

	tool, ok := reg.Get("clarify")
	if !ok {
		t.Fatal("clarify not retrievable")
	}
	clarify, ok := tool.(*tools.ClarifyTool)
	if !ok {
		t.Fatalf("clarify tool type = %T, want *tools.ClarifyTool", tool)
	}
	clarify.Prompter = func(context.Context, tools.ClarifyRequest) (tools.ClarifyReply, error) {
		return tools.ClarifyReply{Answer: "Ship it."}, nil
	}

	if !containsToolDescriptor(reg.AvailableDescriptors(), "clarify") {
		t.Fatal("clarify unavailable after prompter injection")
	}
}

func TestBuildDefaultRegistryRegistersDormantSessionSearch(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{}, "", nil, "")

	entry, ok := reg.Entry("session_search")
	if !ok {
		t.Fatal("session_search not registered")
	}
	if entry.Toolset != "session_search" {
		t.Fatalf("session_search toolset = %q, want session_search", entry.Toolset)
	}
	if containsToolDescriptor(reg.AvailableDescriptors(), "session_search") {
		t.Fatal("session_search unexpectedly available without backend")
	}

	tool, ok := reg.Get("session_search")
	if !ok {
		t.Fatal("session_search not retrievable")
	}
	search, ok := tool.(*tools.SessionSearchTool)
	if !ok {
		t.Fatalf("session_search tool type = %T, want *tools.SessionSearchTool", tool)
	}
	search.Backend = stubSessionSearchBackend{}
	if !containsToolDescriptor(reg.AvailableDescriptors(), "session_search") {
		t.Fatal("session_search unavailable after backend injection")
	}
}

type stubSessionSearchBackend struct{}

func (stubSessionSearchBackend) Search(context.Context, tools.SessionSearchRequest) ([]tools.SessionSearchHit, error) {
	return nil, nil
}

func TestBuildDefaultRegistryRegistersMixtureOfAgents(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{}, "", nil, "")

	entry, ok := reg.Entry("mixture_of_agents")
	if !ok {
		t.Fatal("mixture_of_agents not registered")
	}
	if entry.Toolset != "moa" {
		t.Fatalf("mixture_of_agents toolset = %q, want moa", entry.Toolset)
	}
	if containsToolDescriptor(reg.AvailableDescriptors(), "mixture_of_agents") {
		t.Fatal("mixture_of_agents unexpectedly available without OPENROUTER_API_KEY")
	}

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	if !containsToolDescriptor(reg.AvailableDescriptors(), "mixture_of_agents") {
		t.Fatal("mixture_of_agents unavailable after OPENROUTER_API_KEY injection")
	}
}

func TestBuildDefaultRegistryDelegationEnabled(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{
		Enabled:               true,
		MaxDepth:              2,
		MaxConcurrentChildren: 4,
		DefaultMaxIterations:  9,
		DefaultTimeout:        time.Minute,
	}, "", nil, "")
	if _, ok := reg.Get("delegate_task"); !ok {
		t.Fatal("delegate_task not registered")
	}
}

func TestBuildDefaultRegistryDelegationToolExecutes(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{
		Enabled:               true,
		MaxDepth:              2,
		MaxConcurrentChildren: 3,
		DefaultMaxIterations:  50,
		DefaultTimeout:        time.Second,
	}, "", nil, "")

	tool, ok := reg.Get("delegate_task")
	if !ok {
		t.Fatal("delegate_task not registered")
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"audit runtime"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !bytes.Contains(out, []byte(`"status":"completed"`)) {
		t.Fatalf("output = %s, want completed status", out)
	}
}

func TestBuildDefaultRegistryDelegationToolAppendsRunLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")

	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{
		Enabled:               true,
		MaxDepth:              2,
		MaxConcurrentChildren: 3,
		DefaultMaxIterations:  50,
		DefaultTimeout:        time.Second,
		RunLogPath:            path,
	}, "", nil, "")

	tool, ok := reg.Get("delegate_task")
	if !ok {
		t.Fatal("delegate_task not registered")
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"audit runtime"}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if !bytes.Contains(raw, []byte(`"goal":"audit runtime"`)) {
		t.Fatalf("run log = %s, want goal field", raw)
	}
}

func TestBuildDefaultRegistryDelegationToolDraftsCandidate(t *testing.T) {
	root := t.TempDir()
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{
		Enabled:               true,
		MaxDepth:              2,
		MaxConcurrentChildren: 3,
		DefaultMaxIterations:  50,
		DefaultTimeout:        time.Second,
	}, root, nil, "")

	tool, ok := reg.Get("delegate_task")
	if !ok {
		t.Fatal("delegate_task not registered")
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"audit runtime","draft_candidate_slug":"audit-runtime","allow_no_tool_draft":true}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !bytes.Contains(out, []byte(`"candidate_id":"`)) {
		t.Fatalf("output = %s, want candidate_id", out)
	}

	entries, err := os.ReadDir(filepath.Join(root, "candidates"))
	if err != nil {
		t.Fatalf("ReadDir(candidates): %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("candidate dir count = %d, want 1", len(entries))
	}
	if _, err := os.Stat(filepath.Join(root, "candidates", entries[0].Name(), "SKILL.md")); err != nil {
		t.Fatalf("candidate SKILL.md missing: %v", err)
	}
}

func containsToolDescriptor(descs []tools.ToolDescriptor, name string) bool {
	for _, d := range descs {
		if d.Name == name {
			return true
		}
	}
	return false
}
