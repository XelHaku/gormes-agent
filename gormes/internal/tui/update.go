package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/kernel"
)

// Update is the Bubble Tea event loop. MUST NOT block; all kernel interactions
// go through tea.Cmd returned values.
// Tasks 16 (View) and 17 (full Update + keybindings) expand this.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.editor.SetWidth(maxInt(msg.Width-4, 20))

	case frameMsg:
		m.frame = kernel.RenderFrame(msg)
		if m.frame.Phase == kernel.PhaseIdle {
			m.inFlight = false
		}
		cmds = append(cmds, m.waitFrame())
	}

	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
