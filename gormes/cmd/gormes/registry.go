package main

import (
	"context"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/subagent"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

// buildDefaultRegistry returns a Registry populated with Gormes's built-in
// Go-native tools (echo, now, rand_int). Consumer forks that want to add
// domain-specific tools (scientific simulators, business wrappers, etc.)
// call reg.Register on the returned *Registry before passing it into the
// kernel Config. Gormes itself ships no domain-specific tools.
func buildDefaultRegistry(parentCtx context.Context, delegation config.DelegationCfg) *tools.Registry {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	reg.MustRegister(&tools.NowTool{})
	reg.MustRegister(&tools.RandIntTool{})
	if delegation.Enabled {
		reg.MustRegister(subagent.NewDelegateTool(subagent.NewManager(subagent.ManagerOpts{
			ParentCtx:            parentCtx,
			ParentID:             "root",
			Depth:                0,
			Registry:             subagent.NewRegistry(),
			MaxDepth:             delegation.MaxDepth,
			DefaultMaxIterations: delegation.DefaultMaxIterations,
			DefaultMaxConcurrent: delegation.MaxConcurrentChildren,
			DefaultTimeout:       delegation.DefaultTimeout,
			RunLogPath:           delegation.ResolvedRunLogPath(),
		})))
	}
	return reg
}
