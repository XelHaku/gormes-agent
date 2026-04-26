package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// SlashResult is the typed return value of a SlashHandler. It tells Update
// whether the input was consumed (Handled), what status line to show
// (StatusMessage), and any tea.Cmd to schedule alongside the editor reset.
//
// When Handled is false, Update MUST forward the input to kernel.Submit so a
// buggy handler can never silently drop a turn.
type SlashResult struct {
	Handled       bool
	StatusMessage string
	Cmd           tea.Cmd
}

// SlashHandler implements one slash command. It receives the full editor
// input (including the leading slash) so it can parse subcommands without
// the registry having to split arguments. It receives *Model so it can
// read local TUI state and, for handlers like /mouse, mutate it directly
// before returning the resulting Cmd.
type SlashHandler func(input string, model *Model) SlashResult

// SlashRegistry routes slash commands typed in the editor to handlers.
// Construction is the only writer; Dispatch is read-only over the map, so
// no synchronization is required as long as Register is called before the
// Bubble Tea program starts.
type SlashRegistry struct {
	handlers map[string]SlashHandler
}

// NewSlashRegistry returns an empty registry. Most callers want
// NewDefaultSlashRegistry, which pre-registers /mouse, /scroll, and the
// /save stub.
func NewSlashRegistry() *SlashRegistry {
	return &SlashRegistry{handlers: make(map[string]SlashHandler)}
}

// Register binds a handler to a slash command name. Names are stored
// case-insensitively and without the leading "/" so callers may pass either
// "save" or "/save".
func (r *SlashRegistry) Register(name string, handler SlashHandler) {
	r.handlers[normalizeSlashName(name)] = handler
}

// Dispatch parses the first whitespace-separated token of input. If it
// starts with "/" and matches a registered handler, the handler runs with
// the full original input string. Otherwise, an empty (Handled=false)
// SlashResult is returned and the caller MUST treat input as a normal
// kernel turn.
func (r *SlashRegistry) Dispatch(input string, model *Model) SlashResult {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return SlashResult{}
	}
	first := fields[0]
	if !strings.HasPrefix(first, "/") {
		return SlashResult{}
	}
	handler, ok := r.handlers[normalizeSlashName(first)]
	if !ok {
		return SlashResult{}
	}
	return handler(input, model)
}

func normalizeSlashName(name string) string {
	return strings.ToLower(strings.TrimPrefix(name, "/"))
}

// NewDefaultSlashRegistry returns a registry pre-populated with the slash
// commands the Gormes TUI ships today: /mouse and its /scroll alias,
// plus the /save no-op stub that downstream rows replace with the real
// session export.
func NewDefaultSlashRegistry() *SlashRegistry {
	r := NewSlashRegistry()
	r.Register("mouse", mouseSlashHandler)
	r.Register("scroll", mouseSlashHandler)
	r.Register("save", saveStubHandler)
	return r
}

// mouseSlashHandler adapts the existing parseMouseTrackingSlash result into
// the SlashResult shape, mutating the model's mouseTracking field and
// emitting the terminal-mode Cmd only when the requested state differs from
// the current one, preserving the dedup behavior asserted by
// TestMouseSlashUpdatesRuntimeWithoutSubmitting.
func mouseSlashHandler(input string, model *Model) SlashResult {
	parsed := parseMouseTrackingSlash(input, model.mouseTracking)
	if !parsed.handled {
		return SlashResult{}
	}
	if !parsed.valid {
		return SlashResult{Handled: true, StatusMessage: parsed.message}
	}

	statusMessage := "mouse tracking on"
	if !parsed.next {
		statusMessage = "mouse tracking off"
	}

	var cmd tea.Cmd
	if parsed.next != model.mouseTracking {
		model.mouseTracking = parsed.next
		cmd = model.emitMouseModeCmd(parsed.next)
	}
	return SlashResult{Handled: true, StatusMessage: statusMessage, Cmd: cmd}
}

// saveStubHandler is the placeholder for /save. The follow-up progress row
// "Native TUI /save canonical session export" replaces this stub with the
// real export call; until then we explicitly tell the user the command was
// recognized but not yet wired.
func saveStubHandler(input string, model *Model) SlashResult {
	return SlashResult{
		Handled:       true,
		StatusMessage: "save not yet implemented",
	}
}
