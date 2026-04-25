package subagent

import (
	"errors"
	"fmt"
)

// ErrDurableRouteDenied is returned when an untrusted caller attempts to
// submit privileged deterministic work through the durable-job route.
var ErrDurableRouteDenied = errors.New("subagent: durable route denied")

// TrustClass identifies who is asking the orchestration policy to submit work.
type TrustClass string

const (
	TrustOperator   TrustClass = "operator"
	TrustChildAgent TrustClass = "child-agent"
	TrustSystem     TrustClass = "system"
)

// WorkKind is the coarse class of work being routed.
type WorkKind string

const (
	WorkKindShellCommand WorkKind = "shell_command"
	WorkKindCronJob      WorkKind = "cron_job"
	WorkKindLLMSubagent  WorkKind = "llm_subagent"
)

// OrchestrationRoute is the execution route chosen by the policy.
type OrchestrationRoute string

const (
	RouteDurableJob   OrchestrationRoute = "durable_job"
	RouteLiveSubagent OrchestrationRoute = "live_subagent"
	RouteDenied       OrchestrationRoute = "denied"
)

// OrchestrationLane keeps deterministic restartable work distinct from live
// LLM judgment loops while still reporting both through one control surface.
type OrchestrationLane string

const (
	LaneDeterministic OrchestrationLane = "deterministic"
	LaneLLMSubagent   OrchestrationLane = "llm_subagent"
)

// ExecutionAPI names the Gormes-native API the selected route should use.
type ExecutionAPI string

const (
	ExecutionAPIDurableJob   ExecutionAPI = "durable_job"
	ExecutionAPIDelegateTask ExecutionAPI = "delegate_task"
)

// ControlPlane names the operator-visible orchestration surface shared by
// deterministic durable jobs and live subagent runs.
type ControlPlane string

const (
	ControlPlaneUnifiedOrchestrator ControlPlane = "unified_orchestrator"
)

// MinionRoutingRequest is a pure policy input. It intentionally carries no
// queue/executor handles; this slice only decides where work belongs.
type MinionRoutingRequest struct {
	Kind               WorkKind
	Trust              TrustClass
	Deterministic      bool
	RestartSurvivable  bool
	JudgmentHeavy      bool
	NeedsObservability bool
}

// MinionRoutingDecision describes the selected orchestration lane.
type MinionRoutingDecision struct {
	Route            OrchestrationRoute
	Lane             OrchestrationLane
	ExecutionAPI     ExecutionAPI
	ControlPlane     ControlPlane
	Durable          bool
	PrivilegedSubmit bool
	Allowed          bool
	Reason           string
}

// MinionRoutingPolicy holds the trust matrix for the borrowed GBrain routing
// policy while preserving Gormes-native delegate_task/subagent execution APIs.
type MinionRoutingPolicy struct{}

// DefaultMinionRoutingPolicy returns the built-in routing policy.
func DefaultMinionRoutingPolicy() MinionRoutingPolicy {
	return MinionRoutingPolicy{}
}

// CanSubmit reports whether a caller may submit a work kind on its route.
func (MinionRoutingPolicy) CanSubmit(trust TrustClass, kind WorkKind) bool {
	switch kind {
	case WorkKindShellCommand, WorkKindCronJob:
		return trust == TrustOperator || trust == TrustSystem
	case WorkKindLLMSubagent:
		return trust == TrustOperator || trust == TrustSystem
	default:
		return false
	}
}

// Route classifies work into the smallest policy surface needed for this
// phase: deterministic shell/cron-like work takes the durable-job lane, while
// judgment-heavy LLM work remains a live Go-native delegate_task subagent.
func (p MinionRoutingPolicy) Route(req MinionRoutingRequest) (MinionRoutingDecision, error) {
	switch req.Kind {
	case WorkKindShellCommand, WorkKindCronJob:
		return p.routeDeterministic(req)
	case WorkKindLLMSubagent:
		return p.routeLLMSubagent(req)
	default:
		return MinionRoutingDecision{
			Route:        RouteDenied,
			ControlPlane: ControlPlaneUnifiedOrchestrator,
			Allowed:      false,
			Reason:       "unknown work kind",
		}, fmt.Errorf("%w: unknown work kind %q", ErrDurableRouteDenied, req.Kind)
	}
}

func (p MinionRoutingPolicy) routeDeterministic(req MinionRoutingRequest) (MinionRoutingDecision, error) {
	decision := MinionRoutingDecision{
		Route:            RouteDurableJob,
		Lane:             LaneDeterministic,
		ExecutionAPI:     ExecutionAPIDurableJob,
		ControlPlane:     ControlPlaneUnifiedOrchestrator,
		Durable:          true,
		PrivilegedSubmit: true,
		Allowed:          p.CanSubmit(req.Trust, req.Kind),
		Reason:           "deterministic restart-survivable work uses durable orchestration",
	}
	if !decision.Allowed {
		decision.Route = RouteDenied
		return decision, fmt.Errorf("%w: %s cannot submit %s", ErrDurableRouteDenied, req.Trust, req.Kind)
	}
	return decision, nil
}

func (p MinionRoutingPolicy) routeLLMSubagent(req MinionRoutingRequest) (MinionRoutingDecision, error) {
	decision := MinionRoutingDecision{
		Route:        RouteLiveSubagent,
		Lane:         LaneLLMSubagent,
		ExecutionAPI: ExecutionAPIDelegateTask,
		ControlPlane: ControlPlaneUnifiedOrchestrator,
		Durable:      false,
		Allowed:      p.CanSubmit(req.Trust, req.Kind),
		Reason:       "judgment-heavy LLM work stays on the live Go-native subagent route",
	}
	if !decision.Allowed {
		decision.Route = RouteDenied
		return decision, fmt.Errorf("%w: %s cannot submit %s", ErrDurableRouteDenied, req.Trust, req.Kind)
	}
	return decision, nil
}
