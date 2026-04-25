package autoloop

import "fmt"

// workerOutcome is the loop's internal classification of a worker's exit.
// IsBackendErrorFlag is true ONLY when the worker produced no commit AND
// no diff AND exited with worker_error (i.e. infra failed, not the row).
type workerOutcome struct {
	IsSuccessFlag      bool
	IsBackendErrorFlag bool
	Commit             string
	DiffLines          int
	Category           string
	Backend            string
}

func (w workerOutcome) IsSuccess() bool      { return w.IsSuccessFlag }
func (w workerOutcome) IsBackendError() bool { return w.IsBackendErrorFlag }

// backendDegrader switches the run loop to the next backend in chain after
// the configured threshold of consecutive backend errors. The chain is
// closed: once degraded past the last entry, further backend errors do not
// trigger another switch.
type backendDegrader struct {
	chain                    []string
	current                  string
	consecutiveBackendErrors int
	threshold                int
	degraded                 bool
}

func newBackendDegrader(chain []string, threshold int) *backendDegrader {
	if threshold <= 0 {
		threshold = 3
	}
	if len(chain) == 0 {
		return &backendDegrader{threshold: threshold}
	}
	return &backendDegrader{
		chain:     chain,
		current:   chain[0],
		threshold: threshold,
	}
}

func (d *backendDegrader) Current() string { return d.current }

// ObserveOutcome takes one worker outcome and possibly switches to the next
// backend in chain. Returns (switched, from, to) — when switched is false,
// from and to are empty strings.
func (d *backendDegrader) ObserveOutcome(out workerOutcome) (switched bool, from, to string) {
	if out.IsSuccess() {
		d.consecutiveBackendErrors = 0
		return false, "", ""
	}
	if !out.IsBackendError() {
		return false, "", ""
	}
	// No chain configured: nothing to degrade to. Stay quiet.
	if len(d.chain) == 0 {
		return false, "", ""
	}
	d.consecutiveBackendErrors++
	if d.consecutiveBackendErrors < d.threshold {
		return false, "", ""
	}
	if d.degraded {
		return false, "", ""
	}
	idx := indexOfString(d.chain, d.current)
	if idx < 0 || idx+1 >= len(d.chain) {
		// No further fallback available; mark degraded so we don't keep
		// counting (further backend errors will fail rows individually).
		d.degraded = true
		return false, "", ""
	}
	previous := d.current
	d.current = d.chain[idx+1]
	d.degraded = true
	return true, previous, d.current
}

func indexOfString(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}

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
