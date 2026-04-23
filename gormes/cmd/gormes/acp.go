package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/acp"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

var acpCmd = &cobra.Command{
	Use:   "acp",
	Short: "Run Gormes as an ACP stdio server",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runACP(cmd.Context())
	},
}

func runACP(ctx context.Context) error {
	cfg, err := config.Load(nil)
	if err != nil {
		return err
	}

	skillsRuntime := configuredSkillsRuntime(cfg)
	learningRuntime := configuredLearningRuntime(cfg)
	factory := acp.NewKernelSessionFactory(acp.KernelSessionFactoryOptions{
		Model:    cfg.Hermes.Model,
		Endpoint: hermesEndpoint(cfg),
		ClientFactory: func() hermes.Client {
			client, _ := newLLMClient(cfg)
			return client
		},
		RegistryFactory: func(childClient hermes.Client) *tools.Registry {
			return buildDefaultRegistry(ctx, cfg.Delegation, cfg.SkillsRoot(), childClient, cfg.Hermes.Model)
		},
		ModelRouting: smartModelRouting(cfg),
		Skills:       skillsRuntime,
		SkillUsage:   skillsRuntime,
		Learning:     learningRuntime,
	})

	server := acp.NewServer(acp.Options{
		AgentInfo:  acp.Implementation{Name: "gormes", Title: "Gormes", Version: Version},
		NewSession: factory.NewSession,
	})
	return server.Serve(ctx, os.Stdin, os.Stdout)
}

func hermesEndpoint(cfg config.Config) string {
	_, endpoint := newLLMClient(cfg)
	return endpoint
}
