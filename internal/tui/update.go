package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

// submittedMsg is emitted after a submit Cmd completes. It carries no data —
// its only role is to signal back into Update so the inFlight flag is
// authoritatively set on the same goroutine that reads it.
type submittedMsg struct{}

// cancelledMsg is the symmetric signal for cancel Cmds.
type cancelledMsg struct{}

// Update is the Bubble Tea event loop. MUST NOT block: every kernel
// interaction is dispatched via tea.Cmd returned values.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.editor.SetWidth(maxInt(msg.Width-4, 20))

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			// In-flight: cancel the turn. Idle: quit.
			if m.inFlight {
				cmds = append(cmds, m.cancelCmd())
			} else {
				return m, tea.Quit
			}
		case tea.KeyCtrlD:
			return m, tea.Quit
		case tea.KeyCtrlL:
			// Clear the local view by zeroing the frame's visible content.
			// Kernel history is unchanged; next real frame repopulates.
			m.frame.History = nil
			m.frame.DraftText = ""
			m.frame.LastError = ""
		case tea.KeyEnter:
			if msg.Alt {
				// Alt+Enter is treated as newline-in-editor on many terminals
				// that do not forward Shift+Enter. Fall through to let the
				// textarea insert a newline naturally.
				break
			}
			text := m.editor.Value()
			mouse := parseMouseTrackingSlash(text, m.mouseTracking)
			if mouse.handled {
				m.editor.Reset()
				if !mouse.valid {
					m.statusMessage = mouse.message
					return m, tea.Batch(cmds...)
				}
				m.statusMessage = "mouse tracking on"
				if !mouse.next {
					m.statusMessage = "mouse tracking off"
				}
				if mouse.next != m.mouseTracking {
					m.mouseTracking = mouse.next
					cmds = append(cmds, m.emitMouseModeCmd(mouse.next))
				}
				return m, tea.Batch(cmds...)
			}
			if text != "" && !m.inFlight {
				m.editor.Reset()
				m.inFlight = true
				cmds = append(cmds, m.submitCmd(text))
			}
			// Return early so textarea's own Enter handling does not insert
			// a newline on the now-empty editor.
			return m, tea.Batch(cmds...)
		}

	case frameMsg:
		m.frame = kernel.RenderFrame(msg)
		// Authoritative inFlight reset: the kernel reports PhaseIdle once
		// the turn is fully finalized.
		if m.frame.Phase == kernel.PhaseIdle {
			m.inFlight = false
		}
		cmds = append(cmds, m.waitFrame())

	case submittedMsg, cancelledMsg:
		// No-op — submit/cancel are fire-and-forget. The render-frame pump
		// provides authoritative feedback via frameMsg / m.frame.Phase.
	}

	// Forward the message to the textarea for cursor / input handling.
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// submitCmd wraps the submit callback as a tea.Cmd so it runs off the
// Update goroutine.
func (m Model) submitCmd(text string) tea.Cmd {
	submit := m.submit
	return func() tea.Msg {
		submit(text)
		return submittedMsg{}
	}
}

// cancelCmd wraps the cancel callback as a tea.Cmd.
func (m Model) cancelCmd() tea.Cmd {
	cancel := m.cancel
	return func() tea.Msg {
		cancel()
		return cancelledMsg{}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
