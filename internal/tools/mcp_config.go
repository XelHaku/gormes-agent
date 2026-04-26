package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultMCPToolTimeout    = 120 * time.Second
	defaultMCPConnectTimeout = 60 * time.Second
	defaultMCPSamplingTime   = 30 * time.Second

	// RedactedMCPConfigValue is the public placeholder used in MCP status
	// surfaces when config contains credentials or token-shaped values.
	RedactedMCPConfigValue = "[REDACTED]"
)

// MCPTransport identifies the transport a resolved MCP server would use.
type MCPTransport string

const (
	MCPTransportStdio MCPTransport = "stdio"
	MCPTransportHTTP  MCPTransport = "http"
)

// MCPConfigStatus is the degraded-mode state for one configured server.
type MCPConfigStatus string

const (
	MCPConfigStatusReady            MCPConfigStatus = "ready"
	MCPConfigStatusDisabled         MCPConfigStatus = "disabled"
	MCPConfigStatusMissingSDK       MCPConfigStatus = "missing_sdk"
	MCPConfigStatusInvalidTransport MCPConfigStatus = "invalid_transport"
	MCPConfigStatusInvalidEnv       MCPConfigStatus = "invalid_env"
	MCPConfigStatusInvalidConfig    MCPConfigStatus = "invalid_config"
)

// MCPConfigOptions controls pure MCP config resolution.
type MCPConfigOptions struct {
	LookupEnv func(string) (string, bool)
	// RuntimeAvailable lets callers report missing MCP runtime/SDK support
	// without importing that runtime in the config resolver.
	RuntimeAvailable   *bool
	RuntimeUnavailable string
}

// MCPConfigResolution is the safe, typed MCP config surface. Servers is empty
// when any config error is present so callers cannot launch a partial set.
type MCPConfigResolution struct {
	Servers  []MCPServerDefinition
	Statuses []MCPServerStatus
}

// MCPServerDefinition is a validated MCP server configuration. It contains
// resolved credentials because runtime transport code needs them; use Statuses
// or RedactedStatusText for operator-visible output.
type MCPServerDefinition struct {
	Name           string
	Enabled        bool
	Transport      MCPTransport
	Command        string
	Args           []string
	Env            map[string]string
	URL            string
	Headers        map[string]string
	Timeout        time.Duration
	ConnectTimeout time.Duration
	Sampling       MCPSamplingConfig
}

// MCPSamplingConfig captures server-initiated sampling limits without
// creating any MCP SDK objects.
type MCPSamplingConfig struct {
	Enabled       bool
	Model         string
	MaxTokensCap  int
	Timeout       time.Duration
	MaxRPM        int
	AllowedModels []string
	MaxToolRounds int
	LogLevel      string
}

// MCPServerStatus is the redacted config/status view used before runtime
// connection attempts.
type MCPServerStatus struct {
	Name           string
	Enabled        bool
	Status         MCPConfigStatus
	Reason         string
	Transport      MCPTransport
	Command        string
	Args           []string
	Env            map[string]string
	URL            string
	Headers        map[string]string
	Timeout        time.Duration
	ConnectTimeout time.Duration
	Sampling       MCPSamplingConfig
}

type mcpConfigIssue struct {
	server  string
	status  MCPConfigStatus
	message string
}

// MCPConfigError reports one or more config issues with credentials redacted.
type MCPConfigError struct {
	Issues []mcpConfigIssue
}

func (e *MCPConfigError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "mcp config: invalid"
	}
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, fmt.Sprintf("%s: %s", issue.server, redactMCPString(issue.message)))
	}
	return "mcp config: " + strings.Join(parts, "; ")
}

// ParseMCPConfigYAML parses a Hermes-compatible YAML document with a
// top-level mcp_servers section.
func ParseMCPConfigYAML(data []byte, opts MCPConfigOptions) (MCPConfigResolution, error) {
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return MCPConfigResolution{}, fmt.Errorf("mcp config yaml: %w", err)
	}
	return ResolveMCPConfig(raw, opts)
}

// ParseMCPConfigJSON parses a Hermes-compatible JSON document with a
// top-level mcp_servers section.
func ParseMCPConfigJSON(data []byte, opts MCPConfigOptions) (MCPConfigResolution, error) {
	var raw any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return MCPConfigResolution{}, fmt.Errorf("mcp config json: %w", err)
	}
	return ResolveMCPConfig(raw, opts)
}

// ResolveMCPConfig turns an in-memory config document into safe server
// definitions without importing an MCP SDK, spawning processes, or opening
// transports.
func ResolveMCPConfig(raw any, opts MCPConfigOptions) (MCPConfigResolution, error) {
	lookupEnv := opts.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}

	root := mcpMap(raw)
	if root == nil {
		return MCPConfigResolution{}, nil
	}
	serversRaw, ok := lookupMCPValue(root, "mcp_servers")
	if !ok {
		serversRaw, ok = lookupMCPValue(root, "mcpServers")
	}
	if !ok {
		return MCPConfigResolution{}, nil
	}
	serversMap := mcpMap(serversRaw)
	if serversMap == nil {
		status := MCPServerStatus{
			Name:   "mcp_servers",
			Status: MCPConfigStatusInvalidConfig,
			Reason: "mcp_servers must be a map",
		}
		issue := mcpConfigIssue{server: "mcp_servers", status: MCPConfigStatusInvalidConfig, message: status.Reason}
		return MCPConfigResolution{Statuses: []MCPServerStatus{status}}, &MCPConfigError{Issues: []mcpConfigIssue{issue}}
	}

	names := sortedMCPKeys(serversMap)
	definitions := make([]MCPServerDefinition, 0, len(names))
	statuses := make([]MCPServerStatus, 0, len(names))
	var issues []mcpConfigIssue
	for _, name := range names {
		def, status, issue := resolveMCPServer(name, serversMap[name], lookupEnv)
		statuses = append(statuses, status)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		definitions = append(definitions, def)
	}

	if len(issues) > 0 {
		return MCPConfigResolution{Statuses: statuses}, &MCPConfigError{Issues: issues}
	}
	if opts.RuntimeAvailable != nil && !*opts.RuntimeAvailable {
		reason := strings.TrimSpace(opts.RuntimeUnavailable)
		if reason == "" {
			reason = "MCP runtime unavailable"
		}
		var runtimeIssues []mcpConfigIssue
		for i := range statuses {
			if statuses[i].Status != MCPConfigStatusReady {
				continue
			}
			statuses[i].Status = MCPConfigStatusMissingSDK
			statuses[i].Reason = redactMCPString(reason)
			runtimeIssues = append(runtimeIssues, mcpConfigIssue{
				server:  statuses[i].Name,
				status:  MCPConfigStatusMissingSDK,
				message: reason,
			})
		}
		if len(runtimeIssues) > 0 {
			return MCPConfigResolution{Statuses: statuses}, &MCPConfigError{Issues: runtimeIssues}
		}
	}
	return MCPConfigResolution{Servers: definitions, Statuses: statuses}, nil
}

func resolveMCPServer(name string, raw any, lookupEnv func(string) (string, bool)) (MCPServerDefinition, MCPServerStatus, *mcpConfigIssue) {
	server := mcpMap(raw)
	baseStatus := MCPServerStatus{
		Name:           name,
		Enabled:        true,
		Timeout:        defaultMCPToolTimeout,
		ConnectTimeout: defaultMCPConnectTimeout,
		Sampling:       defaultMCPSamplingConfig(),
	}
	if server == nil {
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidConfig, "server config must be a map"), &mcpConfigIssue{
			server:  name,
			status:  MCPConfigStatusInvalidConfig,
			message: "server config must be a map",
		}
	}

	enabled := parseMCPBool(mcpValue(server, "enabled"), true)
	baseStatus.Enabled = enabled

	command, err := mcpOptionalString(server, "command", lookupEnv)
	if err != nil {
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidEnv, err.Error()), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidEnv, message: err.Error()}
	}
	url, err := mcpOptionalString(server, "url", lookupEnv)
	if err != nil {
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidEnv, err.Error()), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidEnv, message: err.Error()}
	}
	command = strings.TrimSpace(command)
	url = strings.TrimSpace(url)
	hasCommand := command != ""
	hasURL := url != ""
	switch {
	case hasCommand && hasURL:
		reason := "server has both command and url; choose exactly one transport"
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidTransport, reason), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidTransport, message: reason}
	case !hasCommand && !hasURL:
		reason := "server requires command or url"
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidTransport, reason), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidTransport, message: reason}
	}

	args, err := mcpStringList(mcpValue(server, "args"), "args", lookupEnv)
	if err != nil {
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidEnv, err.Error()), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidEnv, message: err.Error()}
	}
	env, err := mcpStringMap(mcpValue(server, "env"), "env", true, lookupEnv)
	if err != nil {
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidEnv, err.Error()), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidEnv, message: err.Error()}
	}
	headers, err := mcpStringMap(mcpValue(server, "headers"), "headers", false, lookupEnv)
	if err != nil {
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidEnv, err.Error()), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidEnv, message: err.Error()}
	}
	timeout, err := mcpDuration(mcpValue(server, "timeout"), defaultMCPToolTimeout, "timeout")
	if err != nil {
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidConfig, err.Error()), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidConfig, message: err.Error()}
	}
	connectTimeout, err := mcpDuration(mcpValue(server, "connect_timeout"), defaultMCPConnectTimeout, "connect_timeout")
	if err != nil {
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidConfig, err.Error()), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidConfig, message: err.Error()}
	}
	sampling, err := mcpSamplingConfig(mcpValue(server, "sampling"), lookupEnv)
	if err != nil {
		return MCPServerDefinition{}, invalidMCPStatus(baseStatus, MCPConfigStatusInvalidConfig, err.Error()), &mcpConfigIssue{server: name, status: MCPConfigStatusInvalidConfig, message: err.Error()}
	}

	transport := MCPTransportStdio
	if hasURL {
		transport = MCPTransportHTTP
	}
	def := MCPServerDefinition{
		Name:           name,
		Enabled:        enabled,
		Transport:      transport,
		Command:        command,
		Args:           args,
		Env:            env,
		URL:            url,
		Headers:        headers,
		Timeout:        timeout,
		ConnectTimeout: connectTimeout,
		Sampling:       sampling,
	}
	status := redactedMCPStatus(def, MCPConfigStatusReady, "")
	if !enabled {
		status.Status = MCPConfigStatusDisabled
		status.Reason = "server disabled by config"
	}
	return def, status, nil
}

func invalidMCPStatus(status MCPServerStatus, kind MCPConfigStatus, reason string) MCPServerStatus {
	status.Status = kind
	status.Reason = redactMCPString(reason)
	if status.Env == nil {
		status.Env = map[string]string{}
	}
	if status.Headers == nil {
		status.Headers = map[string]string{}
	}
	return status
}

func redactedMCPStatus(def MCPServerDefinition, status MCPConfigStatus, reason string) MCPServerStatus {
	return MCPServerStatus{
		Name:           def.Name,
		Enabled:        def.Enabled,
		Status:         status,
		Reason:         redactMCPString(reason),
		Transport:      def.Transport,
		Command:        def.Command,
		Args:           append([]string(nil), def.Args...),
		Env:            redactMCPMap(def.Env),
		URL:            def.URL,
		Headers:        redactMCPMap(def.Headers),
		Timeout:        def.Timeout,
		ConnectTimeout: def.ConnectTimeout,
		Sampling:       def.Sampling,
	}
}

// Server returns the resolved server definition by name.
func (r MCPConfigResolution) Server(name string) (MCPServerDefinition, bool) {
	for _, server := range r.Servers {
		if server.Name == name {
			return server, true
		}
	}
	return MCPServerDefinition{}, false
}

// Status returns the redacted status row by name.
func (r MCPConfigResolution) Status(name string) (MCPServerStatus, bool) {
	for _, status := range r.Statuses {
		if status.Name == name {
			return status, true
		}
	}
	return MCPServerStatus{}, false
}

// RedactedStatusText renders a stable operator-facing status string with all
// token-shaped config values redacted.
func (r MCPConfigResolution) RedactedStatusText() string {
	rows := append([]MCPServerStatus(nil), r.Statuses...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		fields := []string{
			"name=" + row.Name,
			"status=" + string(row.Status),
		}
		if row.Transport != "" {
			fields = append(fields, "transport="+string(row.Transport))
		}
		if row.Command != "" {
			fields = append(fields, "command="+row.Command)
		}
		if row.URL != "" {
			fields = append(fields, "url="+row.URL)
		}
		if len(row.Headers) > 0 {
			fields = append(fields, "headers="+formatMCPStringMap(row.Headers))
		}
		if len(row.Env) > 0 {
			fields = append(fields, "env="+formatMCPStringMap(row.Env))
		}
		if row.Reason != "" {
			fields = append(fields, "reason="+redactMCPString(row.Reason))
		}
		parts = append(parts, strings.Join(fields, " "))
	}
	return strings.Join(parts, "\n")
}

func defaultMCPSamplingConfig() MCPSamplingConfig {
	return MCPSamplingConfig{
		Enabled:       true,
		MaxTokensCap:  4096,
		Timeout:       defaultMCPSamplingTime,
		MaxRPM:        10,
		MaxToolRounds: 5,
		LogLevel:      "info",
	}
}

func mcpSamplingConfig(raw any, lookupEnv func(string) (string, bool)) (MCPSamplingConfig, error) {
	cfg := defaultMCPSamplingConfig()
	if raw == nil {
		return cfg, nil
	}
	values := mcpMap(raw)
	if values == nil {
		return cfg, fmt.Errorf("sampling must be a map")
	}
	if rawEnabled, ok := lookupMCPValue(values, "enabled"); ok {
		cfg.Enabled = parseMCPBool(rawEnabled, cfg.Enabled)
	}
	if rawModel, ok := lookupMCPValue(values, "model"); ok {
		model, err := mcpStringValue(rawModel, "sampling.model", lookupEnv)
		if err != nil {
			return cfg, err
		}
		cfg.Model = model
	}
	if rawCap, ok := lookupMCPValue(values, "max_tokens_cap"); ok {
		parsed, err := mcpInt(rawCap, cfg.MaxTokensCap, "sampling.max_tokens_cap", 0)
		if err != nil {
			return cfg, err
		}
		cfg.MaxTokensCap = parsed
	}
	if rawTimeout, ok := lookupMCPValue(values, "timeout"); ok {
		parsed, err := mcpDuration(rawTimeout, cfg.Timeout, "sampling.timeout")
		if err != nil {
			return cfg, err
		}
		cfg.Timeout = parsed
	}
	if rawRPM, ok := lookupMCPValue(values, "max_rpm"); ok {
		parsed, err := mcpInt(rawRPM, cfg.MaxRPM, "sampling.max_rpm", 1)
		if err != nil {
			return cfg, err
		}
		cfg.MaxRPM = parsed
	}
	if rawModels, ok := lookupMCPValue(values, "allowed_models"); ok {
		parsed, err := mcpStringList(rawModels, "sampling.allowed_models", lookupEnv)
		if err != nil {
			return cfg, err
		}
		cfg.AllowedModels = parsed
	}
	if rawRounds, ok := lookupMCPValue(values, "max_tool_rounds"); ok {
		parsed, err := mcpInt(rawRounds, cfg.MaxToolRounds, "sampling.max_tool_rounds", 0)
		if err != nil {
			return cfg, err
		}
		cfg.MaxToolRounds = parsed
	}
	if rawLevel, ok := lookupMCPValue(values, "log_level"); ok {
		parsed, err := mcpStringValue(rawLevel, "sampling.log_level", lookupEnv)
		if err != nil {
			return cfg, err
		}
		cfg.LogLevel = strings.ToLower(strings.TrimSpace(parsed))
	}
	return cfg, nil
}

func mcpMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[fmt.Sprint(key)] = value
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out
	default:
		return nil
	}
}

func sortedMCPKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func lookupMCPValue(values map[string]any, name string) (any, bool) {
	value, ok := values[name]
	if ok {
		return value, true
	}
	for key, value := range values {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return nil, false
}

func mcpValue(values map[string]any, name string) any {
	value, _ := lookupMCPValue(values, name)
	return value
}

func mcpOptionalString(values map[string]any, name string, lookupEnv func(string) (string, bool)) (string, error) {
	raw, ok := lookupMCPValue(values, name)
	if !ok || raw == nil {
		return "", nil
	}
	return mcpStringValue(raw, name, lookupEnv)
}

func mcpStringValue(value any, field string, lookupEnv func(string) (string, bool)) (string, error) {
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case fmt.Stringer:
		raw = typed.String()
	case json.Number:
		raw = typed.String()
	case int, int64, int32, float64, float32, bool:
		raw = fmt.Sprint(typed)
	default:
		return "", fmt.Errorf("%s must be a string", field)
	}
	return interpolateMCPEnv(raw, lookupEnv)
}

func mcpStringList(value any, field string, lookupEnv func(string) (string, bool)) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	var raw []any
	switch typed := value.(type) {
	case []any:
		raw = typed
	case []string:
		raw = make([]any, len(typed))
		for i, item := range typed {
			raw[i] = item
		}
	default:
		return nil, fmt.Errorf("%s must be a list of strings", field)
	}
	out := make([]string, 0, len(raw))
	for i, item := range raw {
		parsed, err := mcpStringValue(item, fmt.Sprintf("%s[%d]", field, i), lookupEnv)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}

var mcpEnvNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func mcpStringMap(value any, field string, validateKeys bool, lookupEnv func(string) (string, bool)) (map[string]string, error) {
	if value == nil {
		return map[string]string{}, nil
	}
	values := mcpMap(value)
	if values == nil {
		return nil, fmt.Errorf("%s must be a map", field)
	}
	out := make(map[string]string, len(values))
	for _, key := range sortedMCPKeys(values) {
		if validateKeys && !mcpEnvNameRE.MatchString(key) {
			return nil, fmt.Errorf("invalid env variable name %q", key)
		}
		parsed, err := mcpStringValue(values[key], field+"."+key, lookupEnv)
		if err != nil {
			return nil, err
		}
		out[key] = parsed
	}
	return out, nil
}

var mcpEnvRefRE = regexp.MustCompile(`\$\{([^}]+)\}`)

func interpolateMCPEnv(value string, lookupEnv func(string) (string, bool)) (string, error) {
	var firstErr error
	resolved := mcpEnvRefRE.ReplaceAllStringFunc(value, func(match string) string {
		if firstErr != nil {
			return match
		}
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}"))
		if !mcpEnvNameRE.MatchString(name) {
			firstErr = fmt.Errorf("invalid env variable name %q", name)
			return match
		}
		value, ok := lookupEnv(name)
		if !ok {
			firstErr = fmt.Errorf("missing environment variable %s", name)
			return match
		}
		return value
	})
	if firstErr != nil {
		return "", firstErr
	}
	return resolved, nil
}

func parseMCPBool(value any, fallback bool) bool {
	switch typed := value.(type) {
	case nil:
		return fallback
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		default:
			return fallback
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case json.Number:
		i, err := typed.Int64()
		return err == nil && i != 0
	default:
		return fallback
	}
}

func mcpDuration(value any, fallback time.Duration, field string) (time.Duration, error) {
	if value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case time.Duration:
		if typed <= 0 {
			return 0, fmt.Errorf("%s must be positive", field)
		}
		return typed, nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return fallback, nil
		}
		if dur, err := time.ParseDuration(text); err == nil {
			if dur <= 0 {
				return 0, fmt.Errorf("%s must be positive", field)
			}
			return dur, nil
		}
		seconds, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be seconds or a duration", field)
		}
		return secondsDuration(seconds, field)
	case int:
		return secondsDuration(float64(typed), field)
	case int64:
		return secondsDuration(float64(typed), field)
	case int32:
		return secondsDuration(float64(typed), field)
	case float64:
		return secondsDuration(typed, field)
	case float32:
		return secondsDuration(float64(typed), field)
	case json.Number:
		seconds, err := typed.Float64()
		if err != nil {
			return 0, fmt.Errorf("%s must be numeric seconds", field)
		}
		return secondsDuration(seconds, field)
	default:
		return 0, fmt.Errorf("%s must be seconds or a duration", field)
	}
}

func secondsDuration(seconds float64, field string) (time.Duration, error) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return 0, fmt.Errorf("%s must be positive", field)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func mcpInt(value any, fallback int, field string, minimum int) (int, error) {
	if value == nil {
		return fallback, nil
	}
	var parsed int64
	switch typed := value.(type) {
	case int:
		parsed = int64(typed)
	case int64:
		parsed = typed
	case int32:
		parsed = int64(typed)
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("%s must be an integer", field)
		}
		parsed = int64(typed)
	case json.Number:
		var err error
		parsed, err = typed.Int64()
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", field)
		}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return fallback, nil
		}
		var err error
		parsed, err = strconv.ParseInt(text, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", field)
		}
	default:
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	if parsed < int64(minimum) {
		return 0, fmt.Errorf("%s must be at least %d", field, minimum)
	}
	return int(parsed), nil
}

func redactMCPMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if isMCPSecretKey(key) || isMCPSecretValue(value) {
			out[key] = RedactedMCPConfigValue
			continue
		}
		out[key] = redactMCPString(value)
	}
	return out
}

func isMCPSecretKey(key string) bool {
	lower := strings.ToLower(key)
	secretFragments := []string{
		"authorization",
		"auth_header",
		"api_key",
		"access_token",
		"refresh_token",
		"personal_access_token",
		"token",
		"secret",
		"password",
	}
	for _, fragment := range secretFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

var mcpCredentialPattern = regexp.MustCompile(`(?i)(ghp_[A-Za-z0-9_]{1,255}|sk-[A-Za-z0-9_]{1,255}|Bearer\s+\S+|token=[^\s&,;"']{1,255}|key=[^\s&,;"']{1,255}|API_KEY=[^\s&,;"']{1,255}|password=[^\s&,;"']{1,255}|secret=[^\s&,;"']{1,255})`)

func isMCPSecretValue(value string) bool {
	return mcpCredentialPattern.MatchString(value)
}

func redactMCPString(value string) string {
	return mcpCredentialPattern.ReplaceAllString(value, RedactedMCPConfigValue)
}

func formatMCPStringMap(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+redactMCPString(values[key]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
