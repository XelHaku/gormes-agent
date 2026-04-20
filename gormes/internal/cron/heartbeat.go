package cron

import "strings"

// CronHeartbeatPrefix is the verbatim port of upstream Hermes'
// cron/scheduler.py cron_hint. Prepended to every scheduled-job
// prompt so the LLM knows:
//   - Its output is auto-delivered — don't call send_message
//   - It can return exactly "[SILENT]" (and nothing else) to
//     suppress delivery
//
// Matching upstream byte-for-byte matters: if a future Hermes bump
// changes the wording, we want to notice (the byte-match test
// TestHeartbeatPrefix_ContainsLoadBearingPhrases flags drift on
// any of the load-bearing phrases).
const CronHeartbeatPrefix = "[SYSTEM: You are running as a scheduled cron job. " +
	"DELIVERY: Your final response will be automatically delivered " +
	"to the user — do NOT use send_message or try to deliver " +
	"the output yourself. Just produce your report/output as your " +
	"final response and the system handles the rest. " +
	"SILENT: If there is genuinely nothing new to report, respond " +
	"with exactly \"[SILENT]\" (nothing else) to suppress delivery. " +
	"Never combine [SILENT] with content — either report your " +
	"findings normally, or say [SILENT] and nothing more.]\n\n"

// BuildPrompt prepends the cron heartbeat prefix to the operator's
// prompt. The concatenated result is what the kernel sees as the
// user message for the cron turn.
func BuildPrompt(userPrompt string) string {
	return CronHeartbeatPrefix + userPrompt
}

// DetectSilent returns true ONLY when the final response, after
// TrimSpace, equals the literal "[SILENT]" token. Substring matches,
// alternate casings, and responses that explain the token all return
// false — that's intentional. See spec §7.2.
//
// A false return means "deliver normally" (which is the right default
// for any ambiguous output — the operator would rather see a weird
// message than silently miss one).
func DetectSilent(finalResponse string) bool {
	return strings.TrimSpace(finalResponse) == "[SILENT]"
}
