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
	reg := buildDefaultRegistry(context.Background(), config.DelegationCfg{})
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
	})
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
	})

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
	})

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
