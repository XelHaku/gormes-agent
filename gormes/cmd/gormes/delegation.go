package main

import (
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/subagent"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

func registerDelegation(cfg config.Config, reg *tools.Registry, hc hermes.Client) *subagent.Manager {
	if reg == nil || !cfg.Delegation.Enabled {
		return nil
	}

	runner := subagent.NewChatRunner(hc, reg, subagent.ChatRunnerConfig{
		Model:           cfg.Hermes.Model,
		MaxToolDuration: 30 * time.Second,
	})
	mgr := subagent.NewManager(cfg.Delegation, runner, cfg.DelegationRunLogPath())
	reg.MustRegister(subagent.NewDelegateTool(mgr))
	return mgr
}
