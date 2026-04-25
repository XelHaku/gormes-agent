package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

var (
	border    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	header    = lipgloss.NewStyle().Bold(true)
	muted     = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	userStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
	botStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

// View renders the Dashboard. Three size regimes:
//
//	width ≥ 100: full layout, sidebar width 28
//	80 ≤ width < 100: full layout, sidebar width 24
//	20 ≤ width < 80: sidebar collapses into a one-line status strip
//	width < 20 OR height < 10: single-line fallback banner
//
// View never panics on any non-negative (width, height) input.
func (m Model) View() string {
	if m.width < 20 || m.height < 10 {
		return "terminal too narrow — resize to at least 20×10"
	}

	sidebarW := 0
	switch {
	case m.width >= 100:
		sidebarW = 28
	case m.width >= 80:
		sidebarW = 24
	}

	// Account for border chars around the main pane and the sidebar plus a
	// 2-cell gap. Clamp to a minimum so sub-width scenarios still render.
	gap := 0
	if sidebarW > 0 {
		gap = 2
	}
	mainW := m.width - sidebarW - gap - 4 // 2 borders on each pane
	if mainW < 10 {
		mainW = 10
	}
	// Height budget: main pane (border adds 2) + editor pane (textarea +
	// border = editor.Height()+2) + status line (1) = m.height total.
	// So topH = m.height - (editor.Height() + 2) - 1 - 2 (outer borders).
	editorHeight := m.editor.Height()
	if editorHeight < 1 {
		editorHeight = 1
	}
	topH := m.height - editorHeight - 5
	if topH < 3 {
		topH = 3
	}

	main := border.Width(mainW).Height(topH).Render(renderConv(m.frame, mainW))

	var top string
	if sidebarW > 0 {
		sidebar := border.Width(sidebarW).Height(topH).Render(renderSidebar(m.frame, sidebarW))
		top = lipgloss.JoinHorizontal(lipgloss.Top, main, "  ", sidebar)
	} else {
		// Collapsed: main pane full width + one-line status strip above the editor.
		top = main
	}

	editorW := m.width - 2
	if editorW < 10 {
		editorW = 10
	}
	editor := border.Width(editorW).Render(m.editor.View())

	var status string
	if sidebarW > 0 {
		status = muted.Render(fmt.Sprintf(
			"phase: %s · model: %s · session: %s · %s%s",
			m.frame.Phase, m.frame.Model, shortSessionID(m.frame.SessionID), m.mouseStatus(), statusSuffix(m.statusMessage),
		))
	} else {
		// Collapsed mode includes the telemetry in the status line.
		t := m.frame.Telemetry
		status = muted.Render(fmt.Sprintf(
			"phase: %s · model: %s · tok/s: %.1f · lat: %dms · in/out: %d/%d · %s%s",
			m.frame.Phase, m.frame.Model, t.TokensPerSec, t.LatencyMsLast,
			t.TokensInTotal, t.TokensOutTotal, m.mouseStatus(), statusSuffix(m.statusMessage),
		))
	}

	return lipgloss.JoinVertical(lipgloss.Left, top, editor, status)
}

// renderConv renders the conversation pane: prior history turns, the
// streaming draft (if any), and a final LastError line (if any).
func renderConv(f kernel.RenderFrame, width int) string {
	if width < 4 {
		width = 4
	}
	wrap := lipgloss.NewStyle().Width(width - 4)

	var lines []string
	for _, msg := range f.History {
		tag := roleTag(msg.Role)
		lines = append(lines, tag+" "+wrap.Render(msg.Content))
	}
	if f.DraftText != "" {
		lines = append(lines, botStyle.Render("gormes:")+" "+wrap.Render(f.DraftText))
	}
	if f.LastError != "" {
		lines = append(lines, errStyle.Render("err:")+" "+f.LastError)
	}
	if len(lines) == 0 {
		return muted.Render("(start typing below to begin)")
	}
	return strings.Join(lines, "\n\n")
}

// renderSidebar renders the Telemetry + Soul Monitor pane.
func renderSidebar(f kernel.RenderFrame, width int) string {
	if width < 8 {
		width = 8
	}
	sep := strings.Repeat("─", width-4)

	var b strings.Builder
	b.WriteString(header.Render("Telemetry") + "\n")
	b.WriteString(fmt.Sprintf(" model: %s\n", truncateEllipsis(f.Telemetry.Model, width-8)))
	b.WriteString(fmt.Sprintf(" tok/s: %.1f\n", f.Telemetry.TokensPerSec))
	b.WriteString(fmt.Sprintf(" latency: %d ms\n", f.Telemetry.LatencyMsLast))
	b.WriteString(fmt.Sprintf(" in/out: %d/%d\n", f.Telemetry.TokensInTotal, f.Telemetry.TokensOutTotal))
	b.WriteString(sep + "\n")
	b.WriteString(header.Render("Soul Monitor") + "\n")
	if len(f.SoulEvents) == 0 {
		b.WriteString(muted.Render(" (idle)") + "\n")
	} else {
		for _, s := range f.SoulEvents {
			line := fmt.Sprintf(" [%s] %s", s.At.Format("15:04:05"), s.Text)
			b.WriteString(truncateEllipsis(line, width-2) + "\n")
		}
	}
	return b.String()
}

func roleTag(role string) string {
	switch role {
	case "user":
		return userStyle.Render("you:")
	case "assistant":
		return botStyle.Render("gormes:")
	case "system":
		return muted.Render("sys:")
	}
	return muted.Render(role + ":")
}

func truncateEllipsis(s string, n int) string {
	if n <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func shortSessionID(id string) string {
	if id == "" {
		return "new"
	}
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}

func statusSuffix(message string) string {
	if message == "" {
		return ""
	}
	return " · " + message
}
