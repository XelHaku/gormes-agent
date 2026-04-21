package progress

import (
	"fmt"
	"regexp"
	"strings"
)

// startMarker matches <!-- PROGRESS:START kind=<name> --> with flexible spacing.
var startMarker = regexp.MustCompile(`<!--\s*PROGRESS:START\s+kind=([a-zA-Z0-9_-]+)\s*-->`)

const endMarker = "<!-- PROGRESS:END -->"

// ReplaceMarker replaces the content between PROGRESS:START kind=<kind>
// and PROGRESS:END with the supplied body. The markers themselves are
// preserved. Returns an error if the markers are missing, unbalanced,
// or the start marker's kind does not match.
func ReplaceMarker(input, kind, body string) (string, error) {
	loc := startMarker.FindStringIndex(input)
	if loc == nil {
		return "", fmt.Errorf("progress: start marker not found")
	}
	kindMatch := startMarker.FindStringSubmatch(input[loc[0]:loc[1]])
	if len(kindMatch) < 2 || kindMatch[1] != kind {
		got := ""
		if len(kindMatch) >= 2 {
			got = kindMatch[1]
		}
		return "", fmt.Errorf("progress: expected kind=%q, found %q", kind, got)
	}
	bodyStart := loc[1]
	endIdx := strings.Index(input[bodyStart:], endMarker)
	if endIdx < 0 {
		return "", fmt.Errorf("progress: end marker not found after start")
	}
	endAbs := bodyStart + endIdx
	endStop := endAbs + len(endMarker)
	return input[:bodyStart] + "\n" + body + input[endAbs:endStop] + input[endStop:], nil
}
