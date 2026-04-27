package cli

import (
	"regexp"
	"strings"
)

var (
	bracketedPasteStartBoundary = regexp.MustCompile(`(^|[\s>:\]\)])\[200~`)
	bracketedPasteEndBoundary   = regexp.MustCompile(`\[201~($|[\s<\[\(\):;.,!?])`)
	pasteFragmentStartBoundary  = regexp.MustCompile(`(^|[\s>:\]\)])00~`)
	pasteFragmentEndBoundary    = regexp.MustCompile(`01~($|[\s<\[\(\):;.,!?])`)
)

// StripLeakedBracketedPasteWrappers removes bracketed-paste markers that
// terminals can leak into CLI buffers while preserving ordinary inline text.
func StripLeakedBracketedPasteWrappers(text string) string {
	if text == "" {
		return text
	}

	text = strings.ReplaceAll(text, "\x1b[200~", "")
	text = strings.ReplaceAll(text, "\x1b[201~", "")
	text = strings.ReplaceAll(text, "^[[200~", "")
	text = strings.ReplaceAll(text, "^[[201~", "")

	text = bracketedPasteStartBoundary.ReplaceAllString(text, "$1")
	text = bracketedPasteEndBoundary.ReplaceAllString(text, "$1")
	text = pasteFragmentStartBoundary.ReplaceAllString(text, "$1")
	text = pasteFragmentEndBoundary.ReplaceAllString(text, "$1")
	return text
}
