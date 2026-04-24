package cli

import "strings"

// Helpers ported from hermes_cli/cli_output.py (Phase 5.O). Upstream exposes
// a tight set of colored print helpers plus a prompt() / prompt_yes_no()
// pair that several CLI surfaces share (setup, tools_config, mcp_config,
// memory_setup). The Go port keeps the pure formatting and parsing in
// deterministic, TTY-free helpers so the interactive I/O path can layer on
// top (reading from a configurable io.Reader / io.Writer) without dragging
// the string-shape contract into test fixtures.
//
// The Rich-console rendering and terminal-table helpers that upstream
// cli_output.py exposes in neighboring modules stay tracked as follow-on
// scope under 5.O.

// FormatInfo mirrors hermes_cli.cli_output.print_info: a two-space indent
// followed by text, colored dim when useColor is true.
func FormatInfo(text string, useColor bool) string {
	return Colorize("  "+text, useColor, ColorDim)
}

// FormatSuccess mirrors hermes_cli.cli_output.print_success: a "✓ " prefix
// followed by text, colored green when useColor is true.
func FormatSuccess(text string, useColor bool) string {
	return Colorize("✓ "+text, useColor, ColorGreen)
}

// FormatWarning mirrors hermes_cli.cli_output.print_warning: a "⚠ " prefix
// followed by text, colored yellow when useColor is true.
func FormatWarning(text string, useColor bool) string {
	return Colorize("⚠ "+text, useColor, ColorYellow)
}

// FormatError mirrors hermes_cli.cli_output.print_error: a "✗ " prefix
// followed by text, colored red when useColor is true.
func FormatError(text string, useColor bool) string {
	return Colorize("✗ "+text, useColor, ColorRed)
}

// FormatHeader mirrors hermes_cli.cli_output.print_header: a leading
// newline + two-space indent followed by text, colored yellow when
// useColor is true.
func FormatHeader(text string, useColor bool) string {
	return Colorize("\n  "+text, useColor, ColorYellow)
}

// FormatPromptLabel mirrors the display-string construction inside
// hermes_cli.cli_output.prompt: "  <question>[ [<default>]]: " wrapped in
// yellow when useColor is true. The caller is responsible for actually
// reading a line from the user — this helper only renders the prompt.
func FormatPromptLabel(question, defaultValue string, useColor bool) string {
	var b strings.Builder
	b.Grow(len(question) + len(defaultValue) + 8)
	b.WriteString("  ")
	b.WriteString(question)
	if defaultValue != "" {
		b.WriteString(" [")
		b.WriteString(defaultValue)
		b.WriteString("]")
	}
	b.WriteString(": ")
	return Colorize(b.String(), useColor, ColorYellow)
}

// ResolvePromptAnswer mirrors the trailing logic of
// hermes_cli.cli_output.prompt: strip whitespace around the raw reply and
// fall back to defaultValue when the stripped result is empty. An empty
// defaultValue yields the empty string (upstream `return value if value
// else (default or "")`).
func ResolvePromptAnswer(raw, defaultValue string) string {
	stripped := strings.TrimSpace(raw)
	if stripped != "" {
		return stripped
	}
	return defaultValue
}

// FormatYesNoHint mirrors the hint construction in
// hermes_cli.cli_output.prompt_yes_no: "Y/n" when the default is yes,
// otherwise "y/N".
func FormatYesNoHint(defaultYes bool) string {
	if defaultYes {
		return "Y/n"
	}
	return "y/N"
}

// ParseYesNoAnswer mirrors the decision logic of
// hermes_cli.cli_output.prompt_yes_no: empty input (after whitespace
// trimming) returns defaultYes; otherwise the answer is true iff the
// stripped reply starts with 'y' (case-insensitive). Any other non-empty
// input returns false so affirmative answers aren't silently accepted on
// unexpected strings.
func ParseYesNoAnswer(raw string, defaultYes bool) bool {
	stripped := strings.TrimSpace(raw)
	if stripped == "" {
		return defaultYes
	}
	return strings.HasPrefix(strings.ToLower(stripped), "y")
}
