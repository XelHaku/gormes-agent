package tuigateway

import "strings"

// ToolProgressModeAll is the default tool-progress rendering mode —
// every tool.start / tool.complete event is surfaced to the TUI.
const ToolProgressModeAll = "all"

// ToolProgressModeOff suppresses every tool-progress emission.
const ToolProgressModeOff = "off"

// ToolProgressModeNew surfaces only the first appearance of each tool per turn.
const ToolProgressModeNew = "new"

// ToolProgressModeVerbose surfaces tool-progress events with extra detail
// (upstream uses it to show JSON result previews alongside the summary line).
const ToolProgressModeVerbose = "verbose"

// ParseToolProgressMode normalises a raw config value (as decoded from
// `display.tool_progress` in `config.yaml`) into one of the four canonical
// tool-progress mode strings. It mirrors `_load_tool_progress_mode` in
// `tui_gateway/server.py` (lines 377-384) branch-for-branch:
//
//   - a boolean `false` coerces to `"off"` (operators who disable the
//     block with `display: {tool_progress: false}` keep working).
//   - a boolean `true` coerces to `"all"` (the inverse shorthand).
//   - a string is trimmed and lowercased; an empty/whitespace-only string
//     falls back to `"all"` so a blank config entry behaves like the
//     documented default.
//   - unknown string values, and non-bool/non-string types, fall back to
//     `"all"` so a stale or malformed config never silently disables the
//     progress surface.
//
// The helper is pure: it never reads config.yaml, never consults the
// environment, and never allocates beyond the normalised string. Callers
// own the file I/O and slot the result into per-session state the same
// way Python's `_session_tool_progress_mode` caches it on the session.
func ParseToolProgressMode(raw any) string {
	switch v := raw.(type) {
	case bool:
		if v {
			return ToolProgressModeAll
		}
		return ToolProgressModeOff
	case string:
		mode := strings.ToLower(strings.TrimSpace(v))
		switch mode {
		case "":
			return ToolProgressModeAll
		case ToolProgressModeOff, ToolProgressModeNew, ToolProgressModeAll, ToolProgressModeVerbose:
			return mode
		}
	}
	return ToolProgressModeAll
}

// ToolProgressEnabled reports whether the given mode surfaces any
// tool-progress events to the TUI. It mirrors `_tool_progress_enabled`
// in `tui_gateway/server.py` (line 402): only the literal `"off"` mode
// suppresses emission; every other value — including unknown strings —
// is treated as enabled so a stale mode never silently hides progress.
func ToolProgressEnabled(mode string) bool {
	return mode != ToolProgressModeOff
}
