package hermes

import (
	"strings"
	"unicode"
)

const gormesSelfHelpGuidanceBlock = `Gormes self-help guidance:
When the user asks about configuring, setting up, troubleshooting, or using Gormes or Gormes Agent itself, consult Gormes-owned self-help material before answering.
- Prefer the future gormes-self-help skill when it is available.
- If that skill is unavailable, use the local Hugo docs surface at https://docs.gormes.ai/, especially Using Gormes and Building Gormes pages.
- Include evidence self-help-unavailable when falling back to docs because the skill is unavailable.
- Keep this guidance scoped to Gormes/Gormes Agent self-help; omit it for unrelated prompts.`

// GormesSelfHelpGuidanceForPrompt returns the deterministic self-help prompt
// block only for user prompts that ask about operating Gormes itself.
func GormesSelfHelpGuidanceForPrompt(userPrompt string) (string, bool) {
	if !shouldUseGormesSelfHelpGuidance(userPrompt) {
		return "", false
	}
	return gormesSelfHelpGuidanceBlock, true
}

func shouldUseGormesSelfHelpGuidance(userPrompt string) bool {
	tokens := gormesSelfHelpTokens(userPrompt)
	if !containsToken(tokens, "gormes") {
		return false
	}
	if containsAdjacentTokens(tokens, "set", "up") {
		return true
	}
	for i, token := range tokens {
		switch {
		case strings.HasPrefix(token, "configur"):
			return true
		case token == "setup":
			return true
		case strings.HasPrefix(token, "troubleshoot"):
			return true
		case token == "usage":
			return true
		case token == "use" || token == "using":
			if tokenAdjacentTo(tokens, i, "gormes") {
				return true
			}
		case strings.HasPrefix(token, "install"):
			return true
		}
	}
	return false
}

func gormesSelfHelpTokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func containsToken(tokens []string, want string) bool {
	for _, token := range tokens {
		if token == want {
			return true
		}
	}
	return false
}

func containsAdjacentTokens(tokens []string, first, second string) bool {
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i] == first && tokens[i+1] == second {
			return true
		}
	}
	return false
}

func tokenAdjacentTo(tokens []string, index int, want string) bool {
	return (index > 0 && tokens[index-1] == want) || (index+1 < len(tokens) && tokens[index+1] == want)
}
