package cli

import (
	"errors"
	"strings"
	"sync"
)

// ErrBusyCommandActive is returned by BusyCommandGuard.Run when another long-
// running command is already executing. Nested entry is rejected so the busy
// state remains a single source of truth and overlapping handlers cannot
// corrupt the guard's internal accounting.
var ErrBusyCommandActive = errors.New("busy command already active")

// ErrBusyCommandInvalid is returned by BusyCommandGuard.Run when the caller
// supplied an empty command name. An empty name would surface a meaningless
// status to operators and break parity with the CLI command registry.
var ErrBusyCommandInvalid = errors.New("busy command name must not be empty")

// BusyCommandGuard coordinates the "command-busy" input invariant: while a
// long-running CLI command (such as /compress or /reload-mcp) is running, new
// user input must not be accepted, except for slash commands declared with
// ActiveTurnPolicyBypass in CommandRegistry. The guard exposes both the entry
// point for command handlers (Run) and the read side for input dispatchers
// (IsBusy / Status / EvaluateInput).
//
// The zero value is not ready for use; callers must construct via
// NewBusyCommandGuard so the underlying mutex semantics are documented at the
// constructor.
type BusyCommandGuard struct {
	mu     sync.Mutex
	active *busyState
}

type busyState struct {
	name   string
	status string
}

// NewBusyCommandGuard returns an idle guard. Safe for concurrent use across
// the command-handler goroutine (which calls Run) and the input goroutine
// (which calls EvaluateInput / IsBusy / Status).
func NewBusyCommandGuard() *BusyCommandGuard {
	return &BusyCommandGuard{}
}

// Run wraps a long-running CLI command. name is the registry-aligned command
// identifier (e.g. "compress"); status is the operator-facing label rendered
// while fn executes ("Compressing context..."). Run sets command-busy state
// before invoking fn and clears it on every exit path — successful return,
// error, and panic — so the next user input is accepted as soon as the
// handler exits.
//
// If another command is already running, Run returns ErrBusyCommandActive
// without invoking fn. If name is empty, Run returns ErrBusyCommandInvalid.
func (g *BusyCommandGuard) Run(name, status string, fn func() error) error {
	if name == "" {
		return ErrBusyCommandInvalid
	}
	g.mu.Lock()
	if g.active != nil {
		g.mu.Unlock()
		return ErrBusyCommandActive
	}
	g.active = &busyState{name: name, status: status}
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		g.active = nil
		g.mu.Unlock()
	}()
	return fn()
}

// IsBusy reports whether a long-running command is currently executing.
func (g *BusyCommandGuard) IsBusy() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active != nil
}

// Status returns the operator-facing label of the active command, or an empty
// string when idle. The TUI/CLI render this in the status row instead of
// inventing a generic spinner message.
func (g *BusyCommandGuard) Status() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active == nil {
		return ""
	}
	return g.active.status
}

// ActiveCommand returns the registry-aligned command name currently running,
// or an empty string when idle. Useful for telemetry / structured logs that
// want to attribute rejections to the command that owned the busy window.
func (g *BusyCommandGuard) ActiveCommand() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active == nil {
		return ""
	}
	return g.active.name
}

// BusyInputVerdict is the result of BusyCommandGuard.EvaluateInput. When
// Rejected is true, the dispatcher must not forward input to the slash
// registry or the kernel; instead, it surfaces Evidence to the operator.
type BusyInputVerdict struct {
	Rejected bool
	Evidence string
}

// EvaluateInput reports whether the supplied editor text should be rejected
// because a long-running CLI command is currently executing. Idle guards
// always return Rejected=false so the dispatcher's normal slash/kernel paths
// run. Otherwise:
//
//   - Slash commands declared with ActiveTurnPolicyBypass in CommandRegistry
//     (e.g. /help, /stop, /restart) are allowed through so the operator can
//     observe state and abort the busy command.
//   - Anything else — plain text, recognized non-bypass slash commands, and
//     unknown slash text — is rejected with a single uniform busy notice.
func (g *BusyCommandGuard) EvaluateInput(input string) BusyInputVerdict {
	g.mu.Lock()
	active := g.active
	g.mu.Unlock()
	if active == nil {
		return BusyInputVerdict{}
	}
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "/") {
		if cmd, ok := ResolveCommandPolicy(trimmed); ok && cmd.ActiveTurnPolicy == ActiveTurnPolicyBypass {
			return BusyInputVerdict{}
		}
	}
	status := active.status
	if status == "" {
		status = "a long-running command is in progress"
	}
	return BusyInputVerdict{
		Rejected: true,
		Evidence: "Gormes is busy — " + status + " — wait for it to finish or send /stop",
	}
}
