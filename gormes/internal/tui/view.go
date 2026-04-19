package tui

// View renders the current RenderFrame. Minimal placeholder in Task 15;
// Task 16 replaces this with the full lipgloss responsive layout.
func (m Model) View() string {
	if m.width < 20 || m.height < 10 {
		return "terminal too narrow — resize to at least 20×10"
	}
	return "gormes dashboard (view placeholder — Task 16 renders the real layout)\n" + m.editor.View()
}
