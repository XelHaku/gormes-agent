package main

import (
	"context"
	"os/exec"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/skills"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/subagent"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

// buildDefaultRegistry returns a Registry populated with Gormes's built-in
// Go-native tools (echo, now, rand_int, execute_code). Consumer forks that
// want to add domain-specific tools (scientific simulators, business wrappers,
// etc.) call reg.Register on the returned *Registry before passing it into the
// kernel Config. Gormes itself ships no domain-specific tools.
func buildDefaultRegistry(parentCtx context.Context, delegation config.DelegationCfg, skillsRoot string, childClient hermes.Client, childModel string) *tools.Registry {
	reg := tools.NewRegistry()
	reg.MustRegisterEntry(tools.ToolEntry{Tool: &tools.EchoTool{}, Toolset: "core"})
	reg.MustRegisterEntry(tools.ToolEntry{Tool: &tools.NowTool{}, Toolset: "core"})
	reg.MustRegisterEntry(tools.ToolEntry{Tool: &tools.RandIntTool{}, Toolset: "core"})
	clarify := &tools.ClarifyTool{}
	reg.MustRegisterEntry(tools.ToolEntry{
		Tool:    clarify,
		Toolset: "clarify",
		CheckFn: func() bool { return clarify.Prompter != nil },
	})
	todo := &tools.TodoTool{}
	reg.MustRegisterEntry(tools.ToolEntry{
		Tool:    todo,
		Toolset: "todo",
		CheckFn: func() bool { return todo.Store != nil },
	})
	sessionSearch := &tools.SessionSearchTool{}
	reg.MustRegisterEntry(tools.ToolEntry{
		Tool:    sessionSearch,
		Toolset: "session_search",
		CheckFn: func() bool { return sessionSearch.Backend != nil },
	})
	reg.MustRegisterEntry(tools.ToolEntry{
		Tool:    &tools.ExecuteCodeTool{},
		Toolset: "code_execution",
		CheckFn: func() bool {
			_, err := exec.LookPath("go")
			return err == nil
		},
	})
	reg.MustRegisterEntry(tools.ToolEntry{
		Tool:    &tools.MixtureOfAgentsTool{},
		Toolset: "moa",
		CheckFn: tools.MixtureOfAgentsAvailable,
	})
	tools.RegisterBrowserTools(reg)
	if delegation.Enabled {
		var drafter subagent.CandidateDrafter
		if skillsRoot != "" {
			drafter = skillsCandidateDrafter{store: skills.NewStore(skillsRoot, 0)}
		}
		opts := subagent.ManagerOpts{
			ParentCtx:            parentCtx,
			ParentID:             "root",
			Depth:                0,
			Registry:             subagent.NewRegistry(),
			ToolExecutor:         tools.NewInProcessToolExecutor(reg),
			MaxDepth:             delegation.MaxDepth,
			DefaultMaxIterations: delegation.DefaultMaxIterations,
			DefaultMaxConcurrent: delegation.MaxConcurrentChildren,
			DefaultTimeout:       delegation.DefaultTimeout,
			RunLogPath:           delegation.ResolvedRunLogPath(),
			ToolAudit:            audit.NewJSONLWriter(config.ToolAuditLogPath()),
		}
		if childClient != nil {
			descs := registryDescriptors(reg)
			opts.NewRunner = func() subagent.Runner {
				runner := subagent.NewHermesRunner(childClient, childModel, descs)
				return runner
			}
		}
		reg.MustRegisterEntry(tools.ToolEntry{
			Tool:    subagent.NewDelegateTool(subagent.NewManager(opts), drafter),
			Toolset: "delegation",
		})
	}
	return reg
}

func registryDescriptors(reg *tools.Registry) []hermes.ToolDescriptor {
	descs := reg.AvailableDescriptors()
	out := make([]hermes.ToolDescriptor, len(descs))
	for i, d := range descs {
		out[i] = hermes.ToolDescriptor{Name: d.Name, Description: d.Description, Schema: d.Schema}
	}
	return out
}

type skillsCandidateDrafter struct {
	store *skills.Store
}

func (d skillsCandidateDrafter) DraftCandidate(_ context.Context, req subagent.CandidateDraftRequest) (string, error) {
	meta, err := d.store.DraftCandidate(skills.CandidateDraft{
		Slug:            req.Slug,
		Goal:            req.Goal,
		Summary:         req.Summary,
		SourceRunID:     req.SourceRunID,
		ParentSessionID: req.ParentSessionID,
		ChildAgentID:    req.ChildAgentID,
		ToolNames:       append([]string(nil), req.ToolNames...),
	})
	if err != nil {
		return "", err
	}
	return meta.CandidateID, nil
}
