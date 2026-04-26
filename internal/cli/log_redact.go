package cli

import (
	"bytes"
	"regexp"
)

var (
	logRedactedMarker = []byte("[REDACTED]")

	logBearerTokenRE  = regexp.MustCompile(`(?i)(\bBearer[[:space:]]+)[^[:space:],;]+`)
	logAPIKeyValueRE  = regexp.MustCompile(`(?i)(\bapi_key=)[^[:space:]&]+`)
	logXAPIKeyValueRE = regexp.MustCompile(`(?i)(\bx-api-key:[[:space:]]*)[^[:space:],;]+`)
	logTelegramBotRE  = regexp.MustCompile(`[0-9]+:[A-Za-z0-9_-]{20,}`)
	logSlackTokenRE   = regexp.MustCompile(`\bxox[bps]-[A-Za-z0-9-]+`)
	logOpenAIKeyRE    = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{14,}`)
)

// RedactLine replaces known secret-shaped byte spans with "[REDACTED]".
// It returns the input slice unchanged when no redactions are applied.
func RedactLine(line []byte) ([]byte, int) {
	out := line
	total := 0

	for _, re := range []*regexp.Regexp{
		logBearerTokenRE,
		logAPIKeyValueRE,
		logXAPIKeyValueRE,
	} {
		var count int
		out, count = redactPrefixedLogSecret(out, re)
		total += count
	}

	for _, re := range []*regexp.Regexp{
		logTelegramBotRE,
		logSlackTokenRE,
		logOpenAIKeyRE,
	} {
		var count int
		out, count = redactWholeLogSecret(out, re)
		total += count
	}

	return out, total
}

func redactPrefixedLogSecret(line []byte, re *regexp.Regexp) ([]byte, int) {
	matches := re.FindAllSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return line, 0
	}

	var b bytes.Buffer
	b.Grow(len(line))
	last := 0
	for _, match := range matches {
		b.Write(line[last:match[3]])
		b.Write(logRedactedMarker)
		last = match[1]
	}
	b.Write(line[last:])
	return b.Bytes(), len(matches)
}

func redactWholeLogSecret(line []byte, re *regexp.Regexp) ([]byte, int) {
	matches := re.FindAllIndex(line, -1)
	if len(matches) == 0 {
		return line, 0
	}

	var b bytes.Buffer
	b.Grow(len(line))
	last := 0
	for _, match := range matches {
		b.Write(line[last:match[0]])
		b.Write(logRedactedMarker)
		last = match[1]
	}
	b.Write(line[last:])
	return b.Bytes(), len(matches)
}
