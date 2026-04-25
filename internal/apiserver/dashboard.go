package apiserver

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	pluginmeta "github.com/TrebuchetDynamics/gormes-agent/internal/plugins"
)

const (
	dashboardDefaultSessionLimit = 20
	dashboardMaxSessionLimit     = 100
)

const (
	dashboardPanelBuiltIn           = "built_in"
	dashboardPanelOptional          = "optional"
	dashboardPanelOptionalExtension = "optional_extension"
	dashboardReactViteRuntimeAbsent = "absent"
)

// DashboardModelProvider is the model-picker provider shape consumed by the
// dashboard without importing Hermes' React runtime.
type DashboardModelProvider struct {
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Models      []string `json:"models,omitempty"`
	TotalModels int      `json:"total_models,omitempty"`
	IsCurrent   bool     `json:"is_current,omitempty"`
	Warning     string   `json:"warning,omitempty"`
}

// DashboardOAuthStatus mirrors the dashboard's disconnected/connected status
// without owning provider-specific OAuth flows.
type DashboardOAuthStatus struct {
	LoggedIn        bool   `json:"logged_in"`
	Source          string `json:"source,omitempty"`
	SourceLabel     string `json:"source_label,omitempty"`
	TokenPreview    string `json:"token_preview,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	HasRefreshToken bool   `json:"has_refresh_token,omitempty"`
	LastRefresh     string `json:"last_refresh,omitempty"`
	Error           string `json:"error,omitempty"`
}

// DashboardOAuthProvider describes an OAuth-capable provider and its current
// status as a read-only dashboard contract.
type DashboardOAuthProvider struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Flow       string               `json:"flow"`
	CLICommand string               `json:"cli_command"`
	DocsURL    string               `json:"docs_url"`
	Status     DashboardOAuthStatus `json:"status"`
}

// DashboardSessionInfo is the native session summary shape used by the
// dashboard's session list.
type DashboardSessionInfo struct {
	ID            string  `json:"id"`
	Source        *string `json:"source"`
	Model         *string `json:"model"`
	Title         *string `json:"title"`
	StartedAt     int64   `json:"started_at"`
	EndedAt       *int64  `json:"ended_at"`
	LastActive    int64   `json:"last_active"`
	IsActive      bool    `json:"is_active"`
	MessageCount  int     `json:"message_count"`
	ToolCallCount int     `json:"tool_call_count"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	Preview       *string `json:"preview"`
}

type dashboardPanelStatus struct {
	State     string   `json:"state"`
	Category  string   `json:"category,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Endpoints []string `json:"endpoints,omitempty"`
}

type dashboardExtensionRuntimeStatus struct {
	State            string                `json:"state"`
	Reason           string                `json:"reason,omitempty"`
	ReactViteRuntime string                `json:"react_vite_runtime"`
	Evidence         []pluginmeta.Evidence `json:"evidence,omitempty"`
}

type dashboardThemeInventoryStatus struct {
	State    string                 `json:"state"`
	Active   string                 `json:"active,omitempty"`
	Themes   []dashboardThemeStatus `json:"themes"`
	Evidence []pluginmeta.Evidence  `json:"evidence,omitempty"`
}

type dashboardThemeStatus struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
	State       string `json:"state"`
}

type dashboardExtensionStatus struct {
	Runtime       dashboardExtensionRuntimeStatus `json:"runtime"`
	Themes        dashboardThemeInventoryStatus   `json:"themes"`
	UIPlugins     []pluginmeta.PluginStatus       `json:"ui_plugins"`
	BackendRoutes []pluginmeta.CapabilityStatus   `json:"backend_routes"`
}

type dashboardPluginInventoryResponse struct {
	Runtime       dashboardExtensionRuntimeStatus `json:"runtime"`
	Themes        dashboardThemeInventoryStatus   `json:"themes"`
	Plugins       []pluginmeta.PluginStatus       `json:"plugins"`
	Capabilities  []pluginmeta.CapabilityStatus   `json:"capabilities"`
	BackendRoutes []pluginmeta.CapabilityStatus   `json:"backend_routes"`
	Evidence      []pluginmeta.Evidence           `json:"evidence,omitempty"`
}

func (s *Server) handleDashboardStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	activeSessions := 0
	if sessions, total, err := s.responseStore.ListSessions(dashboardMaxSessionLimit, 0, s.now()); err == nil {
		activeSessions = total
		for _, session := range sessions {
			if !session.IsActive {
				activeSessions--
			}
		}
		if activeSessions < 0 {
			activeSessions = 0
		}
	}

	panels := map[string]dashboardPanelStatus{
		"chat":          enabledPanel(dashboardPanelBuiltIn, "/v1/chat/completions"),
		"responses":     enabledPanel(dashboardPanelBuiltIn, "/v1/responses", "/v1/runs", "/v1/runs/{run_id}/events"),
		"sessions":      enabledPanel(dashboardPanelBuiltIn, "/api/sessions", "/api/sessions/{session_id}"),
		"models":        enabledPanel(dashboardPanelBuiltIn, "/v1/models", "/api/model/info", "/api/model/options"),
		"oauth":         enabledPanel(dashboardPanelOptional, "/api/providers/oauth"),
		"tool_progress": enabledPanel(dashboardPanelBuiltIn, "/v1/runs/{run_id}/events"),
		"plugins":       disabledPanel(dashboardPanelOptionalExtension, dashboardPluginPanelReason(s.pluginInventory)),
	}
	if s.loop == nil {
		panels["chat"] = disabledPanel(dashboardPanelBuiltIn, "native turn loop is not configured")
		panels["responses"] = disabledPanel(dashboardPanelBuiltIn, "native turn loop is not configured")
		panels["tool_progress"] = disabledPanel(dashboardPanelBuiltIn, "native turn loop is not configured")
	}
	if len(s.oauthProviders) == 0 {
		panels["oauth"] = disabledPanel(dashboardPanelOptional, "no OAuth providers are configured")
	}
	extensions := dashboardExtensionsFromInventory(s.pluginInventory)

	writeJSON(w, http.StatusOK, map[string]any{
		"active_sessions": activeSessions,
		"version":         "gormes-agent",
		"platform":        "gormes-agent",
		"model":           s.modelName,
		"provider":        s.providerName,
		"panels":          panels,
		"upstream_react_runtime": map[string]any{
			"state":    "absent",
			"required": false,
		},
		"responses":            s.responseHealthStatus(),
		"runs":                 s.runHealthStatus(),
		"plugins":              clonePluginInventory(s.pluginInventory),
		"dashboard_extensions": extensions,
	})
}

func (s *Server) handleDashboardModelInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":                    s.modelName,
		"provider":                 s.providerName,
		"auto_context_length":      0,
		"config_context_length":    0,
		"effective_context_length": 0,
		"capabilities":             map[string]any{"supports_tools": true},
	})
}

func (s *Server) handleDashboardModelOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	if !s.authorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key", "invalid_request_error", "", "invalid_api_key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":     s.modelName,
		"provider":  s.providerName,
		"providers": s.dashboardModelProviders(),
	})
}

func (s *Server) handleDashboardOAuthProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	if !s.authorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key", "invalid_request_error", "", "invalid_api_key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": cloneDashboardOAuthProviders(s.oauthProviders)})
}

func (s *Server) handleDashboardPlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	writeJSON(w, http.StatusOK, dashboardPluginInventoryFromInventory(s.pluginInventory))
}

func (s *Server) handleDashboardSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
		return
	}
	if !s.authorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key", "invalid_request_error", "", "invalid_api_key")
		return
	}
	limit := parseDashboardInt(r.URL.Query().Get("limit"), dashboardDefaultSessionLimit, 1, dashboardMaxSessionLimit)
	offset := parseDashboardInt(r.URL.Query().Get("offset"), 0, 0, 1_000_000)
	sessions, total, err := s.responseStore.ListSessions(limit, offset, s.now())
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "Internal server error: "+err.Error(), "server_error", "", "session_store_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

func (s *Server) handleDashboardSessionByID(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key", "invalid_request_error", "", "invalid_api_key")
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if decoded, err := url.PathUnescape(sessionID); err == nil {
		sessionID = decoded
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeOpenAIError(w, http.StatusNotFound, "Session not found", "invalid_request_error", "", "session_not_found")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		deleted, err := s.responseStore.DeleteSession(sessionID)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, "Internal server error: "+err.Error(), "server_error", "", "session_store_failed")
			return
		}
		if !deleted {
			writeOpenAIError(w, http.StatusNotFound, "Session not found: "+sessionID, "invalid_request_error", "", "session_not_found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session_id": sessionID})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error", "", "method_not_allowed")
	}
}

func (s *Server) dashboardModelProviders() []DashboardModelProvider {
	providers := cloneDashboardModelProviders(s.modelProviders)
	if len(providers) == 0 {
		providers = []DashboardModelProvider{{
			Name:        "Native Gormes",
			Slug:        s.providerName,
			Models:      []string{s.modelName},
			TotalModels: 1,
			IsCurrent:   true,
		}}
	}
	for i := range providers {
		providers[i].Models = append([]string(nil), providers[i].Models...)
		if providers[i].TotalModels == 0 {
			providers[i].TotalModels = len(providers[i].Models)
		}
		if providers[i].Slug == s.providerName {
			providers[i].IsCurrent = true
		}
	}
	return providers
}

func enabledPanel(category string, endpoints ...string) dashboardPanelStatus {
	return dashboardPanelStatus{State: "enabled", Category: category, Endpoints: append([]string(nil), endpoints...)}
}

func disabledPanel(category, reason string) dashboardPanelStatus {
	return dashboardPanelStatus{State: "disabled", Category: category, Reason: reason}
}

func dashboardPluginPanelReason(inventory pluginmeta.Inventory) string {
	if len(inventory.Plugins) > 0 || len(inventory.Capabilities) > 0 {
		return "plugin manifest metadata is available, but plugin runtime execution is disabled in the native API server"
	}
	return "dashboard plugin runtime is not configured in the native API server"
}

func dashboardExtensionsFromInventory(in pluginmeta.Inventory) dashboardExtensionStatus {
	inventory := clonePluginInventory(in)
	return dashboardExtensionStatus{
		Runtime:       disabledDashboardExtensionRuntime(),
		Themes:        unavailableDashboardThemes(),
		UIPlugins:     dashboardUIPlugins(inventory.Plugins),
		BackendRoutes: dashboardBackendRoutes(inventory.Capabilities),
	}
}

func dashboardPluginInventoryFromInventory(in pluginmeta.Inventory) dashboardPluginInventoryResponse {
	inventory := clonePluginInventory(in)
	return dashboardPluginInventoryResponse{
		Runtime:       disabledDashboardExtensionRuntime(),
		Themes:        unavailableDashboardThemes(),
		Plugins:       nonNilPluginStatuses(inventory.Plugins),
		Capabilities:  nonNilCapabilityStatuses(inventory.Capabilities),
		BackendRoutes: nonNilCapabilityStatuses(dashboardBackendRoutes(inventory.Capabilities)),
		Evidence:      append([]pluginmeta.Evidence(nil), inventory.Evidence...),
	}
}

func disabledDashboardExtensionRuntime() dashboardExtensionRuntimeStatus {
	return dashboardExtensionRuntimeStatus{
		State:            pluginmeta.StateDisabled,
		Reason:           "Hermes React/Vite dashboard extension runtime is absent from the native API server; plugin JavaScript and backend route modules are not imported or executed",
		ReactViteRuntime: dashboardReactViteRuntimeAbsent,
		Evidence: []pluginmeta.Evidence{{
			Code:    pluginmeta.EvidenceExecutionDisabled,
			Field:   "react_vite_runtime",
			Message: "Hermes React/Vite dashboard extension runtime is not embedded in Gormes",
		}},
	}
}

func unavailableDashboardThemes() dashboardThemeInventoryStatus {
	return dashboardThemeInventoryStatus{
		State:  pluginmeta.StateUnavailable,
		Themes: []dashboardThemeStatus{},
		Evidence: []pluginmeta.Evidence{{
			Code:    pluginmeta.EvidenceThemeRuntimeUnavailable,
			Field:   "dashboard.theme",
			Message: "dashboard theme discovery and application are unavailable in the native API server",
		}},
	}
}

func dashboardUIPlugins(plugins []pluginmeta.PluginStatus) []pluginmeta.PluginStatus {
	out := make([]pluginmeta.PluginStatus, 0, len(plugins))
	for _, plugin := range plugins {
		if plugin.Dashboard != nil || hasCapabilityKind(plugin.Capabilities, pluginmeta.CapabilityDashboard) {
			out = append(out, clonePluginStatus(plugin))
		}
	}
	return nonNilPluginStatuses(out)
}

func dashboardBackendRoutes(capabilities []pluginmeta.CapabilityStatus) []pluginmeta.CapabilityStatus {
	out := make([]pluginmeta.CapabilityStatus, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability.Kind == pluginmeta.CapabilityBackendRoute {
			out = append(out, clonePluginCapabilityStatus(capability))
		}
	}
	return nonNilCapabilityStatuses(out)
}

func hasCapabilityKind(capabilities []pluginmeta.CapabilityStatus, kind pluginmeta.CapabilityKind) bool {
	for _, capability := range capabilities {
		if capability.Kind == kind {
			return true
		}
	}
	return false
}

func nonNilPluginStatuses(in []pluginmeta.PluginStatus) []pluginmeta.PluginStatus {
	if in == nil {
		return []pluginmeta.PluginStatus{}
	}
	return in
}

func nonNilCapabilityStatuses(in []pluginmeta.CapabilityStatus) []pluginmeta.CapabilityStatus {
	if in == nil {
		return []pluginmeta.CapabilityStatus{}
	}
	return in
}

func parseDashboardInt(raw string, fallback, min, max int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func cloneDashboardModelProviders(in []DashboardModelProvider) []DashboardModelProvider {
	out := append([]DashboardModelProvider(nil), in...)
	for i := range out {
		out[i].Models = append([]string(nil), out[i].Models...)
	}
	return out
}

func cloneDashboardOAuthProviders(in []DashboardOAuthProvider) []DashboardOAuthProvider {
	return append([]DashboardOAuthProvider(nil), in...)
}

func clonePluginInventory(in pluginmeta.Inventory) pluginmeta.Inventory {
	out := pluginmeta.Inventory{
		Plugins:                 make([]pluginmeta.PluginStatus, len(in.Plugins)),
		Capabilities:            make([]pluginmeta.CapabilityStatus, len(in.Capabilities)),
		Evidence:                append([]pluginmeta.Evidence(nil), in.Evidence...),
		ProjectDiscoveryEnabled: in.ProjectDiscoveryEnabled,
	}
	for i, plugin := range in.Plugins {
		out.Plugins[i] = clonePluginStatus(plugin)
	}
	for i, capability := range in.Capabilities {
		out.Capabilities[i] = clonePluginCapabilityStatus(capability)
	}
	sort.Slice(out.Plugins, func(i, j int) bool {
		if out.Plugins[i].Name != out.Plugins[j].Name {
			return out.Plugins[i].Name < out.Plugins[j].Name
		}
		return out.Plugins[i].Source < out.Plugins[j].Source
	})
	sort.Slice(out.Capabilities, func(i, j int) bool {
		if out.Capabilities[i].Plugin != out.Capabilities[j].Plugin {
			return out.Capabilities[i].Plugin < out.Capabilities[j].Plugin
		}
		if out.Capabilities[i].Kind != out.Capabilities[j].Kind {
			return out.Capabilities[i].Kind < out.Capabilities[j].Kind
		}
		return out.Capabilities[i].Name < out.Capabilities[j].Name
	})
	return out
}

func clonePluginStatus(in pluginmeta.PluginStatus) pluginmeta.PluginStatus {
	out := in
	out.Manifest.RequiresEnv = append([]string(nil), in.Manifest.RequiresEnv...)
	out.Manifest.RequiresAuth = append([]string(nil), in.Manifest.RequiresAuth...)
	out.Manifest.Capabilities = append([]pluginmeta.Capability(nil), in.Manifest.Capabilities...)
	out.Capabilities = make([]pluginmeta.CapabilityStatus, len(in.Capabilities))
	for i, capability := range in.Capabilities {
		out.Capabilities[i] = clonePluginCapabilityStatus(capability)
	}
	out.Evidence = append([]pluginmeta.Evidence(nil), in.Evidence...)
	if in.Dashboard != nil {
		dashboard := *in.Dashboard
		dashboard.Slots = append([]string(nil), in.Dashboard.Slots...)
		out.Dashboard = &dashboard
	}
	return out
}

func clonePluginCapabilityStatus(in pluginmeta.CapabilityStatus) pluginmeta.CapabilityStatus {
	out := in
	out.Evidence = append([]pluginmeta.Evidence(nil), in.Evidence...)
	return out
}
