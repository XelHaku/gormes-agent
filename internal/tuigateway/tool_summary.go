package tuigateway

import (
	"encoding/json"
	"fmt"
	"math"
)

// FormatToolSummary returns the compact completion text for a finished tool
// call, mirroring hermes-agent/tui_gateway/server.py:_tool_summary. It is a
// pure formatter: callers provide already-captured result JSON and optional
// duration, and callers decide whether an ok=false result should be emitted.
func FormatToolSummary(name string, resultJSON []byte, durationSeconds *float64) (string, bool) {
	data := decodeToolResult(resultJSON)
	duration := formatToolDuration(durationSeconds)
	suffix := ""
	if duration != "" {
		suffix = " in " + duration
	}

	text := ""
	switch name {
	case "web_search":
		if n, ok := countList(data, "data", "web"); ok {
			label := "searches"
			if n == 1 {
				label = "search"
			}
			text = fmt.Sprintf("Did %d %s", n, label)
		}
	case "web_extract":
		if n, ok := webExtractCount(data); ok {
			label := "pages"
			if n == 1 {
				label = "page"
			}
			text = fmt.Sprintf("Extracted %d %s", n, label)
		}
	}

	if text != "" {
		return text + suffix, true
	}
	if duration != "" {
		return "Completed" + suffix, true
	}
	return "", false
}

func decodeToolResult(resultJSON []byte) any {
	var data any
	if err := json.Unmarshal(resultJSON, &data); err != nil {
		return nil
	}
	return data
}

func countList(obj any, path ...string) (int, bool) {
	cur := obj
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0, false
		}
		cur = m[key]
	}
	list, ok := cur.([]any)
	if !ok {
		return 0, false
	}
	return len(list), true
}

func webExtractCount(data any) (int, bool) {
	if n, ok := countList(data, "results"); ok && n != 0 {
		return n, true
	}
	return countList(data, "data", "results")
}

func formatToolDuration(seconds *float64) string {
	if seconds == nil {
		return ""
	}
	if *seconds < 10 {
		return fmt.Sprintf("%.1fs", *seconds)
	}
	if *seconds < 60 {
		return fmt.Sprintf("%ds", int(math.RoundToEven(*seconds)))
	}
	totalSeconds := int(math.RoundToEven(*seconds))
	mins, secs := totalSeconds/60, totalSeconds%60
	if secs == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm %ds", mins, secs)
}
