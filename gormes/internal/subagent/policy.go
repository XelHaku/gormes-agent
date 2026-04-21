package subagent

import (
	"fmt"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

var blockedTools = map[string]struct{}{
	"delegate_task": {},
}

func IsBlockedTool(name string) bool {
	_, ok := blockedTools[name]
	return ok
}

func ApplyDefaults(spec Spec, cfg config.DelegationCfg) (Spec, error) {
	spec.Goal = strings.TrimSpace(spec.Goal)
	spec.Context = strings.TrimSpace(spec.Context)
	spec.Model = strings.TrimSpace(spec.Model)
	if spec.MaxIterations <= 0 {
		spec.MaxIterations = cfg.DefaultMaxIterations
	}
	if spec.Timeout <= 0 {
		spec.Timeout = cfg.DefaultTimeout
	}
	return spec, ValidateSpec(spec, cfg)
}

func ValidateSpec(spec Spec, cfg config.DelegationCfg) error {
	if strings.TrimSpace(spec.Goal) == "" {
		return fmt.Errorf("subagent: empty goal")
	}
	if spec.MaxIterations <= 0 {
		return fmt.Errorf("subagent: max_iterations must be > 0")
	}
	if spec.Timeout <= 0 {
		return fmt.Errorf("subagent: timeout must be > 0")
	}
	if spec.Depth > cfg.MaxChildDepth {
		return fmt.Errorf("subagent: depth %d exceeds max %d", spec.Depth, cfg.MaxChildDepth)
	}
	return nil
}
