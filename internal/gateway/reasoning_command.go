package gateway

import (
	"errors"
	"fmt"
)

// ReasoningAction enumerates the parsed forms of the gateway /reasoning
// command. The parser is pure: state, persistence, and dispatch live in the
// follow-up apply/dispatch slice.
type ReasoningAction int

const (
	// ReasoningActionShow corresponds to /reasoning with no arguments.
	ReasoningActionShow ReasoningAction = iota
	// ReasoningActionSet corresponds to /reasoning <effort> [--global].
	ReasoningActionSet
	// ReasoningActionReset corresponds to /reasoning reset.
	ReasoningActionReset
)

// ReasoningEffort is the validated effort level recognized by the parser.
// The empty value represents "no effort selected" for non-Set actions.
type ReasoningEffort string

const (
	ReasoningEffortHigh   ReasoningEffort = "high"
	ReasoningEffortLow    ReasoningEffort = "low"
	ReasoningEffortMedium ReasoningEffort = "medium"
)

// ReasoningCommand is the parsed shape of a /reasoning invocation.
type ReasoningCommand struct {
	Action ReasoningAction
	Effort ReasoningEffort
	Global bool
}

// ErrInvalidEffort is returned when the user supplies an effort token outside
// the supported set (high|low|medium). The dispatcher renders this as the
// upstream "unknown argument" warning class.
var ErrInvalidEffort = errors.New("reasoning: invalid effort")

// ErrResetGlobalUnsupported is returned when "reset" is combined with
// "--global". The dispatcher surfaces this verbatim because the upstream
// gateway rejects this combination too.
var ErrResetGlobalUnsupported = errors.New("reasoning: reset --global unsupported")

// ParseReasoningCommand turns the raw split arguments of /reasoning into a
// typed ReasoningCommand. It is pure: no I/O, no clock, no state.
func ParseReasoningCommand(args []string) (ReasoningCommand, error) {
	global := false
	tokens := make([]string, 0, len(args))
	for _, raw := range args {
		if raw == "--global" {
			global = true
			continue
		}
		tokens = append(tokens, raw)
	}

	if len(tokens) == 0 && !global {
		return ReasoningCommand{Action: ReasoningActionShow}, nil
	}

	if len(tokens) == 0 {
		return ReasoningCommand{}, fmt.Errorf("%w: missing argument", ErrInvalidEffort)
	}

	if len(tokens) > 1 {
		return ReasoningCommand{}, fmt.Errorf("%w: %q", ErrInvalidEffort, tokens)
	}

	switch tokens[0] {
	case "reset":
		if global {
			return ReasoningCommand{}, ErrResetGlobalUnsupported
		}
		return ReasoningCommand{Action: ReasoningActionReset}, nil
	case string(ReasoningEffortHigh), string(ReasoningEffortLow), string(ReasoningEffortMedium):
		return ReasoningCommand{
			Action: ReasoningActionSet,
			Effort: ReasoningEffort(tokens[0]),
			Global: global,
		}, nil
	default:
		return ReasoningCommand{}, fmt.Errorf("%w: %q", ErrInvalidEffort, tokens[0])
	}
}
