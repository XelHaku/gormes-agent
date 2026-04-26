// Package tui renders the Gormes Dashboard. The TUI is a pure consumer of
// kernel.RenderFrame: it never assembles assistant text from raw provider
// events, never mutates kernel state directly. It sees the world only through
// the render channel; any user-originated events go back to the kernel via
// the Submit / Cancel callbacks provided by cmd/gormes/main.go.
package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

// Submitter is the callback wired by main.go to enqueue a user turn on the
// kernel. Return value is intentionally omitted — kernel backpressure
// (ErrEventMailboxFull) is vanishingly rare with a 16-slot buffer and, when
// it does fire, the kernel itself surfaces the error on the next render
// frame. The TUI does NOT act on the return value; it just schedules.
type Submitter func(text string)

// Canceller is the callback wired by main.go to send PlatformEventCancel.
type Canceller func()

// Options carries local TUI settings that do not belong to kernel state.
type Options struct {
	MouseTracking bool
	MouseModeCmd  func(enabled bool) tea.Cmd
	// SessionBranch is the injected fork helper invoked by the /branch
	// slash command. nil disables /branch (handler returns
	// `branch: store unavailable`); cmd/gormes wires the real
	// session.Fork-backed implementation in main.go.
	SessionBranch SessionBranchFunc
}

// Model is the Bubble Tea state. The only external dependency is the
// read-side of the render channel (from kernel.Render()). Everything else
// is local UI state.
type Model struct {
	width, height int

	editor textarea.Model

	// frame is the latest RenderFrame received from the kernel. View() renders
	// this snapshot; Update() replaces it on every frameMsg.
	frame kernel.RenderFrame

	frames   <-chan kernel.RenderFrame
	submit   Submitter
	cancel   Canceller
	inFlight bool // true between a user submit and the next PhaseIdle frame

	mouseTracking bool
	mouseModeCmd  func(enabled bool) tea.Cmd
	statusMessage string

	// sessionID, when non-empty, is the locally-tracked active session
	// owned by a successful /branch fork. SessionID() prefers it over
	// frame.SessionID so subsequent UI reads see the branch session even
	// before the kernel acks the switch on its next render frame.
	sessionID     string
	sessionBranch SessionBranchFunc

	slashRegistry *SlashRegistry
}

// NewModel constructs the Bubble Tea model. frames is the kernel's Render()
// channel; submit/cancel are closures from main.go that forward to
// kernel.Submit with the appropriate PlatformEvent kind.
func NewModel(frames <-chan kernel.RenderFrame, submit Submitter, cancel Canceller) Model {
	return NewModelWithOptions(frames, submit, cancel, Options{MouseTracking: true})
}

// NewModelWithOptions constructs the Bubble Tea model with explicit local TUI
// options. cmd/gormes seeds these from config; tests can inject MouseModeCmd to
// assert terminal mode changes without a real terminal.
func NewModelWithOptions(frames <-chan kernel.RenderFrame, submit Submitter, cancel Canceller, opts Options) Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message and hit Enter…"
	ta.ShowLineNumbers = false
	ta.Focus()
	return Model{
		editor:        ta,
		frames:        frames,
		submit:        submit,
		cancel:        cancel,
		mouseTracking: opts.MouseTracking,
		mouseModeCmd:  opts.MouseModeCmd,
		sessionBranch: opts.SessionBranch,
		slashRegistry: NewDefaultSlashRegistry(),
	}
}

// SessionID returns the model's active session identifier. A locally-tracked
// branch session (set by the /branch slash handler) takes precedence over the
// kernel-supplied frame.SessionID so the TUI surfaces the fork target even
// before the next render frame arrives.
func (m *Model) SessionID() string {
	if m.sessionID != "" {
		return m.sessionID
	}
	return m.frame.SessionID
}

// frameMsg wraps an incoming kernel.RenderFrame as a Bubble Tea message so
// Update() can handle it via the normal msg switch.
type frameMsg kernel.RenderFrame

// waitFrame returns a tea.Cmd that blocks on the render channel once and
// converts the next frame into a frameMsg. Update() re-schedules it after
// handling each frameMsg so the pump never stops. If the channel closes
// (kernel exit), we return tea.Quit to unwind cleanly.
func (m Model) waitFrame() tea.Cmd {
	return func() tea.Msg {
		f, ok := <-m.frames
		if !ok {
			return tea.QuitMsg{}
		}
		return frameMsg(f)
	}
}

// Init is the Bubble Tea entry point. We start the cursor blink and the
// first render-frame wait in parallel.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, m.waitFrame()}
	if !m.mouseTracking {
		cmds = append(cmds, m.emitMouseModeCmd(false))
	}
	return tea.Batch(cmds...)
}
