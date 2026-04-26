package hermes

import (
	"regexp"
	"strings"
)

type reasoningTagPattern struct {
	closed       *regexp.Regexp
	unterminated *regexp.Regexp
	orphan       *regexp.Regexp
}

var (
	reasoningTagPatterns = []reasoningTagPattern{
		newReasoningTagPattern("think"),
		newReasoningTagPattern("thinking"),
		newReasoningTagPattern("reasoning"),
		newReasoningTagPattern("thought"),
		newReasoningTagPattern("REASONING_SCRATCHPAD"),
	}
	reasoningTagBlankLines = regexp.MustCompile(`[ \t]*\n[ \t]*\n+[ \t]*`)
)

func newReasoningTagPattern(tag string) reasoningTagPattern {
	name := regexp.QuoteMeta(tag)
	return reasoningTagPattern{
		closed: regexp.MustCompile(
			`(?is)<` + name + `\b[^>]*>.*?</` + name + `\s*>[ \t]*`,
		),
		unterminated: regexp.MustCompile(
			`(?is)(?:^|\n)[ \t]*<` + name + `\b[^>]*>.*$`,
		),
		orphan: regexp.MustCompile(
			`(?is)</?` + name + `\b[^>]*>[ \t]*`,
		),
	}
}

// SanitizeReasoningTags returns visible assistant text with inline reasoning
// XML blocks removed. Callers must keep raw stream/transcript text separately
// for audit rather than treating this sanitized copy as source evidence.
func SanitizeReasoningTags(text string) string {
	if text == "" {
		return ""
	}
	cleaned := text
	for _, pattern := range reasoningTagPatterns {
		cleaned = pattern.closed.ReplaceAllString(cleaned, "")
		cleaned = pattern.unterminated.ReplaceAllString(cleaned, "")
		cleaned = pattern.orphan.ReplaceAllString(cleaned, "")
	}
	cleaned = strings.TrimSpace(cleaned)
	cleaned = reasoningTagBlankLines.ReplaceAllString(cleaned, "\n")
	return strings.TrimSpace(cleaned)
}
