package tools

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

const (
	discordLimitMinimum = 1
	discordLimitMaximum = 100

	discordSearchMembersDefaultLimit = 20
	discordFetchMessagesDefaultLimit = 50
)

// DiscordLimitEvidence is the stable evidence label returned with normalized
// Discord limit values.
type DiscordLimitEvidence string

const (
	DiscordLimitEvidenceProvided  DiscordLimitEvidence = "discord_limit_provided"
	DiscordLimitEvidenceClamped   DiscordLimitEvidence = "discord_limit_clamped"
	DiscordLimitEvidenceDefaulted DiscordLimitEvidence = "discord_limit_defaulted"
)

// DiscordLimitNormalization is the bounded value future Discord REST handlers
// should use, plus evidence for operator-visible degraded input reporting.
type DiscordLimitNormalization struct {
	Limit    int
	Evidence DiscordLimitEvidence
}

// NormalizeDiscordLimit coerces the model-provided limit argument for Discord
// actions that expose bounded result limits.
func NormalizeDiscordLimit(action string, arguments map[string]any) DiscordLimitNormalization {
	defaultLimit := discordDefaultLimit(action)
	raw, ok := arguments["limit"]
	if !ok || raw == nil {
		return DiscordLimitNormalization{Limit: defaultLimit, Evidence: DiscordLimitEvidenceDefaulted}
	}

	limit, ok := coerceDiscordLimit(raw)
	if !ok {
		return DiscordLimitNormalization{Limit: defaultLimit, Evidence: DiscordLimitEvidenceDefaulted}
	}
	if limit < discordLimitMinimum {
		return DiscordLimitNormalization{Limit: discordLimitMinimum, Evidence: DiscordLimitEvidenceClamped}
	}
	if limit > discordLimitMaximum {
		return DiscordLimitNormalization{Limit: discordLimitMaximum, Evidence: DiscordLimitEvidenceClamped}
	}
	return DiscordLimitNormalization{Limit: limit, Evidence: DiscordLimitEvidenceProvided}
}

func discordDefaultLimit(action string) int {
	switch action {
	case "search_members":
		return discordSearchMembersDefaultLimit
	case "fetch_messages":
		return discordFetchMessagesDefaultLimit
	default:
		return discordFetchMessagesDefaultLimit
	}
}

func coerceDiscordLimit(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int8:
		return int(value), true
	case int16:
		return int(value), true
	case int32:
		return int(value), true
	case int64:
		return clampInt64ToInt(value), true
	case uint:
		return clampUint64ToInt(uint64(value)), true
	case uint8:
		return int(value), true
	case uint16:
		return int(value), true
	case uint32:
		return clampUint64ToInt(uint64(value)), true
	case uint64:
		return clampUint64ToInt(value), true
	case float32:
		return coerceDiscordFloat(float64(value))
	case float64:
		return coerceDiscordFloat(value)
	case string:
		return coerceDiscordString(value)
	case json.Number:
		return coerceDiscordJSONNumber(value)
	default:
		return 0, false
	}
}

func coerceDiscordString(value string) (int, bool) {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return limit, true
}

func coerceDiscordJSONNumber(value json.Number) (int, bool) {
	if limit, err := value.Int64(); err == nil {
		return clampInt64ToInt(limit), true
	}
	limit, err := value.Float64()
	if err != nil {
		return 0, false
	}
	return coerceDiscordFloat(limit)
}

func coerceDiscordFloat(value float64) (int, bool) {
	if math.IsNaN(value) {
		return 0, false
	}
	if math.IsInf(value, 1) {
		return discordLimitMaximum + 1, true
	}
	if math.IsInf(value, -1) {
		return discordLimitMinimum - 1, true
	}
	if value < discordLimitMinimum {
		return discordLimitMinimum - 1, true
	}
	if value > discordLimitMaximum {
		return discordLimitMaximum + 1, true
	}
	if value != math.Trunc(value) {
		return 0, false
	}
	if value > float64(math.MaxInt) {
		return math.MaxInt, true
	}
	if value < float64(math.MinInt) {
		return math.MinInt, true
	}
	return int(value), true
}

func clampInt64ToInt(value int64) int {
	if value > int64(math.MaxInt) {
		return math.MaxInt
	}
	if value < int64(math.MinInt) {
		return math.MinInt
	}
	return int(value)
}

func clampUint64ToInt(value uint64) int {
	if value > uint64(math.MaxInt) {
		return math.MaxInt
	}
	return int(value)
}
