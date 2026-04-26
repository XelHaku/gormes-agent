package tui

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

func TestSlashRegistry_DispatchRoutesToRegisteredHandler(t *testing.T) {
	registry := NewSlashRegistry()

	var got string
	registry.Register("foo", func(input string, model *Model) SlashResult {
		got = input
		return SlashResult{Handled: true, StatusMessage: "foo ok"}
	})

	res := registry.Dispatch("/foo bar", nil)
	if !res.Handled {
		t.Fatalf("Handled = false, want true")
	}
	if got != "/foo bar" {
		t.Fatalf("handler received input %q, want %q (full input including slash)", got, "/foo bar")
	}
	if res.StatusMessage != "foo ok" {
		t.Fatalf("StatusMessage = %q, want %q", res.StatusMessage, "foo ok")
	}
}

func TestSlashRegistry_UnknownSlashIsNotHandled(t *testing.T) {
	registry := NewSlashRegistry()

	res := registry.Dispatch("/unknown some args", nil)
	if res.Handled {
		t.Fatalf("Handled = true for /unknown, want false (must fall through to submit)")
	}
}

func TestSlashRegistry_NonSlashInputIsNotHandled(t *testing.T) {
	registry := NewDefaultSlashRegistry()

	for _, input := range []string{"", "   ", "hello world", "save"} {
		t.Run(input, func(t *testing.T) {
			res := registry.Dispatch(input, nil)
			if res.Handled {
				t.Fatalf("Dispatch(%q) Handled = true, want false (no leading slash)", input)
			}
		})
	}
}

func TestSlashRegistry_SaveStubReturnsNotImplemented(t *testing.T) {
	registry := NewDefaultSlashRegistry()

	res := registry.Dispatch("/save", nil)
	if !res.Handled {
		t.Fatal("/save Handled = false, want true (default registry must register save stub)")
	}
	if res.StatusMessage != "save not yet implemented" {
		t.Fatalf("StatusMessage = %q, want %q", res.StatusMessage, "save not yet implemented")
	}
	if res.Cmd != nil {
		t.Fatal("Cmd != nil, want nil for stub (no terminal-mode side effects)")
	}
}

func TestSlashRegistry_MouseHandlerMigrationParity(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		current bool
		want    string
	}{
		{name: "on", input: "/mouse on", current: false, want: "mouse tracking on"},
		{name: "off", input: "/mouse off", current: true, want: "mouse tracking off"},
		{name: "invalid", input: "/mouse foo", current: true, want: "usage: /mouse [on|off|toggle]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &mouseModeRecorder{}
			m := NewModelWithOptions(
				make(chan kernel.RenderFrame),
				func(string) {},
				func() {},
				Options{MouseTracking: tt.current, MouseModeCmd: rec.cmd},
			)

			registry := NewDefaultSlashRegistry()
			res := registry.Dispatch(tt.input, &m)

			if !res.Handled {
				t.Fatalf("Handled = false for %q, want true (parity with parseMouseTrackingSlash)", tt.input)
			}
			if res.StatusMessage != tt.want {
				t.Fatalf("StatusMessage = %q, want %q", res.StatusMessage, tt.want)
			}
		})
	}
}
