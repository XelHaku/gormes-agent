package tools

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

//go:embed testdata/upstream_tool_parity_manifest.json
var upstreamToolParityManifestJSON []byte

// ErrMissingToolParityRow is returned when a handler port is marked complete
// before its upstream descriptor has a parity fixture row.
var ErrMissingToolParityRow = errors.New("tools: missing upstream tool parity row")

// ToolParityIssueKind identifies a degraded-mode doctor finding.
type ToolParityIssueKind string

const (
	ToolParityIssueDisabledTool            ToolParityIssueKind = "disabled_tool"
	ToolParityIssueMissingDependency       ToolParityIssueKind = "missing_dependency"
	ToolParityIssueSchemaDrift             ToolParityIssueKind = "schema_drift"
	ToolParityIssueUnavailableProviderPath ToolParityIssueKind = "unavailable_provider_path"
)

// UpstreamToolParityManifest is the frozen donor descriptor inventory used to
// gate later handler ports.
type UpstreamToolParityManifest struct {
	GeneratedAt  string                  `json:"generated_at"`
	TrustClasses []string                `json:"trust_classes"`
	Source       ToolParitySource        `json:"source"`
	Tools        []UpstreamToolParityRow `json:"tools"`
	Toolsets     []UpstreamToolsetRow    `json:"toolsets"`
}

// ToolParitySource records the donor files used to capture the fixture.
type ToolParitySource struct {
	Registry string `json:"registry"`
	Toolsets string `json:"toolsets"`
}

// UpstreamToolParityRow captures the model-visible descriptor plus the
// operational metadata that must exist before porting a handler.
type UpstreamToolParityRow struct {
	Name            string                 `json:"name"`
	Toolset         string                 `json:"toolset"`
	SourceModule    string                 `json:"source_module"`
	Description     string                 `json:"description"`
	RequiredEnv     []string               `json:"required_env"`
	RequiredEnvMode string                 `json:"required_env_mode"`
	Dependencies    []string               `json:"dependencies"`
	ProviderPaths   []ToolProviderPath     `json:"provider_paths"`
	Schema          json.RawMessage        `json:"schema"`
	ResultEnvelope  ToolResultEnvelope     `json:"result_envelope"`
	TrustClasses    []string               `json:"trust_classes"`
	DegradedStatus  ToolDegradedModeStatus `json:"degraded_status"`
}

// ToolProviderPath captures optional provider-specific availability gates.
type ToolProviderPath struct {
	ID               string   `json:"id"`
	Description      string   `json:"description"`
	RequiredEnv      []string `json:"required_env"`
	RequiredEnvMode  string   `json:"required_env_mode"`
	RequiredBinaries []string `json:"required_binaries"`
}

// ToolResultEnvelope captures the JSON fields the donor returns on success or
// failure. Handler ports can refine these rows before they claim completion.
type ToolResultEnvelope struct {
	Encoding      string   `json:"encoding"`
	SuccessFields []string `json:"success_fields"`
	ErrorFields   []string `json:"error_fields"`
}

// ToolDegradedModeStatus captures how doctor should report degraded tools.
type ToolDegradedModeStatus struct {
	StatusField string   `json:"status_field"`
	Statuses    []string `json:"statuses"`
}

// UpstreamToolsetRow captures static and resolved donor toolset membership.
type UpstreamToolsetRow struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	DirectTools   []string `json:"direct_tools"`
	Includes      []string `json:"includes"`
	ResolvedTools []string `json:"resolved_tools"`
	Source        string   `json:"source"`
}

// ToolParityDoctorOptions controls degraded-mode inventory checks.
type ToolParityDoctorOptions struct {
	Env                    map[string]string
	DisabledTools          map[string]string
	LocalSchemas           map[string]json.RawMessage
	AvailableProviderPaths map[string]bool
}

// ToolParityDoctorReport is the aggregate doctor output for descriptor parity.
type ToolParityDoctorReport struct {
	Issues []ToolParityIssue
}

// ToolParityIssue is one degraded-mode doctor finding.
type ToolParityIssue struct {
	Kind    ToolParityIssueKind `json:"kind"`
	Tool    string              `json:"tool"`
	Toolset string              `json:"toolset,omitempty"`
	Detail  string              `json:"detail"`
}

// LoadUpstreamToolParityManifest returns the embedded upstream descriptor
// inventory fixture.
func LoadUpstreamToolParityManifest() (UpstreamToolParityManifest, error) {
	var manifest UpstreamToolParityManifest
	if err := json.Unmarshal(upstreamToolParityManifestJSON, &manifest); err != nil {
		return UpstreamToolParityManifest{}, fmt.Errorf("load upstream tool parity manifest: %w", err)
	}
	if err := manifest.validate(); err != nil {
		return UpstreamToolParityManifest{}, err
	}
	return manifest, nil
}

// Tool returns a tool descriptor row by name.
func (m UpstreamToolParityManifest) Tool(name string) (UpstreamToolParityRow, bool) {
	for _, row := range m.Tools {
		if row.Name == name {
			return row, true
		}
	}
	return UpstreamToolParityRow{}, false
}

// Toolset returns a toolset row by name.
func (m UpstreamToolParityManifest) Toolset(name string) (UpstreamToolsetRow, bool) {
	for _, row := range m.Toolsets {
		if row.Name == name {
			return row, true
		}
	}
	return UpstreamToolsetRow{}, false
}

// AssertHandlerPortAllowed enforces descriptor-first handler migration.
func (m UpstreamToolParityManifest) AssertHandlerPortAllowed(name string) error {
	if _, ok := m.Tool(name); ok {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrMissingToolParityRow, name)
}

// HasProviderPath reports whether a row captures a provider-specific path.
func (r UpstreamToolParityRow) HasProviderPath(id string) bool {
	for _, path := range r.ProviderPaths {
		if path.ID == id {
			return true
		}
	}
	return false
}

// Doctor reports disabled tools, missing dependencies, schema drift, and
// unavailable provider-specific paths from the frozen descriptor inventory.
func (m UpstreamToolParityManifest) Doctor(opts ToolParityDoctorOptions) ToolParityDoctorReport {
	var issues []ToolParityIssue
	for _, row := range m.Tools {
		if reason, disabled := opts.DisabledTools[row.Name]; disabled {
			issues = append(issues, ToolParityIssue{
				Kind:    ToolParityIssueDisabledTool,
				Tool:    row.Name,
				Toolset: row.Toolset,
				Detail:  reason,
			})
		}
		if missing := missingRequiredEnv(row.RequiredEnv, row.RequiredEnvMode, opts.Env); len(missing) > 0 {
			issues = append(issues, ToolParityIssue{
				Kind:    ToolParityIssueMissingDependency,
				Tool:    row.Name,
				Toolset: row.Toolset,
				Detail:  "missing env: " + strings.Join(missing, ", "),
			})
		}
		if local, ok := opts.LocalSchemas[row.Name]; ok && !sameJSON(local, row.Schema) {
			issues = append(issues, ToolParityIssue{
				Kind:    ToolParityIssueSchemaDrift,
				Tool:    row.Name,
				Toolset: row.Toolset,
				Detail:  "local schema differs from upstream parity fixture",
			})
		}
		for _, path := range row.ProviderPaths {
			if path.ID == "" || opts.AvailableProviderPaths[path.ID] {
				continue
			}
			if pathAvailable(path, opts.Env) {
				continue
			}
			issues = append(issues, ToolParityIssue{
				Kind:    ToolParityIssueUnavailableProviderPath,
				Tool:    row.Name,
				Toolset: row.Toolset,
				Detail:  path.ID + ": " + path.Description,
			})
		}
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Kind != issues[j].Kind {
			return issues[i].Kind < issues[j].Kind
		}
		return issues[i].Tool < issues[j].Tool
	})
	return ToolParityDoctorReport{Issues: issues}
}

func (m UpstreamToolParityManifest) validate() error {
	seen := make(map[string]struct{}, len(m.Tools))
	for _, row := range m.Tools {
		if row.Name == "" {
			return errors.New("upstream tool parity manifest: empty tool name")
		}
		if _, ok := seen[row.Name]; ok {
			return fmt.Errorf("upstream tool parity manifest: duplicate tool %s", row.Name)
		}
		seen[row.Name] = struct{}{}
		if row.Toolset == "" {
			return fmt.Errorf("upstream tool parity manifest: %s has empty toolset", row.Name)
		}
		if !json.Valid(row.Schema) {
			return fmt.Errorf("upstream tool parity manifest: %s has invalid schema JSON", row.Name)
		}
		if row.ResultEnvelope.Encoding == "" {
			return fmt.Errorf("upstream tool parity manifest: %s has empty result envelope", row.Name)
		}
		if row.DegradedStatus.StatusField == "" {
			return fmt.Errorf("upstream tool parity manifest: %s has empty degraded status field", row.Name)
		}
	}
	return nil
}

func pathAvailable(path ToolProviderPath, env map[string]string) bool {
	if len(path.RequiredEnv) > 0 && len(missingRequiredEnv(path.RequiredEnv, path.RequiredEnvMode, env)) == 0 {
		return true
	}
	return false
}

func missingRequiredEnv(required []string, mode string, env map[string]string) []string {
	if len(required) == 0 {
		return nil
	}
	if mode == "any" {
		for _, key := range required {
			if env[key] != "" {
				return nil
			}
		}
		return append([]string(nil), required...)
	}
	var missing []string
	for _, key := range required {
		if env[key] == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func sameJSON(a, b json.RawMessage) bool {
	ca, err := canonicalJSON(a)
	if err != nil {
		return false
	}
	cb, err := canonicalJSON(b)
	if err != nil {
		return false
	}
	return bytes.Equal(ca, cb)
}

func canonicalJSON(raw json.RawMessage) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
