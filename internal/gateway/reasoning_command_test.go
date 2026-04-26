package gateway

import (
	"errors"
	"testing"
)

func TestParseReasoningCommand_ShowFormReturnsActionShow(t *testing.T) {
	cmd, err := ParseReasoningCommand(nil)
	if err != nil {
		t.Fatalf("ParseReasoningCommand(nil) err = %v, want nil", err)
	}
	if cmd.Action != ReasoningActionShow {
		t.Fatalf("Action = %v, want ReasoningActionShow", cmd.Action)
	}
	if cmd.Effort != ReasoningEffort("") {
		t.Fatalf("Effort = %q, want empty", cmd.Effort)
	}
	if cmd.Global {
		t.Fatalf("Global = true, want false")
	}

	cmd, err = ParseReasoningCommand([]string{})
	if err != nil {
		t.Fatalf("ParseReasoningCommand([]) err = %v, want nil", err)
	}
	if cmd.Action != ReasoningActionShow {
		t.Fatalf("Action = %v, want ReasoningActionShow", cmd.Action)
	}
}

func TestParseReasoningCommand_SetSessionScoped(t *testing.T) {
	for _, effort := range []string{"high", "low", "medium"} {
		t.Run(effort, func(t *testing.T) {
			cmd, err := ParseReasoningCommand([]string{effort})
			if err != nil {
				t.Fatalf("ParseReasoningCommand([%q]) err = %v, want nil", effort, err)
			}
			if cmd.Action != ReasoningActionSet {
				t.Fatalf("Action = %v, want ReasoningActionSet", cmd.Action)
			}
			if cmd.Effort != ReasoningEffort(effort) {
				t.Fatalf("Effort = %q, want %q", cmd.Effort, effort)
			}
			if cmd.Global {
				t.Fatalf("Global = true, want false")
			}
		})
	}
}

func TestParseReasoningCommand_SetGlobal(t *testing.T) {
	cmd, err := ParseReasoningCommand([]string{"low", "--global"})
	if err != nil {
		t.Fatalf("ParseReasoningCommand([low --global]) err = %v, want nil", err)
	}
	if cmd.Action != ReasoningActionSet {
		t.Fatalf("Action = %v, want ReasoningActionSet", cmd.Action)
	}
	if cmd.Effort != ReasoningEffort("low") {
		t.Fatalf("Effort = %q, want low", cmd.Effort)
	}
	if !cmd.Global {
		t.Fatalf("Global = false, want true")
	}
}

func TestParseReasoningCommand_ResetSession(t *testing.T) {
	cmd, err := ParseReasoningCommand([]string{"reset"})
	if err != nil {
		t.Fatalf("ParseReasoningCommand([reset]) err = %v, want nil", err)
	}
	if cmd.Action != ReasoningActionReset {
		t.Fatalf("Action = %v, want ReasoningActionReset", cmd.Action)
	}
	if cmd.Global {
		t.Fatalf("Global = true, want false")
	}
	if cmd.Effort != ReasoningEffort("") {
		t.Fatalf("Effort = %q, want empty", cmd.Effort)
	}
}

func TestParseReasoningCommand_RejectGlobalReset(t *testing.T) {
	_, err := ParseReasoningCommand([]string{"reset", "--global"})
	if err == nil {
		t.Fatalf("ParseReasoningCommand([reset --global]) err = nil, want ErrResetGlobalUnsupported")
	}
	if !errors.Is(err, ErrResetGlobalUnsupported) {
		t.Fatalf("err = %v, want ErrResetGlobalUnsupported", err)
	}
}

func TestParseReasoningCommand_RejectInvalidEffort(t *testing.T) {
	_, err := ParseReasoningCommand([]string{"bogus"})
	if err == nil {
		t.Fatalf("ParseReasoningCommand([bogus]) err = nil, want ErrInvalidEffort")
	}
	if !errors.Is(err, ErrInvalidEffort) {
		t.Fatalf("err = %v, want ErrInvalidEffort", err)
	}
}
