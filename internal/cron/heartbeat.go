package cron

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// CronHeartbeatPrefix is the verbatim port of upstream Hermes'
// cron/scheduler.py cron_hint. Prepended to every scheduled-job
// prompt so the LLM knows:
//   - Its output is auto-delivered — don't call send_message
//   - It can return exactly "[SILENT]" (and nothing else) to
//     suppress delivery
//
// Matching upstream byte-for-byte matters: if a future Hermes bump
// changes the wording, we want to notice (the byte-match test
// TestCronHeartbeatPrefixConstantValue flags drift on
// any of the load-bearing phrases).
const CronHeartbeatPrefix = "[IMPORTANT: You are running as a scheduled cron job. " +
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

// BuildPromptForJob prepends bounded context_from output before the job prompt,
// then applies the standard cron heartbeat.
func BuildPromptForJob(ctx context.Context, job Job, runStore *RunStore, log *slog.Logger) string {
	body := job.Prompt
	blocks := contextFromBlocks(ctx, job.ContextFrom, runStore, log)
	if len(blocks) > 0 {
		body = strings.Join(blocks, "") + body
	}
	return BuildPrompt(body)
}

const (
	maxContextFromOutputChars = 8000
	contextFromTruncated      = "\n\n[... output truncated ...]"
)

func contextFromBlocks(ctx context.Context, sourceIDs []string, runStore *RunStore, log *slog.Logger) []string {
	if len(sourceIDs) == 0 {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	if runStore == nil {
		log.Warn("cron: context_from skipped because run store is unavailable")
		return nil
	}
	blocks := make([]string, 0, len(sourceIDs))
	for _, rawID := range sourceIDs {
		sourceID := strings.TrimSpace(rawID)
		if !validContextFromJobID(sourceID) {
			log.Warn("cron: context_from skipped invalid job id", "job_id", sourceID)
			continue
		}
		output, ok, err := runStore.LatestCompletedOutput(ctx, sourceID)
		if err != nil {
			log.Warn("cron: context_from output read failed", "job_id", sourceID, "err", err)
			continue
		}
		output = strings.TrimSpace(output)
		if !ok || output == "" {
			log.Info("cron: context_from source has no completed output", "job_id", sourceID)
			continue
		}
		blocks = append(blocks, formatContextFromBlock(sourceID, boundContextFromOutput(output)))
	}
	return blocks
}

func formatContextFromBlock(sourceID, output string) string {
	return fmt.Sprintf(
		"## Output from job '%s'\n"+
			"The following is the most recent output from a preceding cron job. Use it as context for your analysis.\n\n"+
			"```\n%s\n```\n\n",
		sourceID,
		output,
	)
}

func boundContextFromOutput(output string) string {
	if len(output) <= maxContextFromOutputChars {
		return output
	}
	return output[:maxContextFromOutputChars] + contextFromTruncated
}

func validContextFromJobID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
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
