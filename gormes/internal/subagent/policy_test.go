package subagent

import (
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

func TestValidateSpec_RejectsEmptyGoal(t *testing.T) {
	err := ValidateSpec(Spec{}, config.DelegationCfg{MaxChildDepth: 1})
	if err == nil {
		t.Fatal("ValidateSpec: want error for empty goal")
	}
}

func TestValidateSpec_RejectsWhitespaceGoal(t *testing.T) {
	err := ValidateSpec(Spec{Goal: "   ", MaxIterations: 1, Timeout: time.Second}, config.DelegationCfg{MaxChildDepth: 1})
	if err == nil {
		t.Fatal("ValidateSpec: want error for whitespace-only goal")
	}
}

func TestValidateSpec_DefaultsIterationsAndTimeout(t *testing.T) {
	spec, err := ApplyDefaults(Spec{Goal: "audit this"}, config.DelegationCfg{
		DefaultMaxIterations: 8,
		DefaultTimeout:       45 * time.Second,
		MaxChildDepth:        1,
	})
	if err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	if spec.MaxIterations != 8 {
		t.Errorf("MaxIterations = %d, want 8", spec.MaxIterations)
	}
	if spec.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v, want 45s", spec.Timeout)
	}
}

func TestApplyDefaults_RejectsNonPositiveTimeout(t *testing.T) {
	_, err := ApplyDefaults(Spec{Goal: "audit this"}, config.DelegationCfg{
		DefaultMaxIterations: 8,
		DefaultTimeout:       0,
		MaxChildDepth:        1,
	})
	if err == nil {
		t.Fatal("ApplyDefaults: want timeout validation error")
	}
}

func TestValidateSpec_RejectsDepthAboveLimit(t *testing.T) {
	_, err := ApplyDefaults(Spec{Goal: "x", Depth: 2}, config.DelegationCfg{MaxChildDepth: 1})
	if err == nil {
		t.Fatal("ApplyDefaults: want depth-limit error")
	}
}

func TestBlockedToolSet_ContainsDelegateTask(t *testing.T) {
	if !IsBlockedTool("delegate_task") {
		t.Fatal("delegate_task must be blocked to prevent recursion")
	}
}
