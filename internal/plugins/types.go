package plugins

// Source identifies where a plugin manifest was discovered.
type Source string

const (
	SourceBundled Source = "bundled"
	SourceUser    Source = "user"
	SourceProject Source = "project"
)

const (
	StateDisabled  = "disabled"
	StateInvalid   = "invalid"
	StateMalformed = "malformed"
)

const (
	EvidenceExecutionDisabled         = "execution_disabled"
	EvidenceIncompatibleVersion       = "incompatible_version"
	EvidenceInvalidName               = "invalid_name"
	EvidenceMalformedManifest         = "malformed_manifest"
	EvidenceMissingCredential         = "missing_credential"
	EvidenceMissingRequiredField      = "missing_required_field"
	EvidenceProjectPluginsDisabled    = "project_plugins_disabled"
	EvidenceUnsupportedCapabilityKind = "unsupported_capability_kind"
)

// CapabilityKind is the metadata-only declaration for plugin extension points.
type CapabilityKind string

const (
	CapabilityTool         CapabilityKind = "tool"
	CapabilityHook         CapabilityKind = "hook"
	CapabilityDashboard    CapabilityKind = "dashboard"
	CapabilityBackendRoute CapabilityKind = "backend_route"
)

// Evidence explains why a plugin or capability is unavailable.
type Evidence struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message,omitempty"`
}

// Capability is a manifest declaration before runtime state is applied.
type Capability struct {
	Kind        CapabilityKind `json:"kind"`
	Name        string         `json:"name"`
	SourceField string         `json:"source_field,omitempty"`
}

// Manifest is the normalized metadata parsed from plugin.yaml and optional
// dashboard/manifest.json. It contains declarations only, never runtime code.
type Manifest struct {
	Name           string       `json:"name"`
	Version        string       `json:"version"`
	Label          string       `json:"label,omitempty"`
	Description    string       `json:"description,omitempty"`
	Author         string       `json:"author,omitempty"`
	Kind           string       `json:"kind,omitempty"`
	RequiresGormes string       `json:"requires_gormes,omitempty"`
	RequiresEnv    []string     `json:"requires_env,omitempty"`
	RequiresAuth   []string     `json:"requires_auth,omitempty"`
	Capabilities   []Capability `json:"capabilities,omitempty"`
}

// DashboardTab is the navigation metadata from dashboard/manifest.json.
type DashboardTab struct {
	Path     string `json:"path,omitempty"`
	Position string `json:"position,omitempty"`
	Override string `json:"override,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
}

// DashboardManifest is the dashboard extension metadata from Hermes-style
// dashboard/manifest.json files.
type DashboardManifest struct {
	Name        string       `json:"name"`
	Label       string       `json:"label"`
	Description string       `json:"description,omitempty"`
	Icon        string       `json:"icon,omitempty"`
	Version     string       `json:"version,omitempty"`
	Tab         DashboardTab `json:"tab"`
	Slots       []string     `json:"slots,omitempty"`
	Entry       string       `json:"entry,omitempty"`
	CSS         string       `json:"css,omitempty"`
	API         string       `json:"api,omitempty"`
}

// CapabilityStatus is the disabled/invalid inventory row exposed to tools and
// dashboard surfaces.
type CapabilityStatus struct {
	Plugin   string         `json:"plugin"`
	Kind     CapabilityKind `json:"kind"`
	Name     string         `json:"name"`
	State    string         `json:"state"`
	Evidence []Evidence     `json:"evidence,omitempty"`
}

// PluginStatus is a metadata-only plugin status row. RuntimeCodeExecuted is
// intentionally part of the contract so tests can prove this slice stays inert.
type PluginStatus struct {
	Name                string             `json:"name"`
	Version             string             `json:"version,omitempty"`
	Label               string             `json:"label,omitempty"`
	Description         string             `json:"description,omitempty"`
	Source              Source             `json:"source,omitempty"`
	State               string             `json:"state"`
	Manifest            Manifest           `json:"manifest,omitempty"`
	Dashboard           *DashboardManifest `json:"dashboard,omitempty"`
	Capabilities        []CapabilityStatus `json:"capabilities,omitempty"`
	Evidence            []Evidence         `json:"evidence,omitempty"`
	RuntimeCodeExecuted bool               `json:"runtime_code_executed"`
}

// Inventory is the flattened plugin/capability status payload shared by the
// tool registry and dashboard API.
type Inventory struct {
	Plugins                 []PluginStatus     `json:"plugins,omitempty"`
	Capabilities            []CapabilityStatus `json:"capabilities,omitempty"`
	Evidence                []Evidence         `json:"evidence,omitempty"`
	ProjectDiscoveryEnabled bool               `json:"project_discovery_enabled"`
}

// LoadOptions configures a single plugin-directory load.
type LoadOptions struct {
	Source               Source
	CurrentGormesVersion string
	EnvLookup            func(string) bool
	AuthLookup           func(string) bool
}

// DiscoveryRoots are directory roots containing plugin directories.
type DiscoveryRoots struct {
	Bundled []string
	User    []string
	Project string
}

// DiscoverOptions controls metadata-only discovery.
type DiscoverOptions struct {
	CurrentGormesVersion string
	EnvLookup            func(string) bool
	AuthLookup           func(string) bool
	EnableProjectPlugins bool
}
