package main

import (
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/subagent"
)

func TestRegisterDelegation_DisabledLeavesRegistryUnchanged(t *testing.T) {
	reg := buildDefaultRegistry()
	before := len(reg.Descriptors())

	if got := registerDelegation(config.Config{}, reg, hermes.NewMockClient()); got != nil {
		t.Fatalf("registerDelegation() = %v, want nil", got)
	}

	after := len(reg.Descriptors())
	if after != before {
		t.Fatalf("registry size = %d, want %d", after, before)
	}
	if _, ok := reg.Get("delegate_task"); ok {
		t.Fatal("delegate_task should not be registered when delegation is disabled")
	}
}

func TestRegisterDelegation_EnabledRegistersTool(t *testing.T) {
	reg := buildDefaultRegistry()

	cfg := config.Config{
		Hermes: config.HermesCfg{Model: "hermes-agent"},
		Delegation: config.DelegationCfg{
			Enabled:              true,
			DefaultMaxIterations: 8,
			DefaultTimeout:       45 * time.Second,
			MaxChildDepth:        1,
			RunLogPath:           t.TempDir() + "/runs.jsonl",
		},
	}

	got := registerDelegation(cfg, reg, hermes.NewMockClient())
	if got == nil {
		t.Fatal("registerDelegation() = nil, want manager")
	}

	tool, ok := reg.Get("delegate_task")
	if !ok {
		t.Fatal("delegate_task was not registered")
	}
	if _, ok := tool.(*subagent.DelegateTool); !ok {
		t.Fatalf("tool type = %T, want *subagent.DelegateTool", tool)
	}
}

func TestRegisterDelegation_InvalidDefaultTimeoutLeavesRegistryUnchanged(t *testing.T) {
	reg := buildDefaultRegistry()
	before := len(reg.Descriptors())

	cfg := config.Config{
		Hermes: config.HermesCfg{Model: "hermes-agent"},
		Delegation: config.DelegationCfg{
			Enabled:              true,
			DefaultMaxIterations: 8,
			DefaultTimeout:       111 * time.Second,
			MaxChildDepth:        1,
			RunLogPath:           t.TempDir() + "/runs.jsonl",
		},
	}

	if got := registerDelegation(cfg, reg, hermes.NewMockClient()); got != nil {
		t.Fatalf("registerDelegation() = %v, want nil", got)
	}

	after := len(reg.Descriptors())
	if after != before {
		t.Fatalf("registry size = %d, want %d", after, before)
	}
	if _, ok := reg.Get("delegate_task"); ok {
		t.Fatal("delegate_task should not be registered when default timeout is invalid")
	}
}

func TestRegisterDelegation_InvalidDefaultIterationsLeavesRegistryUnchanged(t *testing.T) {
	reg := buildDefaultRegistry()
	before := len(reg.Descriptors())

	cfg := config.Config{
		Hermes: config.HermesCfg{Model: "hermes-agent"},
		Delegation: config.DelegationCfg{
			Enabled:              true,
			DefaultMaxIterations: 0,
			DefaultTimeout:       45 * time.Second,
			MaxChildDepth:        1,
			RunLogPath:           t.TempDir() + "/runs.jsonl",
		},
	}

	if got := registerDelegation(cfg, reg, hermes.NewMockClient()); got != nil {
		t.Fatalf("registerDelegation() = %v, want nil", got)
	}

	after := len(reg.Descriptors())
	if after != before {
		t.Fatalf("registry size = %d, want %d", after, before)
	}
	if _, ok := reg.Get("delegate_task"); ok {
		t.Fatal("delegate_task should not be registered when default iterations are invalid")
	}
}

func TestRegisterDelegation_InvalidMaxChildDepthLeavesRegistryUnchanged(t *testing.T) {
	reg := buildDefaultRegistry()
	before := len(reg.Descriptors())

	cfg := config.Config{
		Hermes: config.HermesCfg{Model: "hermes-agent"},
		Delegation: config.DelegationCfg{
			Enabled:              true,
			DefaultMaxIterations: 8,
			DefaultTimeout:       45 * time.Second,
			MaxChildDepth:        0,
			RunLogPath:           t.TempDir() + "/runs.jsonl",
		},
	}

	if got := registerDelegation(cfg, reg, hermes.NewMockClient()); got != nil {
		t.Fatalf("registerDelegation() = %v, want nil", got)
	}

	after := len(reg.Descriptors())
	if after != before {
		t.Fatalf("registry size = %d, want %d", after, before)
	}
	if _, ok := reg.Get("delegate_task"); ok {
		t.Fatal("delegate_task should not be registered when max child depth is invalid")
	}
}
