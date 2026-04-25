package subagent

import (
	"errors"
	"strings"
	"testing"
)

func TestMinionPolicyRoutesDeterministicWorkToDurableLane(t *testing.T) {
	policy := DefaultMinionRoutingPolicy()

	tests := []struct {
		name string
		req  MinionRoutingRequest
	}{
		{
			name: "operator shell command",
			req: MinionRoutingRequest{
				Kind:               WorkKindShellCommand,
				Trust:              TrustOperator,
				Deterministic:      true,
				RestartSurvivable:  true,
				NeedsObservability: true,
			},
		},
		{
			name: "system cron job",
			req: MinionRoutingRequest{
				Kind:               WorkKindCronJob,
				Trust:              TrustSystem,
				Deterministic:      true,
				RestartSurvivable:  true,
				NeedsObservability: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := policy.Route(tt.req)
			if err != nil {
				t.Fatalf("Route returned error: %v", err)
			}
			if decision.Route != RouteDurableJob {
				t.Errorf("Route = %q, want %q", decision.Route, RouteDurableJob)
			}
			if decision.Lane != LaneDeterministic {
				t.Errorf("Lane = %q, want %q", decision.Lane, LaneDeterministic)
			}
			if !decision.Durable {
				t.Error("Durable = false, want true")
			}
			if !decision.PrivilegedSubmit {
				t.Error("PrivilegedSubmit = false, want true")
			}
			if decision.ExecutionAPI != ExecutionAPIDurableJob {
				t.Errorf("ExecutionAPI = %q, want %q", decision.ExecutionAPI, ExecutionAPIDurableJob)
			}
			if decision.ControlPlane != ControlPlaneUnifiedOrchestrator {
				t.Errorf("ControlPlane = %q, want %q", decision.ControlPlane, ControlPlaneUnifiedOrchestrator)
			}
		})
	}
}

func TestMinionPolicyKeepsLLMSubagentsLiveAndGoNative(t *testing.T) {
	policy := DefaultMinionRoutingPolicy()

	decision, err := policy.Route(MinionRoutingRequest{
		Kind:               WorkKindLLMSubagent,
		Trust:              TrustOperator,
		JudgmentHeavy:      true,
		NeedsObservability: true,
	})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}

	if decision.Route != RouteLiveSubagent {
		t.Errorf("Route = %q, want %q", decision.Route, RouteLiveSubagent)
	}
	if decision.Lane != LaneLLMSubagent {
		t.Errorf("Lane = %q, want %q", decision.Lane, LaneLLMSubagent)
	}
	if decision.Durable {
		t.Error("Durable = true, want false for live LLM subagents in this slice")
	}
	if decision.ExecutionAPI != ExecutionAPIDelegateTask {
		t.Errorf("ExecutionAPI = %q, want %q", decision.ExecutionAPI, ExecutionAPIDelegateTask)
	}
	if got := NewDelegateTool(nil, nil).Name(); got != string(ExecutionAPIDelegateTask) {
		t.Errorf("DelegateTool.Name() = %q, want %q", got, ExecutionAPIDelegateTask)
	}
	if strings.Contains(strings.ToLower(string(decision.ExecutionAPI)), "minion") {
		t.Errorf("ExecutionAPI = %q, must remain Go-native and not be renamed to Minions", decision.ExecutionAPI)
	}
}

func TestMinionPolicyRejectsChildAgentShellDurableSubmit(t *testing.T) {
	policy := DefaultMinionRoutingPolicy()

	decision, err := policy.Route(MinionRoutingRequest{
		Kind:              WorkKindShellCommand,
		Trust:             TrustChildAgent,
		Deterministic:     true,
		RestartSurvivable: true,
	})
	if !errors.Is(err, ErrDurableRouteDenied) {
		t.Fatalf("Route error = %v, want ErrDurableRouteDenied", err)
	}
	if decision.Route != RouteDenied {
		t.Errorf("Route = %q, want %q", decision.Route, RouteDenied)
	}
	if decision.Lane != LaneDeterministic {
		t.Errorf("Lane = %q, want %q", decision.Lane, LaneDeterministic)
	}
	if decision.Allowed {
		t.Error("Allowed = true, want false")
	}
}

func TestMinionPolicyTrustMatrixDocumentsSubmitBoundary(t *testing.T) {
	policy := DefaultMinionRoutingPolicy()

	tests := []struct {
		trust TrustClass
		kind  WorkKind
		want  bool
	}{
		{trust: TrustOperator, kind: WorkKindShellCommand, want: true},
		{trust: TrustSystem, kind: WorkKindShellCommand, want: true},
		{trust: TrustChildAgent, kind: WorkKindShellCommand, want: false},
		{trust: TrustOperator, kind: WorkKindCronJob, want: true},
		{trust: TrustSystem, kind: WorkKindCronJob, want: true},
		{trust: TrustChildAgent, kind: WorkKindCronJob, want: false},
		{trust: TrustOperator, kind: WorkKindLLMSubagent, want: true},
		{trust: TrustChildAgent, kind: WorkKindLLMSubagent, want: false},
	}

	for _, tt := range tests {
		if got := policy.CanSubmit(tt.trust, tt.kind); got != tt.want {
			t.Errorf("CanSubmit(%q, %q) = %v, want %v", tt.trust, tt.kind, got, tt.want)
		}
	}
}
