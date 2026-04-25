package main

import (
	"fmt"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progressctl"
)

// runProgress dispatches to internal/progressctl. The progress logic itself
// lives there now so cmd/progress and cmd/builder-loop can share it; this
// shim keeps `builder-loop progress {validate|write}` working for operators
// who still type the long form.
func runProgress(deps cliDeps, root string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w\n%s", errParse, subUsage["progress"])
	}

	switch args[0] {
	case "validate":
		format, err := parseFormat(args[1:], "progress validate")
		if err != nil {
			return err
		}
		return progressctl.Validate(deps.stdout, root, format)
	case "write":
		if len(args) != 1 {
			return fmt.Errorf("%w\n%s", errParse, subUsage["progress"])
		}
		return progressctl.Write(deps.stdout, root)
	default:
		return fmt.Errorf("%w\n%s", errParse, subUsage["progress"])
	}
}

