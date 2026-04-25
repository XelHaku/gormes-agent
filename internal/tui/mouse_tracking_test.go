package tui

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

const (
	bubbleTeaEnableMouseAllMotionSeq = "\x1b[?1003h\x1b[?1006h"
	bubbleTeaDisableMouseSeq         = "\x1b[?1002l\x1b[?1003l\x1b[?1006l"
)

func TestParseMouseTrackingSlash(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		current bool
		want    mouseSlashResult
	}{
		{
			name:    "bare mouse toggles off",
			input:   "/mouse",
			current: true,
			want:    mouseSlashResult{handled: true, valid: true, next: false},
		},
		{
			name:    "toggle toggles on",
			input:   "/mouse toggle",
			current: false,
			want:    mouseSlashResult{handled: true, valid: true, next: true},
		},
		{
			name:    "on enables",
			input:   "/mouse on",
			current: false,
			want:    mouseSlashResult{handled: true, valid: true, next: true},
		},
		{
			name:    "off disables",
			input:   "/mouse off",
			current: true,
			want:    mouseSlashResult{handled: true, valid: true, next: false},
		},
		{
			name:    "scroll alias disables",
			input:   "/scroll off",
			current: true,
			want:    mouseSlashResult{handled: true, valid: true, next: false},
		},
		{
			name:    "invalid value is handled as usage error",
			input:   "/mouse sideways",
			current: true,
			want:    mouseSlashResult{handled: true, valid: false, message: "usage: /mouse [on|off|toggle]"},
		},
		{
			name:    "other slash command is not handled",
			input:   "/help",
			current: true,
			want:    mouseSlashResult{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseMouseTrackingSlash(tt.input, tt.current); got != tt.want {
				t.Fatalf("parseMouseTrackingSlash(%q, %v) = %#v, want %#v", tt.input, tt.current, got, tt.want)
			}
		})
	}
}

func TestMouseSlashUpdatesRuntimeWithoutSubmitting(t *testing.T) {
	var submitted []string
	rec := &mouseModeRecorder{}

	m := NewModelWithOptions(
		make(chan kernel.RenderFrame),
		func(text string) { submitted = append(submitted, text) },
		func() {},
		Options{MouseTracking: false, MouseModeCmd: rec.cmd},
	)

	m.editor.SetValue("/mouse on")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	runTestCmd(t, cmd)

	if len(submitted) != 0 {
		t.Fatalf("/mouse was submitted to kernel: %#v", submitted)
	}
	if !m.mouseTracking {
		t.Fatal("mouseTracking = false after /mouse on, want true")
	}
	if !reflect.DeepEqual(rec.modes, []bool{true}) {
		t.Fatalf("emitted modes = %#v, want enable once", rec.modes)
	}

	m.editor.SetValue("/mouse on")
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	runTestCmd(t, cmd)

	if !reflect.DeepEqual(rec.modes, []bool{true}) {
		t.Fatalf("repeated /mouse on emitted modes = %#v, want no duplicate", rec.modes)
	}

	m.editor.SetValue("/mouse toggle")
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	runTestCmd(t, cmd)

	if m.mouseTracking {
		t.Fatal("mouseTracking = true after /mouse toggle from on, want false")
	}
	if !reflect.DeepEqual(rec.modes, []bool{true, false}) {
		t.Fatalf("emitted modes = %#v, want enable then disable", rec.modes)
	}
}

func TestInitDisablesMouseTrackingWhenConfiguredOff(t *testing.T) {
	frames := make(chan kernel.RenderFrame, 1)
	frames <- kernel.RenderFrame{Phase: kernel.PhaseIdle, Seq: 1}
	rec := &mouseModeRecorder{}

	m := NewModelWithOptions(
		frames,
		func(string) {},
		func() {},
		Options{MouseTracking: false, MouseModeCmd: rec.cmd},
	)

	runTestCmd(t, m.Init())

	if !reflect.DeepEqual(rec.modes, []bool{false}) {
		t.Fatalf("initial emitted modes = %#v, want explicit disable on alt-screen entry", rec.modes)
	}
}

func TestViewReportsMouseTrackingDisabled(t *testing.T) {
	m := NewModelWithOptions(
		make(chan kernel.RenderFrame),
		func(string) {},
		func() {},
		Options{MouseTracking: false},
	)
	m.width = 120
	m.height = 40
	m.frame = kernel.RenderFrame{Phase: kernel.PhaseIdle, Model: "hermes-agent"}

	if out := m.View(); !strings.Contains(out, "mouse: disabled") {
		t.Fatalf("View() missing disabled mouse status:\n%s", out)
	}
}

func TestDefaultMouseModeCmdEmitsBubbleTeaTerminalSequences(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    string
	}{
		{name: "enable", enabled: true, want: bubbleTeaEnableMouseAllMotionSeq},
		{name: "disable", enabled: false, want: bubbleTeaDisableMouseSeq},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := terminalOutputForCmd(t, defaultMouseModeCmd(tt.enabled))
			if !strings.Contains(out, tt.want) {
				t.Fatalf("terminal output missing %q:\n%q", tt.want, out)
			}
		})
	}
}

type mouseModeRecorder struct {
	modes []bool
}

func (r *mouseModeRecorder) cmd(enabled bool) tea.Cmd {
	return func() tea.Msg {
		r.modes = append(r.modes, enabled)
		return nil
	}
}

func runTestCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			runTestCmd(t, c)
		}
	default:
	}
}

type initCmdModel struct {
	cmd tea.Cmd
}

func (m initCmdModel) Init() tea.Cmd {
	return tea.Sequence(m.cmd, tea.Quit)
}

func (m initCmdModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m initCmdModel) View() string {
	return ""
}

func terminalOutputForCmd(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	var out bytes.Buffer
	p := tea.NewProgram(
		initCmdModel{cmd: cmd},
		tea.WithInput(bytes.NewBuffer(nil)),
		tea.WithOutput(&out),
		tea.WithoutSignals(),
	)
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(&out)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
