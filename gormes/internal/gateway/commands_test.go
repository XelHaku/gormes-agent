package gateway

import (
	"reflect"
	"strings"
	"testing"
)

func TestCommandRegistryContainsRequiredCommands(t *testing.T) {
	if len(CommandRegistry) == 0 {
		t.Fatal("CommandRegistry is empty")
	}

	required := map[string]bool{
		"help": false,
		"new":  false,
		"stop": false,
	}
	for _, cmd := range CommandRegistry {
		if _, ok := required[cmd.Name]; ok {
			required[cmd.Name] = true
		}
	}
	for name, seen := range required {
		if !seen {
			t.Fatalf("CommandRegistry missing %q", name)
		}
	}
}

func TestResolveCommand(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "help", raw: "/help", want: "help", ok: true},
		{name: "new", raw: "/new", want: "new", ok: true},
		{name: "stop", raw: "/stop", want: "stop", ok: true},
		{name: "telegram alias", raw: "/start", want: "help", ok: true},
		{name: "unknown", raw: "/xyzzy", want: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResolveCommand(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ResolveCommand(%q) ok = %v, want %v", tt.raw, ok, tt.ok)
			}
			if !tt.ok {
				return
			}
			if got.Name != tt.want {
				t.Fatalf("ResolveCommand(%q).Name = %q, want %q", tt.raw, got.Name, tt.want)
			}
		})
	}
}

func TestParseInboundText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantKind EventKind
		wantBody string
	}{
		{name: "help", text: "/help", wantKind: EventStart, wantBody: ""},
		{name: "new", text: "/new", wantKind: EventReset, wantBody: ""},
		{name: "stop", text: "/stop", wantKind: EventCancel, wantBody: ""},
		{name: "unknown slash", text: "/wat", wantKind: EventUnknown, wantBody: ""},
		{name: "submit", text: "hello there", wantKind: EventSubmit, wantBody: "hello there"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotBody := ParseInboundText(tt.text)
			if gotKind != tt.wantKind || gotBody != tt.wantBody {
				t.Fatalf("ParseInboundText(%q) = (%v, %q), want (%v, %q)", tt.text, gotKind, gotBody, tt.wantKind, tt.wantBody)
			}
		})
	}
}

func TestGatewayHelpLinesDerivedFromRegistry(t *testing.T) {
	lines := GatewayHelpLines()
	if len(lines) == 0 {
		t.Fatal("GatewayHelpLines returned no lines")
	}

	joined := strings.Join(lines, "\n")
	for _, want := range []string{"/help", "/new", "/stop"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("GatewayHelpLines missing %q in %q", want, joined)
		}
	}
}

func TestPlatformExposureDeterministic(t *testing.T) {
	tg1 := TelegramBotCommands()
	tg2 := TelegramBotCommands()
	if !reflect.DeepEqual(tg1, tg2) {
		t.Fatalf("TelegramBotCommands unstable:\n%#v\n%#v", tg1, tg2)
	}
	if len(tg1) == 0 {
		t.Fatal("TelegramBotCommands returned no commands")
	}

	slack1 := SlackSubcommandMap()
	slack2 := SlackSubcommandMap()
	if !reflect.DeepEqual(slack1, slack2) {
		t.Fatalf("SlackSubcommandMap unstable:\n%#v\n%#v", slack1, slack2)
	}
	for _, want := range []string{"help", "new", "stop"} {
		if _, ok := slack1[want]; !ok {
			t.Fatalf("SlackSubcommandMap missing %q", want)
		}
	}
}
