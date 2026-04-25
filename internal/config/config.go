// Package config loads Gormes configuration from CLI flags > env > TOML > defaults.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/goncho"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/pflag"
)

// CurrentConfigVersion is the schema version this binary writes + accepts.
// When a breaking change to the TOML schema lands, bump this constant and
// add a migration in runMigrations() so older files stay readable.
const CurrentConfigVersion = 1

type Config struct {
	// ConfigVersion is the schema version of the loaded TOML file. Read
	// before any struct fields so migrations can run against the raw
	// document. Absent in TOML = treated as 1.
	ConfigVersion int `toml:"_config_version"`

	Hermes     HermesCfg     `toml:"hermes"`
	Gateway    GatewayCfg    `toml:"gateway"`
	TUI        TUICfg        `toml:"tui"`
	Input      InputCfg      `toml:"input"`
	Telegram   TelegramCfg   `toml:"telegram"`
	Discord    DiscordCfg    `toml:"discord"`
	Slack      SlackCfg      `toml:"slack"`
	Cron       CronCfg       `toml:"cron"`
	Skills     SkillsCfg     `toml:"skills"`
	Delegation DelegationCfg `toml:"delegation"`
	Goncho     GonchoCfg     `toml:"goncho"`
	// Resume is set only via the --resume CLI flag; intentionally not
	// a TOML field. Empty means "use whatever internal/session had
	// persisted for this binary's default key."
	Resume string `toml:"-"`
}

type TelegramCfg struct {
	BotToken          string `toml:"bot_token"`
	AllowedChatID     int64  `toml:"allowed_chat_id"`
	CoalesceMs        int    `toml:"coalesce_ms"`
	FirstRunDiscovery bool   `toml:"first_run_discovery"`
	// MemoryQueueCap (Phase 3.A): async worker queue capacity in
	// the telegram subcommand's SqliteStore. Defaults to 1024.
	MemoryQueueCap int `toml:"memory_queue_cap"`
	// ExtractorBatchSize / ExtractorPollInterval (Phase 3.B).
	ExtractorBatchSize    int           `toml:"extractor_batch_size"`
	ExtractorPollInterval time.Duration `toml:"extractor_poll_interval"`
	// RecallEnabled / RecallWeightThreshold / RecallMaxFacts / RecallDepth
	// (Phase 3.C).
	RecallEnabled         bool    `toml:"recall_enabled"`
	RecallWeightThreshold float64 `toml:"recall_weight_threshold"`
	RecallMaxFacts        int     `toml:"recall_max_facts"`
	RecallDepth           int     `toml:"recall_depth"`
	// RecallDecayHorizonDays (Phase 3.E.6) — maps to
	// RecallConfig.DecayHorizonDays. An edge's effective weight
	// decays linearly from raw at age=0 to 0 at this many days old.
	// 0 = unset (withDefaults promotes to 180). <0 = disabled.
	RecallDecayHorizonDays int `toml:"recall_decay_horizon_days"`
	// MirrorEnabled / MirrorPath / MirrorInterval (Phase 3.D.5).
	// The Memory Mirror exports SQLite entities/relationships to USER.md.
	MirrorEnabled  bool          `toml:"mirror_enabled"`
	MirrorPath     string        `toml:"mirror_path"`
	MirrorInterval time.Duration `toml:"mirror_interval"`
	// Phase 3.D semantic fusion — all opt-in via SemanticEnabled.
	SemanticEnabled       bool          `toml:"semantic_enabled"`
	SemanticEndpoint      string        `toml:"semantic_endpoint"`
	SemanticModel         string        `toml:"semantic_model"`
	SemanticTopK          int           `toml:"semantic_top_k"`
	SemanticMinSimilarity float64       `toml:"semantic_min_similarity"`
	EmbedderPollInterval  time.Duration `toml:"embedder_poll_interval"`
	EmbedderBatchSize     int           `toml:"embedder_batch_size"`
	EmbedderCallTimeout   time.Duration `toml:"embedder_call_timeout"`
	QueryEmbedTimeout     time.Duration `toml:"query_embed_timeout"`
}

// DiscordCfg drives the Discord channel adapter.
type DiscordCfg struct {
	Token             string   `toml:"token"`
	AllowedChannelID  string   `toml:"allowed_channel_id"`
	ServerActions     []string `toml:"server_actions"`
	CoalesceMs        int      `toml:"coalesce_ms"`
	FirstRunDiscovery bool     `toml:"first_run_discovery"`
}

func (c DiscordCfg) Enabled() bool {
	if c.Token == "" {
		return false
	}
	return c.AllowedChannelID != "" || c.FirstRunDiscovery
}

// SlackCfg drives the Slack Socket Mode channel adapter.
type SlackCfg struct {
	Enabled           bool   `toml:"enabled"`
	BotToken          string `toml:"bot_token"`
	AppToken          string `toml:"app_token"`
	AllowedChannelID  string `toml:"allowed_channel_id"`
	CoalesceMs        int    `toml:"coalesce_ms"`
	FirstRunDiscovery bool   `toml:"first_run_discovery"`
}

type CronCfg struct {
	Enabled        bool          `toml:"enabled"`
	CallTimeout    time.Duration `toml:"call_timeout"`
	MirrorInterval time.Duration `toml:"mirror_interval"`
	MirrorPath     string        `toml:"mirror_path"`
}

// SkillsCfg configures the Phase 2.G0 static skills runtime.
type SkillsCfg struct {
	Root             string `toml:"root"`
	SelectionCap     int    `toml:"selection_cap"`
	MaxDocumentBytes int    `toml:"max_document_bytes"`
	UsageLogPath     string `toml:"usage_log_path"`
}

// DelegationCfg configures Phase 2.E subagent execution.
type DelegationCfg struct {
	Enabled               bool          `toml:"enabled"`
	MaxDepth              int           `toml:"max_depth"`
	MaxConcurrentChildren int           `toml:"max_concurrent_children"`
	DefaultMaxIterations  int           `toml:"default_max_iterations"`
	DefaultTimeout        time.Duration `toml:"default_timeout"`
	RunLogPath            string        `toml:"run_log_path"`
	MaxWaiting            int           `toml:"max_waiting"`
}

// GonchoCfg configures the in-process Honcho-compatible memory facade.
type GonchoCfg struct {
	Enabled                      bool   `toml:"enabled"`
	Workspace                    string `toml:"workspace"`
	ObserverPeer                 string `toml:"observer_peer"`
	RecentMessages               int    `toml:"recent_messages"`
	MaxMessageSize               int    `toml:"max_message_size"`
	MaxFileSize                  int    `toml:"max_file_size"`
	GetContextMaxTokens          int    `toml:"get_context_max_tokens"`
	ReasoningEnabled             bool   `toml:"reasoning_enabled"`
	PeerCardEnabled              bool   `toml:"peer_card_enabled"`
	SummaryEnabled               bool   `toml:"summary_enabled"`
	DreamEnabled                 bool   `toml:"dream_enabled"`
	DreamIdleTimeoutMinutes      int    `toml:"dream_idle_timeout_minutes"`
	DeriverWorkers               int    `toml:"deriver_workers"`
	RepresentationBatchMaxTokens int    `toml:"representation_batch_max_tokens"`
	DialecticDefaultLevel        string `toml:"dialectic_default_level"`
}

func (g GonchoCfg) RuntimeConfig() goncho.Config {
	return goncho.Config{
		Enabled:                      g.Enabled,
		WorkspaceID:                  g.Workspace,
		ObserverPeerID:               g.ObserverPeer,
		RecentMessages:               g.RecentMessages,
		MaxMessageSize:               g.MaxMessageSize,
		MaxFileSize:                  g.MaxFileSize,
		GetContextMaxTokens:          g.GetContextMaxTokens,
		ReasoningEnabled:             g.ReasoningEnabled,
		PeerCardEnabled:              g.PeerCardEnabled,
		SummaryEnabled:               g.SummaryEnabled,
		DreamEnabled:                 g.DreamEnabled,
		DreamIdleTimeout:             time.Duration(g.DreamIdleTimeoutMinutes) * time.Minute,
		DeriverWorkers:               g.DeriverWorkers,
		RepresentationBatchMaxTokens: g.RepresentationBatchMaxTokens,
		DialecticDefaultLevel:        goncho.DialecticLevel(g.DialecticDefaultLevel),
	}
}

func (d *DelegationCfg) UnmarshalTOML(data []byte) error {
	type rawDelegationCfg struct {
		Enabled               bool   `toml:"enabled"`
		MaxDepth              int    `toml:"max_depth"`
		MaxConcurrentChildren int    `toml:"max_concurrent_children"`
		DefaultMaxIterations  int    `toml:"default_max_iterations"`
		DefaultTimeout        string `toml:"default_timeout"`
		RunLogPath            string `toml:"run_log_path"`
		MaxWaiting            int    `toml:"max_waiting"`
	}

	var raw rawDelegationCfg
	if err := toml.Unmarshal(data, &raw); err != nil {
		return err
	}

	*d = DelegationCfg{
		Enabled:               raw.Enabled,
		MaxDepth:              raw.MaxDepth,
		MaxConcurrentChildren: raw.MaxConcurrentChildren,
		DefaultMaxIterations:  raw.DefaultMaxIterations,
		RunLogPath:            raw.RunLogPath,
		MaxWaiting:            raw.MaxWaiting,
	}
	if raw.DefaultTimeout == "" {
		return nil
	}

	dur, err := time.ParseDuration(raw.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("delegation.default_timeout: %w", err)
	}
	d.DefaultTimeout = dur
	return nil
}

type HermesCfg struct {
	Endpoint string `toml:"endpoint"`
	APIKey   string `toml:"api_key"`
	Model    string `toml:"model"`
}

type GatewayCfg struct {
	ProxyURL string `toml:"proxy_url"`
	ProxyKey string `toml:"proxy_key"`
}

type TUICfg struct {
	Theme         string `toml:"theme"`
	MouseTracking bool   `toml:"mouse_tracking"`
}

type InputCfg struct {
	MaxBytes int `toml:"max_bytes"`
	MaxLines int `toml:"max_lines"`
}

// Load resolves configuration from (in precedence order) CLI flags, env vars,
// a TOML file at $XDG_CONFIG_HOME/gormes/config.toml, and built-in defaults.
// Pass os.Args[1:] as args; pass nil to skip flag parsing entirely (useful in tests).
//
// Before anything else, dotenv files at (in decreasing precedence) the
// Gormes XDG config dir and the legacy Hermes home are read into the
// process environment — any key NOT already in the shell env is set
// from the file. This lets operators migrating from Hermes keep their
// `~/.hermes/.env` working without re-keying ~170 secrets.
func Load(args []string) (Config, error) {
	loadDotenvFiles() // populates os.Setenv for unset keys BEFORE loadEnv reads them
	cfg := defaults()
	if err := loadFile(&cfg); err != nil {
		return cfg, err
	}
	if err := loadEnv(&cfg); err != nil {
		return cfg, err
	}
	if err := loadFlags(&cfg, args); err != nil {
		return cfg, err
	}
	if err := validateConfig(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func defaults() Config {
	return Config{
		ConfigVersion: CurrentConfigVersion,
		Hermes: HermesCfg{
			Endpoint: "http://127.0.0.1:8642",
			Model:    "hermes-agent",
		},
		TUI:   TUICfg{Theme: "dark", MouseTracking: true},
		Input: InputCfg{MaxBytes: 200_000, MaxLines: 10_000},
		Telegram: TelegramCfg{
			CoalesceMs:             1000,
			FirstRunDiscovery:      true,
			MemoryQueueCap:         1024,
			ExtractorBatchSize:     5,
			ExtractorPollInterval:  10 * time.Second,
			RecallEnabled:          true,
			RecallWeightThreshold:  1.0,
			RecallMaxFacts:         10,
			RecallDepth:            2,
			RecallDecayHorizonDays: 180,
			MirrorEnabled:          true,
			MirrorPath:             filepath.Join(xdgDataHome(), "gormes", "memory", "USER.md"),
			MirrorInterval:         30 * time.Second,
			SemanticEnabled:        false,
			SemanticEndpoint:       "",
			SemanticModel:          "",
			SemanticTopK:           3,
			SemanticMinSimilarity:  0.35,
			EmbedderPollInterval:   30 * time.Second,
			EmbedderBatchSize:      10,
			EmbedderCallTimeout:    10 * time.Second,
			QueryEmbedTimeout:      60 * time.Millisecond,
		},
		Discord: DiscordCfg{
			CoalesceMs:        1000,
			FirstRunDiscovery: false,
		},
		Slack: SlackCfg{
			Enabled:           false,
			CoalesceMs:        1000,
			FirstRunDiscovery: false,
		},
		Cron: CronCfg{
			Enabled:        false,
			CallTimeout:    60 * time.Second,
			MirrorInterval: 30 * time.Second,
			MirrorPath:     "",
		},
		Skills: SkillsCfg{
			SelectionCap:     3,
			MaxDocumentBytes: 64 * 1024,
			UsageLogPath:     "",
		},
		Delegation: DelegationCfg{
			Enabled:               false,
			MaxDepth:              2,
			MaxConcurrentChildren: 3,
			DefaultMaxIterations:  8,
			DefaultTimeout:        45 * time.Second,
			RunLogPath:            "",
			MaxWaiting:            128,
		},
		Goncho: GonchoCfg{
			Enabled:                      true,
			Workspace:                    goncho.DefaultWorkspaceID,
			ObserverPeer:                 goncho.DefaultObserverPeerID,
			RecentMessages:               goncho.DefaultRecentMessages,
			MaxMessageSize:               goncho.DefaultMaxMessageSize,
			MaxFileSize:                  goncho.DefaultMaxFileSize,
			GetContextMaxTokens:          goncho.DefaultGetContextMaxTokens,
			ReasoningEnabled:             true,
			PeerCardEnabled:              true,
			SummaryEnabled:               true,
			DreamEnabled:                 false,
			DreamIdleTimeoutMinutes:      int(goncho.DefaultDreamIdleTimeout / time.Minute),
			DeriverWorkers:               goncho.DefaultDeriverWorkers,
			RepresentationBatchMaxTokens: goncho.DefaultRepresentationBatchMaxTokens,
			DialecticDefaultLevel:        string(goncho.DialecticLevelLow),
		},
	}
}

func loadFile(cfg *Config) error {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := toml.NewDecoder(bytes.NewReader(data)).EnableUnmarshalerInterface().Decode(cfg); err != nil {
		return err
	}
	// Absent _config_version in TOML = treat as v1 (pre-versioning files).
	// Defaults() set it to CurrentConfigVersion, but unmarshal overwrites
	// with 0 when the key isn't present.
	if cfg.ConfigVersion == 0 {
		cfg.ConfigVersion = 1
	}
	return migrateConfig(cfg)
}

// migrateConfig applies version-gated schema migrations in sequence,
// bumping cfg.ConfigVersion after each step. A config written by a
// newer binary (version > CurrentConfigVersion) is rejected with a
// clear error so operators know to upgrade — silently downgrading
// would quietly drop unknown fields.
func migrateConfig(cfg *Config) error {
	if cfg.ConfigVersion > CurrentConfigVersion {
		return fmt.Errorf(
			"config: _config_version=%d is from a newer binary (this binary knows up to v%d); upgrade gormes or hand-edit the file",
			cfg.ConfigVersion, CurrentConfigVersion)
	}
	// No migrations defined yet (v1 is the first version). When a v1->v2
	// schema change ships, add:
	//   if cfg.ConfigVersion == 1 { migrate1to2(cfg); cfg.ConfigVersion = 2 }
	// Each step is idempotent because it only runs when ConfigVersion
	// matches its source version.
	cfg.ConfigVersion = CurrentConfigVersion
	return nil
}

func loadEnv(cfg *Config) error {
	if v := os.Getenv("GORMES_ENDPOINT"); v != "" {
		cfg.Hermes.Endpoint = v
	}
	if v := os.Getenv("GORMES_MODEL"); v != "" {
		cfg.Hermes.Model = v
	}
	if v := os.Getenv("GORMES_API_KEY"); v != "" {
		cfg.Hermes.APIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("GATEWAY_PROXY_URL")); v != "" {
		cfg.Gateway.ProxyURL = v
	}
	if v := strings.TrimSpace(os.Getenv("GATEWAY_PROXY_KEY")); v != "" {
		cfg.Gateway.ProxyKey = v
	}
	if v := os.Getenv("GORMES_TUI_MOUSE_TRACKING"); v != "" {
		parsed, err := parseEnvBool("GORMES_TUI_MOUSE_TRACKING", v)
		if err != nil {
			return err
		}
		cfg.TUI.MouseTracking = parsed
	}
	if v := os.Getenv("GORMES_TELEGRAM_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
	if v := os.Getenv("GORMES_TELEGRAM_CHAT_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Telegram.AllowedChatID = id
		}
	}
	if v := os.Getenv("GORMES_DISCORD_TOKEN"); v != "" {
		cfg.Discord.Token = v
	}
	if v := os.Getenv("GORMES_DISCORD_CHANNEL_ID"); v != "" {
		cfg.Discord.AllowedChannelID = v
	}
	if v := os.Getenv("GORMES_DISCORD_SERVER_ACTIONS"); v != "" {
		cfg.Discord.ServerActions = parseEnvCSV(v)
	}
	if v := os.Getenv("GORMES_SLACK_ENABLED"); v != "" {
		parsed, err := parseEnvBool("GORMES_SLACK_ENABLED", v)
		if err != nil {
			return err
		}
		cfg.Slack.Enabled = parsed
	}
	if v := os.Getenv("GORMES_SLACK_BOT_TOKEN"); v != "" {
		cfg.Slack.BotToken = v
	}
	if v := os.Getenv("GORMES_SLACK_APP_TOKEN"); v != "" {
		cfg.Slack.AppToken = v
	}
	if v := os.Getenv("GORMES_SLACK_CHANNEL_ID"); v != "" {
		cfg.Slack.AllowedChannelID = v
	}
	if v := os.Getenv("GORMES_SLACK_COALESCE_MS"); v != "" {
		parsed, err := parseEnvInt("GORMES_SLACK_COALESCE_MS", v)
		if err != nil {
			return err
		}
		cfg.Slack.CoalesceMs = parsed
	}
	if v := os.Getenv("GORMES_SLACK_FIRST_RUN_DISCOVERY"); v != "" {
		parsed, err := parseEnvBool("GORMES_SLACK_FIRST_RUN_DISCOVERY", v)
		if err != nil {
			return err
		}
		cfg.Slack.FirstRunDiscovery = parsed
	}
	if v := os.Getenv("GORMES_SKILLS_ROOT"); v != "" {
		cfg.Skills.Root = v
	}
	if v := os.Getenv("GORMES_GONCHO_ENABLED"); v != "" {
		parsed, err := parseEnvBool("GORMES_GONCHO_ENABLED", v)
		if err != nil {
			return err
		}
		cfg.Goncho.Enabled = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_WORKSPACE"); v != "" {
		cfg.Goncho.Workspace = v
	}
	if v := os.Getenv("GORMES_GONCHO_OBSERVER_PEER"); v != "" {
		cfg.Goncho.ObserverPeer = v
	}
	if v := os.Getenv("GORMES_GONCHO_RECENT_MESSAGES"); v != "" {
		parsed, err := parseEnvInt("GORMES_GONCHO_RECENT_MESSAGES", v)
		if err != nil {
			return err
		}
		cfg.Goncho.RecentMessages = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_MAX_MESSAGE_SIZE"); v != "" {
		parsed, err := parseEnvInt("GORMES_GONCHO_MAX_MESSAGE_SIZE", v)
		if err != nil {
			return err
		}
		cfg.Goncho.MaxMessageSize = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_MAX_FILE_SIZE"); v != "" {
		parsed, err := parseEnvInt("GORMES_GONCHO_MAX_FILE_SIZE", v)
		if err != nil {
			return err
		}
		cfg.Goncho.MaxFileSize = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_GET_CONTEXT_MAX_TOKENS"); v != "" {
		parsed, err := parseEnvInt("GORMES_GONCHO_GET_CONTEXT_MAX_TOKENS", v)
		if err != nil {
			return err
		}
		cfg.Goncho.GetContextMaxTokens = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_REASONING_ENABLED"); v != "" {
		parsed, err := parseEnvBool("GORMES_GONCHO_REASONING_ENABLED", v)
		if err != nil {
			return err
		}
		cfg.Goncho.ReasoningEnabled = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_PEER_CARD_ENABLED"); v != "" {
		parsed, err := parseEnvBool("GORMES_GONCHO_PEER_CARD_ENABLED", v)
		if err != nil {
			return err
		}
		cfg.Goncho.PeerCardEnabled = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_SUMMARY_ENABLED"); v != "" {
		parsed, err := parseEnvBool("GORMES_GONCHO_SUMMARY_ENABLED", v)
		if err != nil {
			return err
		}
		cfg.Goncho.SummaryEnabled = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_DREAM_ENABLED"); v != "" {
		parsed, err := parseEnvBool("GORMES_GONCHO_DREAM_ENABLED", v)
		if err != nil {
			return err
		}
		cfg.Goncho.DreamEnabled = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_DREAM_IDLE_TIMEOUT_MINUTES"); v != "" {
		parsed, err := parseEnvInt("GORMES_GONCHO_DREAM_IDLE_TIMEOUT_MINUTES", v)
		if err != nil {
			return err
		}
		cfg.Goncho.DreamIdleTimeoutMinutes = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_DERIVER_WORKERS"); v != "" {
		parsed, err := parseEnvInt("GORMES_GONCHO_DERIVER_WORKERS", v)
		if err != nil {
			return err
		}
		cfg.Goncho.DeriverWorkers = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_REPRESENTATION_BATCH_MAX_TOKENS"); v != "" {
		parsed, err := parseEnvInt("GORMES_GONCHO_REPRESENTATION_BATCH_MAX_TOKENS", v)
		if err != nil {
			return err
		}
		cfg.Goncho.RepresentationBatchMaxTokens = parsed
	}
	if v := os.Getenv("GORMES_GONCHO_DIALECTIC_DEFAULT_LEVEL"); v != "" {
		cfg.Goncho.DialecticDefaultLevel = v
	}
	return nil
}

func parseEnvBool(name, value string) (bool, error) {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("config env %s: %w", name, err)
	}
	return parsed, nil
}

func parseEnvInt(name, value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("config env %s: %w", name, err)
	}
	return parsed, nil
}

func parseEnvCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func loadFlags(cfg *Config, args []string) error {
	if args == nil {
		return nil
	}
	fs := pflag.NewFlagSet("gormes", pflag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "Hermes api_server base URL")
	model := fs.String("model", "", "served model name")
	resume := fs.String("resume", "", "override persisted session_id for this binary's default key")
	// No --api-key flag — secrets stay out of process argv.
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *endpoint != "" {
		cfg.Hermes.Endpoint = *endpoint
	}
	if *model != "" {
		cfg.Hermes.Model = *model
	}
	if *resume != "" {
		cfg.Resume = *resume
	}
	return nil
}

func validateConfig(cfg *Config) error {
	cfg.Gateway.ProxyURL = normalizeGatewayProxyURL(cfg.Gateway.ProxyURL)
	cfg.Gateway.ProxyKey = strings.TrimSpace(cfg.Gateway.ProxyKey)
	cfg.Slack.BotToken = strings.TrimSpace(cfg.Slack.BotToken)
	cfg.Slack.AppToken = strings.TrimSpace(cfg.Slack.AppToken)
	cfg.Slack.AllowedChannelID = strings.TrimSpace(cfg.Slack.AllowedChannelID)
	cfg.Goncho.Workspace = strings.TrimSpace(cfg.Goncho.Workspace)
	cfg.Goncho.ObserverPeer = strings.TrimSpace(cfg.Goncho.ObserverPeer)
	cfg.Goncho.DialecticDefaultLevel = strings.ToLower(strings.TrimSpace(cfg.Goncho.DialecticDefaultLevel))

	if cfg.Goncho.Workspace == "" {
		return fmt.Errorf("config: goncho.workspace is required")
	}
	if cfg.Goncho.ObserverPeer == "" {
		return fmt.Errorf("config: goncho.observer_peer is required")
	}
	if !goncho.ValidDialecticLevel(cfg.Goncho.DialecticDefaultLevel) {
		return fmt.Errorf("config: goncho.dialectic_default_level %q is invalid; want one of minimal, low, medium, high, max", cfg.Goncho.DialecticDefaultLevel)
	}
	for _, limit := range []struct {
		name  string
		value int
	}{
		{name: "recent_messages", value: cfg.Goncho.RecentMessages},
		{name: "max_message_size", value: cfg.Goncho.MaxMessageSize},
		{name: "max_file_size", value: cfg.Goncho.MaxFileSize},
		{name: "get_context_max_tokens", value: cfg.Goncho.GetContextMaxTokens},
		{name: "dream_idle_timeout_minutes", value: cfg.Goncho.DreamIdleTimeoutMinutes},
		{name: "deriver_workers", value: cfg.Goncho.DeriverWorkers},
		{name: "representation_batch_max_tokens", value: cfg.Goncho.RepresentationBatchMaxTokens},
	} {
		if limit.value < 0 {
			return fmt.Errorf("config: goncho.%s must be non-negative, got %d", limit.name, limit.value)
		}
	}
	if cfg.Goncho.DeriverWorkers == 0 {
		return fmt.Errorf("config: goncho.deriver_workers must be at least 1")
	}
	if cfg.Delegation.MaxWaiting < 0 {
		return fmt.Errorf("config: delegation.max_waiting must be non-negative, got %d", cfg.Delegation.MaxWaiting)
	}
	return nil
}

func normalizeGatewayProxyURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func xdgConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

// ConfigPath returns the Gormes TOML config file path resolved from XDG rules.
func ConfigPath() string {
	return filepath.Join(xdgConfigHome(), "gormes", "config.toml")
}

// LogPath returns the default path for the Gormes log file.
func LogPath() string {
	return filepath.Join(xdgDataHome(), "gormes", "gormes.log")
}

// CrashLogDir returns the directory where TUI panic dumps are written.
func CrashLogDir() string {
	return filepath.Join(xdgDataHome(), "gormes")
}

// SessionDBPath returns the default location of the bbolt sessions map.
// Honors XDG_DATA_HOME; falls back to ~/.local/share/gormes/sessions.db.
func SessionDBPath() string {
	return filepath.Join(xdgDataHome(), "gormes", "sessions.db")
}

// SessionIndexMirrorPath returns the default location of the read-only YAML
// mirror for the bbolt session map.
func SessionIndexMirrorPath() string {
	return filepath.Join(xdgDataHome(), "gormes", "sessions", "index.yaml")
}

// MemoryDBPath returns the default location of the Phase-3.A SQLite
// memory database. Honors XDG_DATA_HOME; falls back to
// ~/.local/share/gormes/memory.db.
func MemoryDBPath() string {
	return filepath.Join(xdgDataHome(), "gormes", "memory.db")
}

// CronMirrorPath returns the resolved CRON.md path — either
// cfg.Cron.MirrorPath (explicit override) or the XDG default
// $XDG_DATA_HOME/gormes/cron/CRON.md.
func (c Config) CronMirrorPath() string {
	if c.Cron.MirrorPath != "" {
		return c.Cron.MirrorPath
	}
	return filepath.Join(xdgDataHome(), "gormes", "cron", "CRON.md")
}

// SkillsRoot returns the root directory of the static skills runtime.
// Explicit override wins; otherwise the XDG default is used.
func (c Config) SkillsRoot() string {
	if c.Skills.Root != "" {
		return c.Skills.Root
	}
	return filepath.Join(xdgDataHome(), "gormes", "skills")
}

// HooksRoot returns the root directory for gateway HOOK.yaml hook directories.
// Gormes uses XDG data paths for live state instead of ~/.hermes/.
func HooksRoot() string {
	return filepath.Join(xdgDataHome(), "gormes", "hooks")
}

// GatewayRuntimeStatusPath returns the shared gateway_state.json read-model
// path for live gateway lifecycle status.
func GatewayRuntimeStatusPath() string {
	return filepath.Join(xdgDataHome(), "gormes", "gateway_state.json")
}

// BootPath returns the BOOT.md path used by the built-in gateway startup hook.
func BootPath() string {
	return filepath.Join(xdgDataHome(), "gormes", "BOOT.md")
}

// SkillsUsageLogPath returns the append-only JSONL path for skill usage.
// Explicit override wins; otherwise it lives under the skills root.
func (c Config) SkillsUsageLogPath() string {
	if c.Skills.UsageLogPath != "" {
		return c.Skills.UsageLogPath
	}
	return filepath.Join(c.SkillsRoot(), "usage.jsonl")
}

// ToolAuditLogPath returns the append-only JSONL path for tool execution
// audit records.
func ToolAuditLogPath() string {
	return filepath.Join(xdgDataHome(), "gormes", "tools", "audit.jsonl")
}

// ResolvedRunLogPath returns the JSONL path for append-only subagent run logs.
// An explicit TOML override wins; otherwise Gormes writes under XDG_DATA_HOME.
func (d DelegationCfg) ResolvedRunLogPath() string {
	if d.RunLogPath != "" {
		return d.RunLogPath
	}
	return filepath.Join(xdgDataHome(), "gormes", "subagents", "runs.jsonl")
}

// LegacyHermesHome reports an upstream Hermes state directory if one is
// discoverable. Returns the path and true when either $HERMES_HOME is
// set OR ~/.hermes/ exists. Gormes does NOT read state from this path
// at runtime — it uses XDG directories exclusively — but surfacing the
// detection at startup lets operators know their Hermes state wasn't
// silently ignored.
//
// Planned: Phase 5.O will add a `gormes migrate --from-hermes` command
// that copies relevant state (sessions, memory snapshots) across.
// Until then, operators see a one-line info log and can migrate
// manually.
func LegacyHermesHome() (string, bool) {
	if v := strings.TrimSpace(os.Getenv("HERMES_HOME")); v != "" {
		return v, true
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	candidate := filepath.Join(home, ".hermes")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, true
	}
	return "", false
}
