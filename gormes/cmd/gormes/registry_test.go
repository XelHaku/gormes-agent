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
)

func TestBuildDefaultRegistryDelegationDisabled(t *testing.T) {
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{}, "", nil, "")
	if _, ok := reg.Get("delegate_task"); ok {
		t.Fatal("delegate_task unexpectedly registered")
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
