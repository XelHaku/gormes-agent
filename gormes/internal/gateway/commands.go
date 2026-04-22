package gateway

import (
	"fmt"
	"strings"
)

// CommandDef is the canonical slash-command definition shared by gateway
// parsing, help text, and per-platform command exposure helpers.
type CommandDef struct {
	Name        string
	Description string
	Kind        EventKind
	Aliases     []string
}

// PlatformCommand is the platform-facing command/menu shape used for channel
// exposure helpers such as Telegram bot menus.
type PlatformCommand struct {
	Name        string
	Description string
}

// CommandRegistry is the single source of truth for gateway slash commands.
var CommandRegistry = []CommandDef{
	{
		Name:        "help",
		Description: "Show available commands",
		Kind:        EventStart,
		Aliases:     []string{"start"},
	},
	{
		Name:        "new",
		Description: "Start a fresh session",
		Kind:        EventReset,
	},
	{
		Name:        "stop",
		Description: "Cancel the active turn",
		Kind:        EventCancel,
	},
}

var commandLookup = buildCommandLookup()

func buildCommandLookup() map[string]CommandDef {
	lookup := make(map[string]CommandDef, len(CommandRegistry)*2)
	for _, cmd := range CommandRegistry {
		lookup[cmd.Name] = cmd
		for _, alias := range cmd.Aliases {
			lookup[alias] = cmd
		}
	}
	return lookup
}

// ResolveCommand maps a slash command or alias to its canonical definition.
func ResolveCommand(name string) (CommandDef, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	key = strings.TrimPrefix(key, "/")
	if i := strings.IndexAny(key, " \t\r\n"); i >= 0 {
		key = key[:i]
	}
	cmd, ok := commandLookup[key]
	return cmd, ok
}

// ParseInboundText normalizes a channel message into a shared EventKind/body
// pair. Plain text becomes EventSubmit; recognized slash commands become their
// mapped EventKind; unknown slash commands become EventUnknown.
func ParseInboundText(text string) (EventKind, string) {
	body := strings.TrimSpace(text)
	if !strings.HasPrefix(body, "/") {
		return EventSubmit, body
	}
	cmd, ok := ResolveCommand(body)
	if !ok {
		return EventUnknown, ""
	}
	return cmd.Kind, ""
}

// GatewayHelpLines renders registry-driven help output in canonical order.
func GatewayHelpLines() []string {
	lines := make([]string, 0, len(CommandRegistry))
	for _, cmd := range CommandRegistry {
		aliasNote := ""
		if len(cmd.Aliases) > 0 {
			aliases := make([]string, len(cmd.Aliases))
			for i, alias := range cmd.Aliases {
				aliases[i] = "`/" + alias + "`"
			}
			aliasNote = " (alias: " + strings.Join(aliases, ", ") + ")"
		}
		lines = append(lines, fmt.Sprintf("`/%s` -- %s%s", cmd.Name, cmd.Description, aliasNote))
	}
	return lines
}

func gatewayHelpText() string {
	return "Gormes is online. Available commands:\n" + strings.Join(GatewayHelpLines(), "\n")
}

// TelegramBotCommands returns the canonical Telegram command menu in registry
// order. Aliases are intentionally excluded from the menu surface.
func TelegramBotCommands() []PlatformCommand {
	out := make([]PlatformCommand, 0, len(CommandRegistry))
	for _, cmd := range CommandRegistry {
		out = append(out, PlatformCommand{
			Name:        strings.ReplaceAll(cmd.Name, "-", "_"),
			Description: cmd.Description,
		})
	}
	return out
}

// SlackSubcommandMap returns the canonical slash mapping Slack should expose.
// Both canonical names and aliases resolve to their slash-prefixed entry.
func SlackSubcommandMap() map[string]string {
	out := make(map[string]string, len(CommandRegistry)*2)
	for _, cmd := range CommandRegistry {
		out[cmd.Name] = "/" + cmd.Name
		for _, alias := range cmd.Aliases {
			out[alias] = "/" + alias
		}
	}
	return out
}
