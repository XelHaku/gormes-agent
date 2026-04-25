package tools

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDiscordToolsetSplitDescriptorsAndPlatformScope(t *testing.T) {
	caps := DiscordApplicationCapabilities{
		Detected:                true,
		HasGuildMembersIntent:   true,
		HasMessageContentIntent: true,
	}
	base := DiscordToolsetOptions{
		Platform:     "discord",
		BotToken:     "bot-token",
		Capabilities: caps,
	}

	if got := DiscordToolsetDescriptors(base); len(got) != 0 {
		t.Fatalf("default descriptors = %v, want none until discord toolsets are explicitly enabled", descriptorNames(got))
	}

	enabled := base
	enabled.EnabledToolsets = []string{DiscordToolsetCore, DiscordToolsetAdmin}
	descriptors := DiscordToolsetDescriptors(enabled)
	if got, want := descriptorNames(descriptors), []string{DiscordToolName, DiscordAdminToolName}; !reflect.DeepEqual(got, want) {
		t.Fatalf("descriptor names = %v, want %v", got, want)
	}
	if containsString(descriptorNames(descriptors), "discord_server") {
		t.Fatal("legacy discord_server must not be advertised as an active descriptor")
	}

	core := mustDescriptor(t, descriptors, DiscordToolName)
	assertActionEnum(t, core.Schema, []string{"search_members", "fetch_messages", "create_thread"})
	assertDescriptionContains(t, core.Description, "fetch_messages(channel_id)")
	assertDescriptionContains(t, core.Description, "search_members(guild_id, query)")
	assertDescriptionContains(t, core.Description, "create_thread(channel_id, name)")
	assertDescriptionOmits(t, core.Description, "list_guilds()")
	assertDescriptionOmits(t, core.Description, "add_role(")

	admin := mustDescriptor(t, descriptors, DiscordAdminToolName)
	assertActionEnum(t, admin.Schema, []string{
		"list_guilds",
		"server_info",
		"list_channels",
		"channel_info",
		"list_roles",
		"member_info",
		"list_pins",
		"pin_message",
		"unpin_message",
		"add_role",
		"remove_role",
	})
	assertDescriptionContains(t, admin.Description, "list_guilds()")
	assertDescriptionContains(t, admin.Description, "add_role(guild_id, user_id, role_id)")
	assertDescriptionOmits(t, admin.Description, "fetch_messages(")
	assertDescriptionOmits(t, admin.Description, "create_thread(")

	for _, platform := range []string{"cli", "telegram", "slack"} {
		scoped := enabled
		scoped.Platform = platform
		if got := DiscordToolsetDescriptors(scoped); len(got) != 0 {
			t.Fatalf("%s descriptors = %v, want no Discord descriptors outside Discord/gateway scope", platform, descriptorNames(got))
		}
	}

	gateway := enabled
	gateway.Platform = "gateway"
	gateway.EnabledToolsets = []string{DiscordToolsetCore}
	if got, want := descriptorNames(DiscordToolsetDescriptors(gateway)), []string{DiscordToolName}; !reflect.DeepEqual(got, want) {
		t.Fatalf("gateway descriptors = %v, want %v", got, want)
	}
}

func TestDiscordToolsetDropsSchemasForMissingTokenIntentsAndAllowlist(t *testing.T) {
	fullCaps := DiscordApplicationCapabilities{
		Detected:                true,
		HasGuildMembersIntent:   true,
		HasMessageContentIntent: true,
	}
	base := DiscordToolsetOptions{
		Platform:        "discord",
		EnabledToolsets: []string{DiscordToolsetCore, DiscordToolsetAdmin},
		BotToken:        "bot-token",
		Capabilities:    fullCaps,
	}

	missingToken := base
	missingToken.BotToken = ""
	if got := DiscordToolsetDescriptors(missingToken); len(got) != 0 {
		t.Fatalf("missing token descriptors = %v, want none", descriptorNames(got))
	}
	assertStatus(t, DiscordToolsetStatuses(missingToken), DiscordToolName, DiscordToolStatusMissingToken)
	assertStatus(t, DiscordToolsetStatuses(missingToken), DiscordAdminToolName, DiscordToolStatusMissingToken)

	noMembers := base
	noMembers.Capabilities.HasGuildMembersIntent = false
	descriptors := DiscordToolsetDescriptors(noMembers)
	core := mustDescriptor(t, descriptors, DiscordToolName)
	assertActionEnum(t, core.Schema, []string{"fetch_messages", "create_thread"})
	assertDescriptionOmits(t, core.Description, "search_members")
	assertSchemaOmitsProperties(t, core.Schema, "query")
	admin := mustDescriptor(t, descriptors, DiscordAdminToolName)
	assertActionEnum(t, admin.Schema, []string{
		"list_guilds",
		"server_info",
		"list_channels",
		"channel_info",
		"list_roles",
		"list_pins",
		"pin_message",
		"unpin_message",
		"add_role",
		"remove_role",
	})
	assertDescriptionOmits(t, admin.Description, "member_info")

	noContent := base
	noContent.Capabilities.HasMessageContentIntent = false
	core = mustDescriptor(t, DiscordToolsetDescriptors(noContent), DiscordToolName)
	assertActionEnum(t, core.Schema, []string{"search_members", "fetch_messages", "create_thread"})
	assertDescriptionContains(t, core.Description, "MESSAGE_CONTENT")

	adminOnlyAllowlist := base
	adminOnlyAllowlist.ActionAllowlistSet = true
	adminOnlyAllowlist.AllowedActions = []string{"list_guilds", "pin_message"}
	descriptors = DiscordToolsetDescriptors(adminOnlyAllowlist)
	if names := descriptorNames(descriptors); !reflect.DeepEqual(names, []string{DiscordAdminToolName}) {
		t.Fatalf("admin allowlist descriptor names = %v, want only discord_admin", names)
	}
	admin = mustDescriptor(t, descriptors, DiscordAdminToolName)
	assertActionEnum(t, admin.Schema, []string{"list_guilds", "pin_message"})
	assertDescriptionContains(t, admin.Description, "list_guilds()")
	assertDescriptionContains(t, admin.Description, "pin_message(channel_id, message_id)")
	assertDescriptionOmits(t, admin.Description, "list_channels(")
	assertDescriptionOmits(t, admin.Description, "add_role(")
	assertSchemaOmitsProperties(t, admin.Schema, "guild_id", "role_id", "user_id")

	emptyAllowlist := base
	emptyAllowlist.ActionAllowlistSet = true
	emptyAllowlist.AllowedActions = nil
	if got := DiscordToolsetDescriptors(emptyAllowlist); len(got) != 0 {
		t.Fatalf("empty allowlist descriptors = %v, want both Discord schemas dropped", descriptorNames(got))
	}
	assertStatus(t, DiscordToolsetStatuses(emptyAllowlist), DiscordToolName, DiscordToolStatusNoActions)
	assertStatus(t, DiscordToolsetStatuses(emptyAllowlist), DiscordAdminToolName, DiscordToolStatusNoActions)
}

func descriptorNames(descriptors []ToolDescriptor) []string {
	names := make([]string, len(descriptors))
	for i, descriptor := range descriptors {
		names[i] = descriptor.Name
	}
	return names
}

func mustDescriptor(t *testing.T, descriptors []ToolDescriptor, name string) ToolDescriptor {
	t.Helper()
	for _, descriptor := range descriptors {
		if descriptor.Name == name {
			return descriptor
		}
	}
	t.Fatalf("missing descriptor %q in %v", name, descriptorNames(descriptors))
	return ToolDescriptor{}
}

func assertActionEnum(t *testing.T, raw json.RawMessage, want []string) {
	t.Helper()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	var action struct {
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(schema.Properties["action"], &action); err != nil {
		t.Fatalf("unmarshal action property: %v", err)
	}
	got := action.Enum
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("action enum = %v, want %v", got, want)
	}
}

func assertDescriptionContains(t *testing.T, description, want string) {
	t.Helper()
	if !strings.Contains(description, want) {
		t.Fatalf("description does not contain %q:\n%s", want, description)
	}
}

func assertDescriptionOmits(t *testing.T, description, unwanted string) {
	t.Helper()
	if strings.Contains(description, unwanted) {
		t.Fatalf("description contains stale action %q:\n%s", unwanted, description)
	}
}

func assertSchemaOmitsProperties(t *testing.T, raw json.RawMessage, names ...string) {
	t.Helper()
	properties := schemaProperties(t, raw)
	for _, name := range names {
		if _, ok := properties[name]; ok {
			t.Fatalf("schema still advertises property %q in %v", name, propertyNames(properties))
		}
	}
}

func schemaProperties(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return schema.Properties
}

func propertyNames(properties map[string]json.RawMessage) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	return names
}

func assertStatus(t *testing.T, statuses []DiscordToolsetStatus, name string, want DiscordToolStatus) {
	t.Helper()
	for _, status := range statuses {
		if status.Name == name {
			if status.Status != want {
				t.Fatalf("%s status = %q, want %q (%s)", name, status.Status, want, status.Reason)
			}
			return
		}
	}
	t.Fatalf("missing status for %s in %#v", name, statuses)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
