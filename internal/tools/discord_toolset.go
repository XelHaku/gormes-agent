package tools

import (
	"encoding/json"
	"strings"
)

const (
	DiscordToolName      = "discord"
	DiscordAdminToolName = "discord_admin"

	DiscordToolsetCore  = "discord"
	DiscordToolsetAdmin = "discord_admin"
)

// DiscordToolStatus is the descriptor-level availability state for Discord
// toolsets. Unavailable states do not advertise a schema to the model.
type DiscordToolStatus string

const (
	DiscordToolStatusAvailable                DiscordToolStatus = "available"
	DiscordToolStatusDisabled                 DiscordToolStatus = "disabled"
	DiscordToolStatusMissingToken             DiscordToolStatus = "missing_token"
	DiscordToolStatusUnavailablePlatformScope DiscordToolStatus = "unavailable_platform_scope"
	DiscordToolStatusNoActions                DiscordToolStatus = "no_available_actions"
)

// DiscordApplicationCapabilities is the intent snapshot used to build
// descriptor schemas without making live Discord API calls.
type DiscordApplicationCapabilities struct {
	Detected                bool
	HasGuildMembersIntent   bool
	HasMessageContentIntent bool
}

// DiscordToolsetOptions controls Discord descriptor availability.
type DiscordToolsetOptions struct {
	Platform           string
	EnabledToolsets    []string
	BotToken           string
	Capabilities       DiscordApplicationCapabilities
	AllowedActions     []string
	ActionAllowlistSet bool
}

// DiscordToolsetStatus reports whether one Discord toolset will advertise a
// model-visible descriptor and why unavailable schemas were dropped.
type DiscordToolsetStatus struct {
	Name       string
	Toolset    string
	Status     DiscordToolStatus
	Reason     string
	Actions    []string
	Descriptor *ToolDescriptor
}

type discordActionClass string

const (
	discordActionCore  discordActionClass = "core"
	discordActionAdmin discordActionClass = "admin"
)

type discordActionSpec struct {
	Name                  string
	Signature             string
	Description           string
	Class                 discordActionClass
	RequiresMembersIntent bool
}

var discordActionManifest = []discordActionSpec{
	{Name: "list_guilds", Signature: "()", Description: "list servers the bot is in", Class: discordActionAdmin},
	{Name: "server_info", Signature: "(guild_id)", Description: "server details + member counts", Class: discordActionAdmin},
	{Name: "list_channels", Signature: "(guild_id)", Description: "all channels grouped by category", Class: discordActionAdmin},
	{Name: "channel_info", Signature: "(channel_id)", Description: "single channel details", Class: discordActionAdmin},
	{Name: "list_roles", Signature: "(guild_id)", Description: "roles sorted by position", Class: discordActionAdmin},
	{Name: "member_info", Signature: "(guild_id, user_id)", Description: "lookup a specific member", Class: discordActionAdmin, RequiresMembersIntent: true},
	{Name: "search_members", Signature: "(guild_id, query)", Description: "find members by name prefix", Class: discordActionCore, RequiresMembersIntent: true},
	{Name: "fetch_messages", Signature: "(channel_id)", Description: "recent messages; optional before/after snowflakes", Class: discordActionCore},
	{Name: "list_pins", Signature: "(channel_id)", Description: "pinned messages in a channel", Class: discordActionAdmin},
	{Name: "pin_message", Signature: "(channel_id, message_id)", Description: "pin a message", Class: discordActionAdmin},
	{Name: "unpin_message", Signature: "(channel_id, message_id)", Description: "unpin a message", Class: discordActionAdmin},
	{Name: "create_thread", Signature: "(channel_id, name)", Description: "create a public thread; optional message_id anchor", Class: discordActionCore},
	{Name: "add_role", Signature: "(guild_id, user_id, role_id)", Description: "assign a role", Class: discordActionAdmin},
	{Name: "remove_role", Signature: "(guild_id, user_id, role_id)", Description: "remove a role", Class: discordActionAdmin},
}

// DiscordToolsetAllowedForPlatform reports whether a Discord toolset may be
// configured for a platform. The toolsets are still default-off on allowed
// platforms and must be explicitly enabled by the caller.
func DiscordToolsetAllowedForPlatform(toolset, platform string) bool {
	switch normalizeToolsetName(toolset) {
	case DiscordToolsetCore, DiscordToolsetAdmin:
		p := strings.ToLower(strings.TrimSpace(platform))
		return p == "discord" || p == "gateway"
	default:
		return false
	}
}

// DiscordToolsetDescriptors returns the model-visible Discord descriptors for
// the supplied platform/toolset/capability snapshot.
func DiscordToolsetDescriptors(opts DiscordToolsetOptions) []ToolDescriptor {
	statuses := DiscordToolsetStatuses(opts)
	out := make([]ToolDescriptor, 0, len(statuses))
	for _, status := range statuses {
		if status.Descriptor != nil {
			out = append(out, *status.Descriptor)
		}
	}
	return out
}

// DiscordToolsetStatuses returns descriptor availability for both split
// Discord toolsets in deterministic descriptor order.
func DiscordToolsetStatuses(opts DiscordToolsetOptions) []DiscordToolsetStatus {
	enabled := normalizedStringSet(opts.EnabledToolsets)
	statuses := make([]DiscordToolsetStatus, 0, 2)
	for _, spec := range []struct {
		name    string
		toolset string
		class   discordActionClass
	}{
		{name: DiscordToolName, toolset: DiscordToolsetCore, class: discordActionCore},
		{name: DiscordAdminToolName, toolset: DiscordToolsetAdmin, class: discordActionAdmin},
	} {
		status := DiscordToolsetStatus{Name: spec.name, Toolset: spec.toolset}
		if !DiscordToolsetAllowedForPlatform(spec.toolset, opts.Platform) {
			status.Status = DiscordToolStatusUnavailablePlatformScope
			status.Reason = "discord toolsets are available only for the Discord or gateway platform scope"
			statuses = append(statuses, status)
			continue
		}
		if !enabled[spec.toolset] {
			status.Status = DiscordToolStatusDisabled
			status.Reason = "discord toolset is default-off and was not explicitly enabled"
			statuses = append(statuses, status)
			continue
		}
		if strings.TrimSpace(opts.BotToken) == "" {
			status.Status = DiscordToolStatusMissingToken
			status.Reason = "missing Discord bot token"
			statuses = append(statuses, status)
			continue
		}
		actions := availableDiscordActions(spec.class, opts)
		if len(actions) == 0 {
			status.Status = DiscordToolStatusNoActions
			status.Reason = "all actions were removed by Discord intent or allowlist filters"
			statuses = append(statuses, status)
			continue
		}
		descriptor := buildDiscordToolDescriptor(spec.name, spec.class, actions, opts.Capabilities)
		status.Status = DiscordToolStatusAvailable
		status.Actions = append([]string(nil), actions...)
		status.Descriptor = &descriptor
		statuses = append(statuses, status)
	}
	return statuses
}

func availableDiscordActions(class discordActionClass, opts DiscordToolsetOptions) []string {
	caps := normalizedDiscordCapabilities(opts.Capabilities)
	allowlist := normalizedStringSet(opts.AllowedActions)
	actions := make([]string, 0, len(discordActionManifest))
	for _, action := range discordActionManifest {
		if action.Class != class {
			continue
		}
		if action.RequiresMembersIntent && !caps.HasGuildMembersIntent {
			continue
		}
		if opts.ActionAllowlistSet && !allowlist[action.Name] {
			continue
		}
		actions = append(actions, action.Name)
	}
	return actions
}

func normalizedDiscordCapabilities(caps DiscordApplicationCapabilities) DiscordApplicationCapabilities {
	if !caps.Detected && !caps.HasGuildMembersIntent && !caps.HasMessageContentIntent {
		caps.HasGuildMembersIntent = true
		caps.HasMessageContentIntent = true
	}
	return caps
}

func buildDiscordToolDescriptor(name string, class discordActionClass, actions []string, caps DiscordApplicationCapabilities) ToolDescriptor {
	description := buildDiscordDescription(name, class, actions, caps)
	schema := map[string]any{
		"type":       "object",
		"properties": discordSchemaProperties(actions),
		"required":   []string{"action"},
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	return ToolDescriptor{Name: name, Description: description, Schema: raw}
}

func discordSchemaProperties(actions []string) map[string]any {
	properties := map[string]any{
		"action": map[string]any{
			"type": "string",
			"enum": actions,
		},
	}
	if hasAnyDiscordAction(actions, "server_info", "list_channels", "list_roles", "member_info", "search_members", "add_role", "remove_role") {
		properties["guild_id"] = map[string]any{
			"type":        "string",
			"description": "Discord server (guild) ID.",
		}
	}
	if hasAnyDiscordAction(actions, "channel_info", "fetch_messages", "list_pins", "pin_message", "unpin_message", "create_thread") {
		properties["channel_id"] = map[string]any{
			"type":        "string",
			"description": "Discord channel ID.",
		}
	}
	if hasAnyDiscordAction(actions, "member_info", "add_role", "remove_role") {
		properties["user_id"] = map[string]any{
			"type":        "string",
			"description": "Discord user ID.",
		}
	}
	if hasAnyDiscordAction(actions, "add_role", "remove_role") {
		properties["role_id"] = map[string]any{
			"type":        "string",
			"description": "Discord role ID.",
		}
	}
	if hasAnyDiscordAction(actions, "pin_message", "unpin_message", "create_thread") {
		properties["message_id"] = map[string]any{
			"type":        "string",
			"description": "Discord message ID.",
		}
	}
	if hasAnyDiscordAction(actions, "search_members") {
		properties["query"] = map[string]any{
			"type":        "string",
			"description": "Member name prefix to search for.",
		}
	}
	if hasAnyDiscordAction(actions, "create_thread") {
		properties["name"] = map[string]any{
			"type":        "string",
			"description": "New thread name.",
		}
		properties["auto_archive_duration"] = map[string]any{
			"type":        "integer",
			"enum":        []int{60, 1440, 4320, 10080},
			"description": "Thread archive duration in minutes.",
		}
	}
	if hasAnyDiscordAction(actions, "fetch_messages", "search_members") {
		properties["limit"] = map[string]any{
			"type":        "integer",
			"minimum":     discordLimitMinimum,
			"maximum":     discordLimitMaximum,
			"description": "Max results to return.",
		}
	}
	if hasAnyDiscordAction(actions, "fetch_messages") {
		properties["before"] = map[string]any{
			"type":        "string",
			"description": "Snowflake ID for reverse pagination.",
		}
		properties["after"] = map[string]any{
			"type":        "string",
			"description": "Snowflake ID for forward pagination.",
		}
	}
	return properties
}

func buildDiscordDescription(name string, class discordActionClass, actions []string, caps DiscordApplicationCapabilities) string {
	lines := make([]string, 0, len(actions))
	selected := normalizedStringSet(actions)
	for _, action := range discordActionManifest {
		if action.Class != class || !selected[action.Name] {
			continue
		}
		lines = append(lines, "  "+action.Name+action.Signature+" - "+action.Description)
	}

	var b strings.Builder
	if name == DiscordAdminToolName {
		b.WriteString("Manage a Discord server via the REST API.\n\n")
		b.WriteString("Available actions:\n")
		b.WriteString(strings.Join(lines, "\n"))
		if selected["list_guilds"] && selected["list_channels"] {
			b.WriteString("\n\nCall list_guilds first to discover guild_ids, then list_channels for channel_ids.")
		} else if selected["list_guilds"] {
			b.WriteString("\n\nCall list_guilds first to discover guild_ids.")
		}
		if selected["add_role"] || selected["remove_role"] {
			b.WriteString(" Runtime errors will tell you if the bot lacks a specific per-guild permission such as MANAGE_ROLES.")
		} else {
			b.WriteString(" Runtime errors will report missing per-guild permissions.")
		}
	} else {
		b.WriteString("Read and participate in a Discord server.\n\n")
		b.WriteString("Available actions:\n")
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteString("\n\nUse the channel_id from the current conversation context.")
		if selected["search_members"] {
			b.WriteString(" Use search_members to look up user IDs by name prefix.")
		}
	}

	caps = normalizedDiscordCapabilities(caps)
	if caps.Detected && !caps.HasMessageContentIntent && hasAnyDiscordAction(actions, "fetch_messages", "list_pins") {
		b.WriteString("\n\nNOTE: Bot does not have the MESSAGE_CONTENT privileged intent. Message content may be empty outside direct mentions or DMs, but metadata remains available.")
	}
	return b.String()
}

func hasAnyDiscordAction(actions []string, names ...string) bool {
	set := normalizedStringSet(actions)
	for _, name := range names {
		if set[name] {
			return true
		}
	}
	return false
}

func normalizedStringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		normalized := normalizeToolsetName(value)
		if normalized != "" {
			out[normalized] = true
		}
	}
	return out
}

func normalizeToolsetName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
