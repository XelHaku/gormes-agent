package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const mouseSlashUsage = "usage: /mouse [on|off|toggle]"

type mouseSlashResult struct {
	handled bool
	valid   bool
	next    bool
	message string
}

func parseMouseTrackingSlash(input string, current bool) mouseSlashResult {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return mouseSlashResult{}
	}

	name := strings.ToLower(fields[0])
	if name != "/mouse" && name != "/scroll" {
		return mouseSlashResult{}
	}
	if len(fields) > 2 {
		return mouseSlashResult{handled: true, message: mouseSlashUsage}
	}

	arg := ""
	if len(fields) == 2 {
		arg = strings.ToLower(fields[1])
	}

	switch arg {
	case "", "toggle":
		return mouseSlashResult{handled: true, valid: true, next: !current}
	case "on":
		return mouseSlashResult{handled: true, valid: true, next: true}
	case "off":
		return mouseSlashResult{handled: true, valid: true, next: false}
	default:
		return mouseSlashResult{handled: true, message: mouseSlashUsage}
	}
}

func defaultMouseModeCmd(enabled bool) tea.Cmd {
	if enabled {
		return tea.EnableMouseAllMotion
	}
	return tea.DisableMouse
}

func (m Model) emitMouseModeCmd(enabled bool) tea.Cmd {
	if m.mouseModeCmd != nil {
		return m.mouseModeCmd(enabled)
	}
	return defaultMouseModeCmd(enabled)
}

func (m Model) mouseStatus() string {
	if m.mouseTracking {
		return "mouse: on"
	}
	return "mouse: disabled"
}
