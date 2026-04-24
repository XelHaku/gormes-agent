package autoloop

import "fmt"

func BuildBackendCommand(backend, mode string) ([]string, error) {
	if backend == "" {
		backend = "codexu"
	}
	if mode == "" {
		mode = "safe"
	}

	sandbox, err := sandboxForMode(mode)
	if err != nil {
		return nil, err
	}

	switch backend {
	case "codexu":
		return []string{"codexu", "exec", "--json", "--ephemeral", "-m", "gpt-5.5", "-c", "approval_policy=never", "--sandbox", sandbox}, nil
	case "claudeu":
		return []string{"claudeu", "exec", "--json", "-m", "gpt-5.5", "-c", "approval_policy=never", "--sandbox", sandbox}, nil
	case "opencode":
		return []string{"opencode", "run", "--no-interactive"}, nil
	default:
		return nil, fmt.Errorf("invalid BACKEND %q: expected codexu, claudeu, or opencode", backend)
	}
}

func sandboxForMode(mode string) (string, error) {
	switch mode {
	case "safe", "unattended":
		return "workspace-write", nil
	case "full":
		return "danger-full-access", nil
	default:
		return "", fmt.Errorf("invalid MODE %q: expected safe, unattended, or full", mode)
	}
}
