package contextengine

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
)

const (
	DefaultThresholdPercent              = 0.50
	DefaultTargetRatio                   = 0.20
	DefaultFallbackContext               = 128_000
	MinimumContextLength                 = 64_000
	maxSummaryContextFraction            = 0.05
	minSummaryTokens                     = 2_000
	summaryRatio                         = 0.20
	summaryTokensCeiling                 = 12_000
	ineffectiveCompressionSavingsPercent = 10.0
	ineffectiveCompressionLimit          = 2
)

var contextProbeTiers = []int{128_000, 64_000, 32_000, 16_000, 8_000}

type Config struct {
	ContextLength    int
	ThresholdPercent float64
	TargetRatio      float64
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
}

type Status struct {
	LastPromptTokens            int  `json:"last_prompt_tokens"`
	LastCompletionTokens        int  `json:"last_completion_tokens"`
	LastTotalTokens             int  `json:"last_total_tokens"`
	ThresholdTokens             int  `json:"threshold_tokens"`
	ContextLength               int  `json:"context_length"`
	CompressionCount            int  `json:"compression_count"`
	TailTokenBudget             int  `json:"tail_token_budget"`
	MaxSummaryTokens            int  `json:"max_summary_tokens"`
	ContextProbed               bool `json:"context_probed"`
	IneffectiveCompressionCount int  `json:"ineffective_compression_count"`
}

type RequestPlan struct {
	Messages        []hermes.Message
	EstimatedTokens int
	DroppedMessages int
}

// Compressor carries the provider-free budget math needed before the later
// context-engine and summarizer slices get wired into the kernel loop.
type Compressor struct {
	contextLength               int
	thresholdPercent            float64
	targetRatio                 float64
	thresholdTokens             int
	tailTokenBudget             int
	maxSummaryTokens            int
	lastPromptTokens            int
	lastCompletionTokens        int
	lastTotalTokens             int
	compressionCount            int
	contextProbed               bool
	lastCompressionSavingsPct   float64
	ineffectiveCompressionCount int
}

func NewCompressor(cfg Config) *Compressor {
	thresholdPercent := cfg.ThresholdPercent
	if thresholdPercent <= 0 {
		thresholdPercent = DefaultThresholdPercent
	}
	targetRatio := cfg.TargetRatio
	if targetRatio <= 0 {
		targetRatio = DefaultTargetRatio
	}
	targetRatio = clamp(targetRatio, 0.10, 0.80)

	contextLength := cfg.ContextLength
	if contextLength <= 0 {
		contextLength = DefaultFallbackContext
	}

	c := &Compressor{
		contextLength:             contextLength,
		thresholdPercent:          thresholdPercent,
		targetRatio:               targetRatio,
		lastCompressionSavingsPct: 100.0,
	}
	c.recomputeBudgets()
	return c
}

func (c *Compressor) ContextLength() int { return c.contextLength }

func (c *Compressor) ThresholdTokens() int { return c.thresholdTokens }

func (c *Compressor) TailTokenBudget() int { return c.tailTokenBudget }

func (c *Compressor) MaxSummaryTokens() int { return c.maxSummaryTokens }

func (c *Compressor) ContextProbed() bool { return c.contextProbed }

func (c *Compressor) CompressionCount() int { return c.compressionCount }

func (c *Compressor) IneffectiveCompressionCount() int { return c.ineffectiveCompressionCount }

func (c *Compressor) UpdateFromResponse(usage Usage) {
	c.lastPromptTokens = usage.PromptTokens
	c.lastCompletionTokens = usage.CompletionTokens
	c.lastTotalTokens = usage.PromptTokens + usage.CompletionTokens
}

func (c *Compressor) UpdateModelContext(contextLength int) {
	if contextLength <= 0 {
		return
	}
	c.contextLength = contextLength
	c.contextProbed = false
	c.recomputeBudgets()
}

func (c *Compressor) Status() Status {
	return Status{
		LastPromptTokens:            c.lastPromptTokens,
		LastCompletionTokens:        c.lastCompletionTokens,
		LastTotalTokens:             c.lastTotalTokens,
		ThresholdTokens:             c.thresholdTokens,
		ContextLength:               c.contextLength,
		CompressionCount:            c.compressionCount,
		TailTokenBudget:             c.tailTokenBudget,
		MaxSummaryTokens:            c.maxSummaryTokens,
		ContextProbed:               c.contextProbed,
		IneffectiveCompressionCount: c.ineffectiveCompressionCount,
	}
}

func (c *Compressor) HandleToolCall(name string, _ json.RawMessage) (json.RawMessage, error) {
	switch strings.TrimSpace(name) {
	case "get_status":
		return json.Marshal(c.Status())
	default:
		return json.Marshal(map[string]string{
			"error": fmt.Sprintf("unknown tool: %q", name),
		})
	}
}

func (c *Compressor) ShouldCompress(promptTokens int) bool {
	tokens := promptTokens
	if tokens <= 0 {
		tokens = c.lastPromptTokens
	}
	if tokens < c.thresholdTokens {
		return false
	}
	if c.ineffectiveCompressionCount >= ineffectiveCompressionLimit {
		return false
	}
	return true
}

func (c *Compressor) SummaryBudget(contentTokens int) int {
	if contentTokens <= 0 {
		return minSummaryTokens
	}
	budget := int(float64(contentTokens) * summaryRatio)
	if budget < minSummaryTokens {
		return minSummaryTokens
	}
	if budget > c.maxSummaryTokens {
		return c.maxSummaryTokens
	}
	return budget
}

func (c *Compressor) RecordCompression(beforePromptTokens, afterPromptTokens int) {
	c.compressionCount++
	savingsPct := 0.0
	if beforePromptTokens > 0 && afterPromptTokens < beforePromptTokens {
		savingsPct = float64(beforePromptTokens-afterPromptTokens) / float64(beforePromptTokens) * 100
	}
	c.lastCompressionSavingsPct = savingsPct
	if savingsPct < ineffectiveCompressionSavingsPercent {
		c.ineffectiveCompressionCount++
		return
	}
	c.ineffectiveCompressionCount = 0
}

func (c *Compressor) StepDownContextLength() bool {
	next, ok := nextProbeTier(c.contextLength)
	if !ok {
		return false
	}
	c.contextLength = next
	c.contextProbed = true
	c.recomputeBudgets()
	return true
}

func (c *Compressor) PlanMessages(systemMsgs, history []hermes.Message) RequestPlan {
	plan := RequestPlan{
		Messages: append([]hermes.Message(nil), systemMsgs...),
	}
	plan.EstimatedTokens = estimateMessagesTokens(plan.Messages)
	if len(history) == 0 {
		return plan
	}

	groups := historyGroups(history)
	selected := make([]messageGroup, 0, len(groups))
	selectedTokens := 0
	for i, group := range groups {
		if i == 0 || plan.EstimatedTokens+selectedTokens+group.tokens <= c.thresholdTokens {
			selected = append(selected, group)
			selectedTokens += group.tokens
			continue
		}
		for _, dropped := range groups[i:] {
			plan.DroppedMessages += len(dropped.messages)
		}
		break
	}

	plan.Messages = appendSelectedGroups(plan.Messages, selected)
	plan.EstimatedTokens += selectedTokens
	return plan
}

func (c *Compressor) recomputeBudgets() {
	c.thresholdTokens = thresholdTokensFor(c.contextLength, c.thresholdPercent)
	c.tailTokenBudget = int(float64(c.thresholdTokens) * c.targetRatio)
	c.maxSummaryTokens = maxSummaryBudgetFor(c.contextLength)
}

func nextProbeTier(current int) (int, bool) {
	for _, tier := range contextProbeTiers {
		if tier < current {
			return tier, true
		}
	}
	return 0, false
}

func clamp(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func thresholdTokensFor(contextLength int, thresholdPercent float64) int {
	threshold := int(float64(contextLength) * thresholdPercent)
	if threshold < MinimumContextLength {
		return MinimumContextLength
	}
	return threshold
}

func maxSummaryBudgetFor(contextLength int) int {
	maxSummary := int(float64(contextLength) * maxSummaryContextFraction)
	if maxSummary > summaryTokensCeiling {
		return summaryTokensCeiling
	}
	return maxSummary
}

const estimatedMessageOverheadTokens = 16

type messageGroup struct {
	messages []hermes.Message
	tokens   int
}

func historyGroups(history []hermes.Message) []messageGroup {
	groups := make([]messageGroup, 0, len(history))
	for i := len(history) - 1; i >= 0; {
		if i == len(history)-1 {
			groups = append(groups, buildGroup(history[i]))
			i--
			continue
		}
		if history[i].Role == "assistant" && i-1 >= 0 && history[i-1].Role == "user" {
			groups = append(groups, buildGroup(history[i-1], history[i]))
			i -= 2
			continue
		}
		groups = append(groups, buildGroup(history[i]))
		i--
	}
	return groups
}

func buildGroup(messages ...hermes.Message) messageGroup {
	group := messageGroup{
		messages: append([]hermes.Message(nil), messages...),
	}
	group.tokens = estimateMessagesTokens(group.messages)
	return group
}

func estimateMessagesTokens(messages []hermes.Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateMessageTokens(msg)
	}
	return total
}

func appendSelectedGroups(dst []hermes.Message, selected []messageGroup) []hermes.Message {
	for i := len(selected) - 1; i >= 0; i-- {
		dst = append(dst, selected[i].messages...)
	}
	return dst
}

func estimateMessageTokens(msg hermes.Message) int {
	total := estimatedMessageOverheadTokens
	total += utf8.RuneCountInString(msg.Role)
	total += utf8.RuneCountInString(msg.Content)
	total += utf8.RuneCountInString(msg.ToolCallID)
	total += utf8.RuneCountInString(msg.Name)
	for _, tc := range msg.ToolCalls {
		total += estimatedMessageOverheadTokens
		total += utf8.RuneCountInString(tc.ID)
		total += utf8.RuneCountInString(tc.Name)
		total += utf8.RuneCount(tc.Arguments)
	}
	return total
}
