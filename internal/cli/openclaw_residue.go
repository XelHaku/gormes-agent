package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// DetectOpenClawResidue reports whether home contains an OpenClaw workspace
// directory. It is a best-effort filesystem probe only; stat errors degrade to
// false so CLI startup can continue.
func DetectOpenClawResidue(home string) bool {
	if strings.TrimSpace(home) == "" {
		return false
	}

	info, err := os.Stat(filepath.Join(home, ".openclaw"))
	if err != nil {
		return false
	}
	return info.IsDir()
}

// OpenClawResidueHint returns Gormes-facing guidance for an old OpenClaw
// workspace. The cleanup command is injected by callers so startup wiring can
// choose the command text without this helper registering or running it.
func OpenClawResidueHint(commandName string) string {
	cleanupCommand := strings.TrimSpace(commandName)
	if cleanupCommand == "" {
		cleanupCommand = "the Gormes OpenClaw cleanup command"
	}

	return "Heads up: an OpenClaw workspace was detected at ~/.openclaw/.\n" +
		"After migrating to Gormes, old OpenClaw config or memory there can still be picked up by mistake.\n" +
		"Run `" + cleanupCommand + "` to archive the old workspace when you are ready. This tip only shows once."
}
