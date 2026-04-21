# Gormes Phase 2.B.2-2.B.3 — Discord + Slack Batch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `gormes discord` and `gormes slack` on top of the existing Hermes kernel, session, memory, and tool runtime while reusing PicoClaw's channel-edge patterns and extracting only a narrow shared runtime boot from the Telegram command.

**Architecture:** The only shared extraction in this batch is `cmd/gormes/gateway_runtime.go`, which owns the cross-command boot path already duplicated in `cmd/gormes/telegram.go`. Discord and Slack stay as sibling adapter packages (`internal/discord`, `internal/slack`) that mirror Telegram's split: SDK wrapper, bot loop, render/coalesce helpers, tests. Single-kernel semantics remain intact in this batch: one operative conversation key per command, Discord DM mode means one active DM channel per process, and Slack thread awareness is transport-only rather than per-thread kernel isolation.

**Tech Stack:** Go 1.25, `github.com/bwmarrin/discordgo`, `github.com/slack-go/slack`, `github.com/slack-go/slack/socketmode`, existing `cobra`, `bbolt`, SQLite memory store, Hermes HTTP client, and the shipped Telegram adapter as the Gormes-side reference shape.

**Spec:** [`gormes/docs/superpowers/specs/2026-04-21-gormes-phase2b2-discord-slack-design.md`](../specs/2026-04-21-gormes-phase2b2-discord-slack-design.md)

**Primary Donor References:** PicoClaw Discord edge patterns come from `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw/pkg/channels/discord/discord.go`. Slack Socket Mode, thread, and ACK behavior come from `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw/pkg/channels/slack/slack.go`. Shared channel decomposition cues come from `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw/pkg/channels/base.go`, `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw/pkg/channels/manager.go`, and `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw/pkg/channels/split.go`, but none of those runtime ownership patterns should be copied wholesale into Gormes.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `gormes/go.mod` | Modify | Add Discord and Slack SDK dependencies |
| `gormes/go.sum` | Modify | Lock dependency hashes |
| `gormes/internal/config/config.go` | Modify | Add `DiscordCfg` and `SlackCfg`, defaults, env loading |
| `gormes/internal/config/config_test.go` | Modify | TDD for new config sections and env overrides |
| `gormes/cmd/gormes/gateway_runtime.go` | Create | Shared runtime boot extracted from `telegram.go` |
| `gormes/cmd/gormes/gateway_runtime_test.go` | Create | TDD for session resume, chat-key wiring, recall gating |
| `gormes/cmd/gormes/main.go` | Modify | Introduce `newRootCmd()` and register `discord` + `slack` |
| `gormes/cmd/gormes/telegram.go` | Modify | Replace duplicated boot path with `openGatewayRuntime()` |
| `gormes/cmd/gormes/discord.go` | Create | `gormes discord` command, validation, startup, shutdown |
| `gormes/cmd/gormes/discord_test.go` | Create | TDD for Discord config validation and root registration |
| `gormes/cmd/gormes/slack.go` | Create | `gormes slack` command, validation, startup, shutdown |
| `gormes/cmd/gormes/slack_test.go` | Create | TDD for Slack config validation and root registration |
| `gormes/internal/discord/client.go` | Create | Narrow adapter-facing Discord client interface and event structs |
| `gormes/internal/discord/real_client.go` | Create | `discordgo` wrapper that satisfies the narrow client interface |
| `gormes/internal/discord/render.go` | Create | Discord-specific stream/final/error text formatting |
| `gormes/internal/discord/coalesce.go` | Create | Coalesced send/edit loop for Discord streaming |
| `gormes/internal/discord/bot.go` | Create | Discord adapter run loop, filtering, command parsing, typing, persistence |
| `gormes/internal/discord/mock_test.go` | Create | Mock Discord client and scripted inbound helpers |
| `gormes/internal/discord/bot_test.go` | Create | TDD for Discord inbound/outbound behavior |
| `gormes/internal/slack/client.go` | Create | Narrow Socket Mode client interface and normalized Slack event struct |
| `gormes/internal/slack/real_client.go` | Create | `slack-go/slack` + `socketmode` wrapper |
| `gormes/internal/slack/render.go` | Create | Slack text formatting for placeholder/final/error frames |
| `gormes/internal/slack/bot.go` | Create | Slack event loop, ACK path, thread-aware reply routing, persistence |
| `gormes/internal/slack/mock_test.go` | Create | Mock Slack client with deterministic event/ack tracking |
| `gormes/internal/slack/bot_test.go` | Create | TDD for Slack ACK, filtering, thread routing, outbound behavior |
| `gormes/internal/buildisolation_test.go` | Modify | Guard `internal/kernel` from taking Discord/Slack SDK deps |
| `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md` | Modify | Mark Discord + Slack batch as shipped and describe the narrow shared runtime boot |
| `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md` | Modify | Flip Discord and Slack rows to shipped and link implementation docs/specs |
| `gormes/docs/content/building-gormes/gateway-donor-map/shared-adapter-patterns.md` | Modify | Add direct references from donor patterns to landed Gormes files |

## Task 1: Add Discord + Slack dependencies and config surfaces

**Files:**
- Modify: `gormes/go.mod`
- Modify: `gormes/go.sum`
- Modify: `gormes/internal/config/config.go`
- Modify: `gormes/internal/config/config_test.go`

**Reference donors:**
- `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw/pkg/channels/discord/discord.go`
- `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw/pkg/channels/slack/slack.go`

- [ ] **Step 1: Write the failing config tests**

Append these tests to `gormes/internal/config/config_test.go`:

```go
func TestLoad_DiscordDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Discord.MentionRequired != true {
		t.Errorf("Discord.MentionRequired default = %v, want true", cfg.Discord.MentionRequired)
	}
	if cfg.Discord.CoalesceMs != 1000 {
		t.Errorf("Discord.CoalesceMs default = %d, want 1000", cfg.Discord.CoalesceMs)
	}
}

func TestLoad_DiscordEnvOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GORMES_DISCORD_TOKEN", "discord-token")
	t.Setenv("GORMES_DISCORD_CHANNEL_ID", "chan-1")
	t.Setenv("GORMES_DISCORD_GUILD_ID", "guild-1")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Discord.BotToken != "discord-token" {
		t.Errorf("Discord.BotToken = %q, want discord-token", cfg.Discord.BotToken)
	}
	if cfg.Discord.AllowedChannelID != "chan-1" {
		t.Errorf("Discord.AllowedChannelID = %q, want chan-1", cfg.Discord.AllowedChannelID)
	}
	if cfg.Discord.AllowedGuildID != "guild-1" {
		t.Errorf("Discord.AllowedGuildID = %q, want guild-1", cfg.Discord.AllowedGuildID)
	}
}

func TestLoad_SlackDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Slack.SocketMode {
		t.Error("Slack.SocketMode default = false, want true")
	}
	if !cfg.Slack.ReplyInThread {
		t.Error("Slack.ReplyInThread default = false, want true")
	}
	if cfg.Slack.CoalesceMs != 1000 {
		t.Errorf("Slack.CoalesceMs default = %d, want 1000", cfg.Slack.CoalesceMs)
	}
}

func TestLoad_SlackEnvOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GORMES_SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("GORMES_SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("GORMES_SLACK_CHANNEL_ID", "C12345")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Slack.BotToken != "xoxb-test" {
		t.Errorf("Slack.BotToken = %q, want xoxb-test", cfg.Slack.BotToken)
	}
	if cfg.Slack.AppToken != "xapp-test" {
		t.Errorf("Slack.AppToken = %q, want xapp-test", cfg.Slack.AppToken)
	}
	if cfg.Slack.AllowedChannelID != "C12345" {
		t.Errorf("Slack.AllowedChannelID = %q, want C12345", cfg.Slack.AllowedChannelID)
	}
}
```

- [ ] **Step 2: Run the tests to prove the fields do not exist yet**

```bash
cd gormes
go test ./internal/config -count=1
```

Expected: FAIL with compile errors such as `cfg.Discord undefined` and `cfg.Slack undefined`.

- [ ] **Step 3: Add the SDK modules up front**

```bash
cd gormes
go get github.com/bwmarrin/discordgo@latest
go get github.com/slack-go/slack@latest
go get github.com/slack-go/slack/socketmode@latest
```

Expected: `go.mod` gains the Discord and Slack modules; `go.sum` updates.

- [ ] **Step 4: Implement `DiscordCfg` and `SlackCfg`**

Update `gormes/internal/config/config.go` with these additions:

```go
type Config struct {
	ConfigVersion int `toml:"_config_version"`

	Hermes     HermesCfg     `toml:"hermes"`
	TUI        TUICfg        `toml:"tui"`
	Input      InputCfg      `toml:"input"`
	Telegram   TelegramCfg   `toml:"telegram"`
	Discord    DiscordCfg    `toml:"discord"`
	Slack      SlackCfg      `toml:"slack"`
	Cron       CronCfg       `toml:"cron"`
	Delegation DelegationCfg `toml:"delegation"`
	Resume     string        `toml:"-"`
}

type DiscordCfg struct {
	BotToken         string `toml:"bot_token"`
	AllowedChannelID string `toml:"allowed_channel_id"`
	AllowedGuildID   string `toml:"allowed_guild_id"`
	MentionRequired  bool   `toml:"mention_required"`
	CoalesceMs       int    `toml:"coalesce_ms"`
}

type SlackCfg struct {
	BotToken         string `toml:"bot_token"`
	AppToken         string `toml:"app_token"`
	AllowedChannelID string `toml:"allowed_channel_id"`
	SocketMode       bool   `toml:"socket_mode"`
	CoalesceMs       int    `toml:"coalesce_ms"`
	ReplyInThread    bool   `toml:"reply_in_thread"`
}

func defaults() Config {
	return Config{
		ConfigVersion: CurrentConfigVersion,
		Hermes: HermesCfg{
			Endpoint: "http://127.0.0.1:8642",
			Model:    "hermes-agent",
		},
		TUI:   TUICfg{Theme: "dark"},
		Input: InputCfg{MaxBytes: 200_000, MaxLines: 10_000},
		Telegram: TelegramCfg{
			CoalesceMs:            1000,
			FirstRunDiscovery:     true,
			MemoryQueueCap:        1024,
			ExtractorBatchSize:    5,
			ExtractorPollInterval: 10 * time.Second,
			RecallEnabled:         true,
			RecallWeightThreshold: 1.0,
			RecallMaxFacts:        10,
			RecallDepth:           2,
			RecallDecayHorizonDays: 180,
			MirrorEnabled:         true,
			MirrorPath:            filepath.Join(xdgDataHome(), "gormes", "memory", "USER.md"),
			MirrorInterval:        30 * time.Second,
			SemanticEnabled:       false,
			SemanticTopK:          3,
			SemanticMinSimilarity: 0.35,
			EmbedderPollInterval:  30 * time.Second,
			EmbedderBatchSize:     10,
			EmbedderCallTimeout:   10 * time.Second,
			QueryEmbedTimeout:     60 * time.Millisecond,
		},
		Discord: DiscordCfg{
			MentionRequired: true,
			CoalesceMs:      1000,
		},
		Slack: SlackCfg{
			SocketMode:    true,
			CoalesceMs:    1000,
			ReplyInThread: true,
		},
		Cron: CronCfg{
			Enabled:        false,
			CallTimeout:    60 * time.Second,
			MirrorInterval: 30 * time.Second,
			MirrorPath:     "",
		},
		Delegation: DelegationCfg{
			Enabled:              false,
			DefaultMaxIterations: 8,
			DefaultTimeout:       45 * time.Second,
			MaxChildDepth:        1,
			RunLogPath:           "",
		},
	}
}

func loadEnv(cfg *Config) {
	if v := os.Getenv("GORMES_ENDPOINT"); v != "" {
		cfg.Hermes.Endpoint = v
	}
	if v := os.Getenv("GORMES_MODEL"); v != "" {
		cfg.Hermes.Model = v
	}
	if v := os.Getenv("GORMES_API_KEY"); v != "" {
		cfg.Hermes.APIKey = v
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
		cfg.Discord.BotToken = v
	}
	if v := os.Getenv("GORMES_DISCORD_CHANNEL_ID"); v != "" {
		cfg.Discord.AllowedChannelID = v
	}
	if v := os.Getenv("GORMES_DISCORD_GUILD_ID"); v != "" {
		cfg.Discord.AllowedGuildID = v
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
}
```

- [ ] **Step 5: Run the config tests again**

```bash
cd gormes
go test ./internal/config -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd ..
git add gormes/go.mod gormes/go.sum gormes/internal/config/config.go gormes/internal/config/config_test.go
git commit -m "feat(gormes): add discord and slack config surfaces"
```

## Task 2: Extract the shared gateway runtime boot from `telegram.go`

**Files:**
- Create: `gormes/cmd/gormes/gateway_runtime.go`
- Create: `gormes/cmd/gormes/gateway_runtime_test.go`
- Modify: `gormes/cmd/gormes/telegram.go`

**Reference anchors:**
- `gormes/cmd/gormes/telegram.go`
- `gormes/internal/session/*`
- `gormes/internal/memory/*`
- `gormes/internal/kernel/*`

- [ ] **Step 1: Write the failing runtime tests**

Create `gormes/cmd/gormes/gateway_runtime_test.go`:

```go
package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

func testGatewayConfig() config.Config {
	return config.Config{
		Hermes: config.HermesCfg{
			Endpoint: "http://127.0.0.1:8642",
			Model:    "hermes-agent",
		},
		Input: config.InputCfg{
			MaxBytes: 200_000,
			MaxLines: 10_000,
		},
		Telegram: config.TelegramCfg{
			MemoryQueueCap:        8,
			ExtractorBatchSize:    1,
			ExtractorPollInterval: 10 * time.Second,
			RecallEnabled:         true,
			RecallWeightThreshold: 1.0,
			RecallMaxFacts:        10,
			RecallDepth:           2,
			RecallDecayHorizonDays: 180,
			SemanticEnabled:       false,
			QueryEmbedTimeout:     60 * time.Millisecond,
		},
	}
}

func TestOpenGatewayRuntime_AppliesResumeOverride(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	rt, err := openGatewayRuntime(testGatewayConfig(), gatewayRuntimeOptions{
		ChatKey:        "discord:chan-1",
		ResumeOverride: "sess-123",
		RecallEnabled:  true,
	}, slog.Default())
	if err != nil {
		t.Fatalf("openGatewayRuntime: %v", err)
	}
	defer rt.Close(context.Background())

	if rt.chatKey != "discord:chan-1" {
		t.Errorf("chatKey = %q, want discord:chan-1", rt.chatKey)
	}
	if rt.initialSID != "sess-123" {
		t.Errorf("initialSID = %q, want sess-123", rt.initialSID)
	}
}

func TestOpenGatewayRuntime_DisablesRecallWhenChatKeyEmpty(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	rt, err := openGatewayRuntime(testGatewayConfig(), gatewayRuntimeOptions{
		ChatKey:       "",
		RecallEnabled: true,
	}, slog.Default())
	if err != nil {
		t.Fatalf("openGatewayRuntime: %v", err)
	}
	defer rt.Close(context.Background())

	if rt.recallActive {
		t.Error("recallActive = true, want false when chatKey is empty")
	}
}
```

- [ ] **Step 2: Run the tests to prove the helper does not exist yet**

```bash
cd gormes
go test ./cmd/gormes -run 'TestOpenGatewayRuntime_' -count=1
```

Expected: FAIL with `undefined: openGatewayRuntime`, `undefined: gatewayRuntimeOptions`, or similar compile errors.

- [ ] **Step 3: Implement the shared runtime helper**

Create `gormes/cmd/gormes/gateway_runtime.go`:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

type gatewayRuntimeOptions struct {
	ChatKey        string
	ResumeOverride string
	RecallEnabled  bool
}

type gatewayRuntime struct {
	SessionMap   *session.BoltMap
	MemoryStore  *memory.SqliteStore
	Kernel       *kernel.Kernel
	Extractor    *memory.Extractor
	Embedder     *memory.Embedder
	chatKey      string
	initialSID   string
	recallActive bool
}

func openGatewayRuntime(cfg config.Config, opt gatewayRuntimeOptions, log *slog.Logger) (*gatewayRuntime, error) {
	if log == nil {
		log = slog.Default()
	}

	smap, err := session.OpenBolt(config.SessionDBPath())
	if err != nil {
		return nil, fmt.Errorf("session map: %w", err)
	}

	ctx := context.Background()
	if opt.ChatKey != "" && opt.ResumeOverride != "" {
		if err := smap.Put(ctx, opt.ChatKey, opt.ResumeOverride); err != nil {
			smap.Close()
			return nil, fmt.Errorf("apply resume override: %w", err)
		}
	}

	var initialSID string
	if opt.ChatKey != "" {
		if sid, err := smap.Get(ctx, opt.ChatKey); err == nil {
			initialSID = sid
		}
	}

	mstore, err := memory.OpenSqlite(config.MemoryDBPath(), cfg.Telegram.MemoryQueueCap, log)
	if err != nil {
		smap.Close()
		return nil, fmt.Errorf("memory store: %w", err)
	}

	mstore.StartMirror(memory.MirrorConfig{
		Enabled:  cfg.Telegram.MirrorEnabled,
		Path:     cfg.Telegram.MirrorPath,
		Interval: cfg.Telegram.MirrorInterval,
		Logger:   log,
	})

	hc := hermes.NewHTTPClient(cfg.Hermes.Endpoint, cfg.Hermes.APIKey)
	reg := buildDefaultRegistry()
	registerDelegation(cfg, reg, hc)
	tm := telemetry.New()

	recallActive := opt.RecallEnabled && opt.ChatKey != ""
	var recallProv kernel.RecallProvider
	if recallActive {
		recallProv = &recallAdapter{p: memory.NewRecall(mstore, memory.RecallConfig{
			WeightThreshold:       cfg.Telegram.RecallWeightThreshold,
			MaxFacts:              cfg.Telegram.RecallMaxFacts,
			Depth:                 cfg.Telegram.RecallDepth,
			DecayHorizonDays:      cfg.Telegram.RecallDecayHorizonDays,
			SemanticModel:         cfg.Telegram.SemanticModel,
			SemanticTopK:          cfg.Telegram.SemanticTopK,
			SemanticMinSimilarity: cfg.Telegram.SemanticMinSimilarity,
			QueryEmbedTimeout:     cfg.Telegram.QueryEmbedTimeout,
		}, log)}
	}

	k := kernel.New(kernel.Config{
		Model:             cfg.Hermes.Model,
		Endpoint:          cfg.Hermes.Endpoint,
		Admission:         kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
		InitialSessionID:  initialSID,
		Recall:            recallProv,
		ChatKey:           opt.ChatKey,
	}, hc, mstore, tm, log)

	ext := memory.NewExtractor(mstore, hc, memory.ExtractorConfig{
		Model:        cfg.Hermes.Model,
		BatchSize:    cfg.Telegram.ExtractorBatchSize,
		PollInterval: cfg.Telegram.ExtractorPollInterval,
	}, log)

	return &gatewayRuntime{
		SessionMap:   smap,
		MemoryStore:  mstore,
		Kernel:       k,
		Extractor:    ext,
		chatKey:      opt.ChatKey,
		initialSID:   initialSID,
		recallActive: recallActive,
	}, nil
}

func (rt *gatewayRuntime) Start(ctx context.Context) {
	go rt.Kernel.Run(ctx)
	go rt.Extractor.Run(ctx)
}

func (rt *gatewayRuntime) Close(ctx context.Context) {
	if rt.Embedder != nil {
		if err := rt.Embedder.Close(ctx); err != nil {
			slog.Warn("embedder close", "err", err)
		}
	}
	if rt.Extractor != nil {
		if err := rt.Extractor.Close(ctx); err != nil {
			slog.Warn("extractor close", "err", err)
		}
	}
	if rt.MemoryStore != nil {
		if err := rt.MemoryStore.Close(ctx); err != nil {
			slog.Warn("memory store close", "err", err)
		}
	}
	if rt.SessionMap != nil {
		_ = rt.SessionMap.Close()
	}
}
```

- [ ] **Step 4: Refactor `telegram.go` to consume the helper**

Replace the duplicated boot section in `gormes/cmd/gormes/telegram.go` with this shape:

```go
func runTelegram(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if p, ok := config.LegacyHermesHome(); ok {
		slog.Info("detected upstream Hermes home — Gormes uses XDG paths and does NOT read state from it; run `gormes migrate --from-hermes` (planned Phase 5.O) to import sessions and memory", "hermes_home", p)
	}

	if cfg.Telegram.BotToken == "" {
		return fmt.Errorf("no Telegram bot token — set GORMES_TELEGRAM_TOKEN env or [telegram].bot_token in config.toml")
	}
	if cfg.Telegram.AllowedChatID == 0 && !cfg.Telegram.FirstRunDiscovery {
		return fmt.Errorf("no chat allowlist and discovery disabled — set one of [telegram].allowed_chat_id or [telegram].first_run_discovery = true")
	}

	key := ""
	if cfg.Telegram.AllowedChatID != 0 {
		key = session.TelegramKey(cfg.Telegram.AllowedChatID)
	}

	rt, err := openGatewayRuntime(cfg, gatewayRuntimeOptions{
		ChatKey:        key,
		ResumeOverride: cfg.Resume,
		RecallEnabled:  cfg.Telegram.AllowedChatID != 0,
	}, slog.Default())
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
		defer cancelShutdown()
		rt.Close(shutdownCtx)
	}()

	tc, err := telegram.NewRealClient(cfg.Telegram.BotToken)
	if err != nil {
		return err
	}

	bot := telegram.New(telegram.Config{
		AllowedChatID:     cfg.Telegram.AllowedChatID,
		CoalesceMs:        cfg.Telegram.CoalesceMs,
		FirstRunDiscovery: cfg.Telegram.FirstRunDiscovery,
		SessionMap:        rt.SessionMap,
		SessionKey:        key,
	}, tc, rt.Kernel, slog.Default())

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rt.Start(rootCtx)
	return bot.Run(rootCtx)
}
```

Leave the existing Telegram cron block in place, but swap its dependencies to `rt.SessionMap.DB()`, `rt.MemoryStore.DB()`, and `rt.Kernel`.

- [ ] **Step 5: Run the runtime and Telegram command tests**

```bash
cd gormes
go test ./cmd/gormes -run 'TestOpenGatewayRuntime_|TestTelegramDeliverySink_' -count=1
go test ./internal/telegram -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd ..
git add gormes/cmd/gormes/gateway_runtime.go gormes/cmd/gormes/gateway_runtime_test.go gormes/cmd/gormes/telegram.go
git commit -m "refactor(gormes): extract shared gateway runtime boot"
```

## Task 3: Build the Discord adapter package from Telegram shape + PicoClaw donor cues

**Files:**
- Create: `gormes/internal/discord/client.go`
- Create: `gormes/internal/discord/real_client.go`
- Create: `gormes/internal/discord/render.go`
- Create: `gormes/internal/discord/coalesce.go`
- Create: `gormes/internal/discord/bot.go`
- Create: `gormes/internal/discord/mock_test.go`
- Create: `gormes/internal/discord/bot_test.go`

**Reference donors:**
- `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw/pkg/channels/discord/discord.go`
- `gormes/internal/telegram/client.go`
- `gormes/internal/telegram/bot.go`
- `gormes/internal/telegram/coalesce.go`
- `gormes/internal/telegram/render.go`

- [ ] **Step 1: Write the failing Discord adapter tests**

Create `gormes/internal/discord/bot_test.go`:

```go
package discord

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

func newScriptedKernel(reply, sid string) *kernel.Kernel {
	hc := hermes.NewMockClient()
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: len(reply)})
	hc.Script(events, sid)

	return kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hc, store.NewNoop(), telemetry.New(), nil)
}

func TestBot_SubmitsMentionedGuildMessage(t *testing.T) {
	mc := newMockClient("bot-1")
	k := newScriptedKernel("roger", "sess-discord-guild")
	b := New(Config{
		AllowedGuildID:   "guild-1",
		AllowedChannelID: "chan-1",
		MentionRequired:  true,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushMessage(InboundMessage{
		ChannelID:    "chan-1",
		GuildID:      "guild-1",
		AuthorID:     "user-1",
		Content:      "<@bot-1> hi",
		MentionedBot: true,
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastSentText(), "roger") {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("last sent text = %q, want streamed reply containing roger", mc.lastSentText())
}

func TestBot_RejectsGuildMessageWithoutMention(t *testing.T) {
	mc := newMockClient("bot-1")
	k := newScriptedKernel("unused", "sess-unused")
	b := New(Config{
		AllowedGuildID:   "guild-1",
		AllowedChannelID: "chan-1",
		MentionRequired:  true,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushMessage(InboundMessage{
		ChannelID:    "chan-1",
		GuildID:      "guild-1",
		AuthorID:     "user-1",
		Content:      "hi without mention",
		MentionedBot: false,
	})

	time.Sleep(100 * time.Millisecond)
	if got := len(mc.sentTexts()); got != 0 {
		t.Fatalf("sent texts = %d, want 0", got)
	}
}

func TestBot_AcceptsDMDiscoveryAndPersistsSession(t *testing.T) {
	mc := newMockClient("bot-1")
	k := newScriptedKernel("dm ok", "sess-discord-dm")
	smap := session.NewMemMap()
	b := New(Config{
		AllowedChannelID: "",
		MentionRequired:  true,
		CoalesceMs:       50,
		SessionMap:       smap,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushMessage(InboundMessage{
		ChannelID: "dm-42",
		AuthorID:  "user-1",
		Content:   "hello in dm",
		IsDM:      true,
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastSentText(), "dm ok") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	gotSID, err := smap.Get(context.Background(), SessionKey("dm-42"))
	if err != nil {
		t.Fatalf("Get persisted session: %v", err)
	}
	if gotSID != "sess-discord-dm" {
		t.Fatalf("persisted sid = %q, want sess-discord-dm", gotSID)
	}
}
```

- [ ] **Step 2: Run the tests and confirm the package is missing**

```bash
cd gormes
go test ./internal/discord -count=1
```

Expected: FAIL with `no required module provides package ./internal/discord` or compile errors from missing symbols.

- [ ] **Step 3: Create the narrow client seam and the mock**

Create `gormes/internal/discord/client.go`:

```go
package discord

type InboundMessage struct {
	ID           string
	ChannelID    string
	GuildID      string
	AuthorID     string
	Content      string
	IsDM         bool
	MentionedBot bool
}

type Client interface {
	Open() error
	Close() error
	SelfID() string
	SetMessageHandler(func(InboundMessage))
	Send(channelID, text string) (string, error)
	Edit(channelID, messageID, text string) error
	Typing(channelID string) error
}

func SessionKey(channelID string) string {
	return "discord:" + channelID
}
```

Create `gormes/internal/discord/mock_test.go`:

```go
package discord

import "sync"

type mockClient struct {
	selfID  string
	handler func(InboundMessage)

	mu          sync.Mutex
	sent        []string
	edits       []string
	typingCalls int
}

func newMockClient(selfID string) *mockClient {
	return &mockClient{selfID: selfID}
}

func (m *mockClient) Open() error  { return nil }
func (m *mockClient) Close() error { return nil }
func (m *mockClient) SelfID() string { return m.selfID }
func (m *mockClient) SetMessageHandler(fn func(InboundMessage)) { m.handler = fn }

func (m *mockClient) Send(_ string, text string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, text)
	return "msg-1", nil
}

func (m *mockClient) Edit(_ string, _ string, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.edits = append(m.edits, text)
	return nil
}

func (m *mockClient) Typing(_ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typingCalls++
	return nil
}

func (m *mockClient) pushMessage(msg InboundMessage) {
	if m.handler != nil {
		m.handler(msg)
	}
}

func (m *mockClient) sentTexts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.sent))
	copy(out, m.sent)
	return out
}

func (m *mockClient) lastSentText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.edits) > 0 {
		return m.edits[len(m.edits)-1]
	}
	if len(m.sent) > 0 {
		return m.sent[len(m.sent)-1]
	}
	return ""
}
```

- [ ] **Step 4: Implement the Discord adapter**

Create `gormes/internal/discord/bot.go`, `render.go`, `coalesce.go`, and `real_client.go` with this structure:

```go
package discord

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
)

type Config struct {
	AllowedGuildID   string
	AllowedChannelID string
	MentionRequired  bool
	CoalesceMs       int
	SessionMap       session.Map
}

type Bot struct {
	cfg             Config
	client          Client
	kernel          *kernel.Kernel
	log             *slog.Logger
	activeChannelID string
	lastSID         string
}

func New(cfg Config, client Client, k *kernel.Kernel, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	if cfg.CoalesceMs <= 0 {
		cfg.CoalesceMs = 1000
	}
	return &Bot{cfg: cfg, client: client, kernel: k, log: log}
}

func (b *Bot) Run(ctx context.Context) error {
	b.client.SetMessageHandler(func(msg InboundMessage) { b.handleMessage(ctx, msg) })
	if err := b.client.Open(); err != nil {
		return err
	}
	defer b.client.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go b.runOutbound(ctx, &wg)

	<-ctx.Done()
	wg.Wait()
	return nil
}

func (b *Bot) handleMessage(ctx context.Context, msg InboundMessage) {
	if !b.allowed(msg) {
		return
	}
	text := strings.TrimSpace(stripSelfMention(msg.Content, b.client.SelfID()))
	if text == "" {
		return
	}

	b.activeChannelID = msg.ChannelID

	switch text {
	case "/start":
		_, _ = b.client.Send(msg.ChannelID, "Gormes is online. Commands: /stop /new")
	case "/stop":
		_ = b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel})
	case "/new":
		if err := b.kernel.ResetSession(); err != nil {
			_, _ = b.client.Send(msg.ChannelID, "Cannot reset during active turn — send /stop first.")
			return
		}
		_, _ = b.client.Send(msg.ChannelID, "Session reset. Next message starts fresh.")
	default:
		if err := b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: text}); err != nil {
			_, _ = b.client.Send(msg.ChannelID, "Busy — try again in a second.")
		}
	}
}

func (b *Bot) allowed(msg InboundMessage) bool {
	if msg.IsDM {
		if b.cfg.AllowedChannelID == "" {
			return true
		}
		return msg.ChannelID == b.cfg.AllowedChannelID
	}
	if b.cfg.AllowedGuildID != "" && msg.GuildID != b.cfg.AllowedGuildID {
		return false
	}
	if b.cfg.AllowedChannelID != "" && msg.ChannelID != b.cfg.AllowedChannelID {
		return false
	}
	if b.cfg.MentionRequired && !msg.MentionedBot {
		return false
	}
	return true
}

func (b *Bot) runOutbound(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	frames := b.kernel.Render()
	var c *coalescer
	var cancelTyping context.CancelFunc

	for {
		select {
		case <-ctx.Done():
			if cancelTyping != nil {
				cancelTyping()
			}
			return
		case f, ok := <-frames:
			if !ok {
				return
			}
			b.persistIfChanged(ctx, f)
			if b.activeChannelID == "" {
				continue
			}
			switch f.Phase {
			case kernel.PhaseConnecting, kernel.PhaseStreaming, kernel.PhaseFinalizing, kernel.PhaseReconnecting:
				if c == nil {
					typingCtx, stop := context.WithCancel(ctx)
					cancelTyping = stop
					go runTypingLoop(typingCtx, b.client, b.activeChannelID)
					c = newCoalescer(b.client, b.activeChannelID, time.Duration(b.cfg.CoalesceMs)*time.Millisecond)
					go c.run(ctx)
				}
				c.submit(formatStream(f))
			case kernel.PhaseIdle:
				if c != nil {
					c.flushImmediate(formatFinal(f))
					c = nil
				} else {
					_, _ = b.client.Send(b.activeChannelID, formatFinal(f))
				}
				if cancelTyping != nil {
					cancelTyping()
					cancelTyping = nil
				}
			case kernel.PhaseFailed, kernel.PhaseCancelling:
				text := formatError(f)
				if c != nil {
					c.flushImmediate(text)
					c = nil
				} else {
					_, _ = b.client.Send(b.activeChannelID, text)
				}
				if cancelTyping != nil {
					cancelTyping()
					cancelTyping = nil
				}
			}
		}
	}
}

func (b *Bot) persistIfChanged(ctx context.Context, f kernel.RenderFrame) {
	if b.cfg.SessionMap == nil || b.activeChannelID == "" || f.SessionID == "" || f.SessionID == b.lastSID {
		return
	}
	if err := b.cfg.SessionMap.Put(ctx, SessionKey(b.activeChannelID), f.SessionID); err != nil {
		b.log.Warn("discord: failed to persist session_id", "channel_id", b.activeChannelID, "err", err)
		return
	}
	b.lastSID = f.SessionID
}

type realClient struct {
	session *discordgo.Session
	selfID  string
}

func NewRealClient(token string) (Client, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	u, err := s.User("@me")
	if err != nil {
		return nil, err
	}
	return &realClient{session: s, selfID: u.ID}, nil
}

func (c *realClient) Open() error  { return c.session.Open() }
func (c *realClient) Close() error { return c.session.Close() }
func (c *realClient) SelfID() string { return c.selfID }
func (c *realClient) SetMessageHandler(fn func(InboundMessage)) {
	c.session.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author == nil || m.Author.Bot {
			return
		}
		fn(InboundMessage{
			ID:           m.ID,
			ChannelID:    m.ChannelID,
			GuildID:      m.GuildID,
			AuthorID:     m.Author.ID,
			Content:      m.Content,
			IsDM:         m.GuildID == "",
			MentionedBot: mentioned(m.Mentions, c.selfID),
		})
	})
}
func (c *realClient) Send(channelID, text string) (string, error) {
	msg, err := c.session.ChannelMessageSend(channelID, text)
	if err != nil {
		return "", err
	}
	return msg.ID, nil
}
func (c *realClient) Edit(channelID, messageID, text string) error {
	_, err := c.session.ChannelMessageEdit(channelID, messageID, text)
	return err
}
func (c *realClient) Typing(channelID string) error {
	return c.session.ChannelTyping(channelID)
}
```

`render.go` should keep the same semantics as Telegram but use plain text rather than MarkdownV2 escaping:

```go
func stripSelfMention(text, selfID string) string {
	text = strings.ReplaceAll(text, "<@"+selfID+">", "")
	text = strings.ReplaceAll(text, "<@!"+selfID+">", "")
	return text
}

func mentioned(users []*discordgo.User, selfID string) bool {
	for _, u := range users {
		if u != nil && u.ID == selfID {
			return true
		}
	}
	return false
}

func truncateDiscord(s string) string {
	const maxDiscordText = 2000
	runes := []rune(s)
	if len(runes) <= maxDiscordText {
		return s
	}
	return string(runes[:maxDiscordText-1]) + "…"
}

func formatStream(f kernel.RenderFrame) string {
	text := truncateDiscord(f.DraftText)
	if len(f.SoulEvents) > 0 {
		last := f.SoulEvents[len(f.SoulEvents)-1]
		if last.Text != "" && last.Text != "idle" {
			text += "\n\n[tool] " + last.Text
		}
	}
	if f.Phase == kernel.PhaseReconnecting {
		text += "\n\nreconnecting..."
	}
	return text
}

func formatFinal(f kernel.RenderFrame) string {
	for i := len(f.History) - 1; i >= 0; i-- {
		if f.History[i].Role == "assistant" {
			return truncateDiscord(f.History[i].Content)
		}
	}
	return "(empty reply)"
}

func formatError(f kernel.RenderFrame) string {
	if f.LastError == "" {
		return "cancelled"
	}
	return truncateDiscord("error: " + f.LastError)
}
```

`coalesce.go` should mirror Telegram's send/edit window:

```go
type coalescer struct {
	client    Client
	channelID string
	window    time.Duration
	msgID     string
	in        chan string
}

func newCoalescer(client Client, channelID string, window time.Duration) *coalescer {
	return &coalescer{
		client:    client,
		channelID: channelID,
		window:    window,
		in:        make(chan string, 1),
	}
}

func (c *coalescer) submit(text string) {
	select {
	case c.in <- text:
	default:
		<-c.in
		c.in <- text
	}
}

func (c *coalescer) run(ctx context.Context) {
	ticker := time.NewTicker(c.window)
	defer ticker.Stop()

	var latest string
	for {
		select {
		case <-ctx.Done():
			return
		case latest = <-c.in:
		case <-ticker.C:
			if latest == "" {
				continue
			}
			if c.msgID == "" {
				id, err := c.client.Send(c.channelID, latest)
				if err == nil {
					c.msgID = id
				}
			} else {
				_ = c.client.Edit(c.channelID, c.msgID, latest)
			}
		}
	}
}

func (c *coalescer) flushImmediate(text string) {
	if c.msgID == "" {
		_, _ = c.client.Send(c.channelID, text)
		return
	}
	_ = c.client.Edit(c.channelID, c.msgID, text)
}

func runTypingLoop(ctx context.Context, client Client, channelID string) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	_ = client.Typing(channelID)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = client.Typing(channelID)
		}
	}
}
```

- [ ] **Step 5: Run the Discord tests**

```bash
cd gormes
go test ./internal/discord -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd ..
git add gormes/internal/discord
git commit -m "feat(gormes): add discord adapter package"
```

## Task 4: Wire `gormes discord` and register it on the root command

**Files:**
- Create: `gormes/cmd/gormes/discord.go`
- Create: `gormes/cmd/gormes/discord_test.go`
- Modify: `gormes/cmd/gormes/main.go`

**Reference anchors:**
- `gormes/cmd/gormes/main.go`
- `gormes/cmd/gormes/telegram.go`
- `gormes/internal/discord/*`

- [ ] **Step 1: Write the failing command tests**

Create `gormes/cmd/gormes/discord_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

func TestValidateDiscordConfig_RejectsMissingToken(t *testing.T) {
	err := validateDiscordConfig(config.Config{})
	if err == nil || !strings.Contains(err.Error(), "no Discord bot token") {
		t.Fatalf("err = %v, want missing token error", err)
	}
}

func TestValidateDiscordConfig_RejectsGuildWithoutChannel(t *testing.T) {
	cfg := config.Config{}
	cfg.Discord.BotToken = "discord-token"
	cfg.Discord.AllowedGuildID = "guild-1"
	err := validateDiscordConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "allowed_channel_id") {
		t.Fatalf("err = %v, want allowed_channel_id validation error", err)
	}
}

func TestNewRootCmd_RegistersDiscord(t *testing.T) {
	root := newRootCmd()
	if root.Commands()[0] == nil {
		t.Fatal("root has no subcommands")
	}
	names := make(map[string]bool)
	for _, cmd := range root.Commands() {
		names[cmd.Name()] = true
	}
	if !names["discord"] {
		t.Fatal("root command missing discord subcommand")
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
cd gormes
go test ./cmd/gormes -run 'TestValidateDiscordConfig_|TestNewRootCmd_' -count=1
```

Expected: FAIL with `undefined: validateDiscordConfig`, `undefined: newRootCmd`, or missing Discord subcommand registration.

- [ ] **Step 3: Implement `discord.go` and `newRootCmd()`**

Create `gormes/cmd/gormes/discord.go`:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	discordadapter "github.com/TrebuchetDynamics/gormes-agent/gormes/internal/discord"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

var discordCmd = &cobra.Command{
	Use:          "discord",
	Short:        "Run Gormes as a Discord adapter",
	SilenceUsage: true,
	RunE:         runDiscord,
}

func validateDiscordConfig(cfg config.Config) error {
	if cfg.Discord.BotToken == "" {
		return fmt.Errorf("no Discord bot token — set GORMES_DISCORD_TOKEN env or [discord].bot_token in config.toml")
	}
	if cfg.Discord.AllowedGuildID != "" && cfg.Discord.AllowedChannelID == "" {
		return fmt.Errorf("discord guild mode requires [discord].allowed_channel_id")
	}
	return nil
}

func runDiscord(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := validateDiscordConfig(cfg); err != nil {
		return err
	}

	chatKey := ""
	if cfg.Discord.AllowedChannelID != "" {
		chatKey = discordadapter.SessionKey(cfg.Discord.AllowedChannelID)
	}

	rt, err := openGatewayRuntime(cfg, gatewayRuntimeOptions{
		ChatKey:        chatKey,
		ResumeOverride: cfg.Resume,
		RecallEnabled:  chatKey != "",
	}, slog.Default())
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
		defer cancel()
		rt.Close(shutdownCtx)
	}()

	client, err := discordadapter.NewRealClient(cfg.Discord.BotToken)
	if err != nil {
		return err
	}

	bot := discordadapter.New(discordadapter.Config{
		AllowedGuildID:   cfg.Discord.AllowedGuildID,
		AllowedChannelID: cfg.Discord.AllowedChannelID,
		MentionRequired:  cfg.Discord.MentionRequired,
		CoalesceMs:       cfg.Discord.CoalesceMs,
		SessionMap:       rt.SessionMap,
	}, client, rt.Kernel, slog.Default())

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	rt.Start(rootCtx)
	return bot.Run(rootCtx)
}
```

Refactor `gormes/cmd/gormes/main.go` to expose a reusable builder:

```go
func main() {
	defer func() {
		if r := recover(); r != nil {
			dumpCrash(r)
			os.Exit(2)
		}
	}()

	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "gormes",
		Short:        "Go frontend for Hermes Agent",
		SilenceUsage: true,
		RunE:         runTUI,
	}
	root.Flags().Bool("offline", false, "skip startup api_server health check (dev only — turns the TUI into a cosmetic smoke-tester)")
	root.Flags().String("resume", "", "override persisted session_id for the TUI's default key")
	root.AddCommand(doctorCmd, versionCmd, telegramCmd, discordCmd)
	return root
}
```

- [ ] **Step 4: Run the command tests and a build**

```bash
cd gormes
go test ./cmd/gormes -run 'TestValidateDiscordConfig_|TestNewRootCmd_' -count=1
go build ./cmd/gormes
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd ..
git add gormes/cmd/gormes/main.go gormes/cmd/gormes/discord.go gormes/cmd/gormes/discord_test.go
git commit -m "feat(gormes): wire discord command"
```

## Task 5: Build the Slack adapter package with Socket Mode ACK + thread-aware replies

**Files:**
- Create: `gormes/internal/slack/client.go`
- Create: `gormes/internal/slack/real_client.go`
- Create: `gormes/internal/slack/render.go`
- Create: `gormes/internal/slack/bot.go`
- Create: `gormes/internal/slack/mock_test.go`
- Create: `gormes/internal/slack/bot_test.go`

**Reference donors:**
- `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw/pkg/channels/slack/slack.go`
- `gormes/internal/telegram/bot.go`

- [ ] **Step 1: Write the failing Slack tests**

Create `gormes/internal/slack/bot_test.go`:

```go
package slack

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

func newSlackKernel(reply, sid string) *kernel.Kernel {
	hc := hermes.NewMockClient()
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: len(reply)})
	hc.Script(events, sid)
	return kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hc, store.NewNoop(), telemetry.New(), nil)
}

func TestBot_AcksEventsBeforeHandling(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("roger", "sess-slack")
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-1",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello",
		Timestamp: "1711111111.000100",
		ThreadTS:  "1711111111.000100",
	})

	time.Sleep(100 * time.Millisecond)
	if !mc.wasAcked("req-1") {
		t.Fatal("expected request req-1 to be acked")
	}
}

func TestBot_UsesThreadTSForReplies(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("thread ok", "sess-thread")
	smap := session.NewMemMap()
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       50,
		SessionMap:       smap,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-2",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello thread",
		Timestamp: "1711111111.000200",
		ThreadTS:  "1711111111.000200",
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastOutputText(), "thread ok") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	if mc.lastThreadTS() != "1711111111.000200" {
		t.Fatalf("thread_ts = %q, want 1711111111.000200", mc.lastThreadTS())
	}
	gotSID, err := smap.Get(context.Background(), SessionKey("C123"))
	if err != nil {
		t.Fatalf("Get persisted session: %v", err)
	}
	if gotSID != "sess-thread" {
		t.Fatalf("persisted sid = %q, want sess-thread", gotSID)
	}
}

func TestBot_RejectsOtherChannels(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("unused", "sess-unused")
	b := New(Config{AllowedChannelID: "C123", ReplyInThread: true}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-3",
		ChannelID: "C999",
		UserID:    "U1",
		Text:      "wrong room",
		Timestamp: "1711111111.000300",
	})

	time.Sleep(100 * time.Millisecond)
	if got := len(mc.outputs()); got != 0 {
		t.Fatalf("outputs = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run the tests and confirm the package is absent**

```bash
cd gormes
go test ./internal/slack -count=1
```

Expected: FAIL with missing package or undefined symbol errors.

- [ ] **Step 3: Create the Slack client seam and mock**

Create `gormes/internal/slack/client.go`:

```go
package slack

import "context"

type Event struct {
	RequestID string
	ChannelID string
	UserID    string
	Text      string
	Timestamp string
	ThreadTS  string
}

type Client interface {
	AuthTest(context.Context) (string, error)
	Run(context.Context, func(Event)) error
	Ack(requestID string)
	PostMessage(ctx context.Context, channelID, threadTS, text string) (string, error)
	UpdateMessage(ctx context.Context, channelID, ts, text string) error
}

func SessionKey(channelID string) string {
	return "slack:" + channelID
}
```

Create `gormes/internal/slack/mock_test.go`:

```go
package slack

import (
	"context"
	"sync"
)

type output struct {
	channelID string
	threadTS  string
	text      string
}

type mockClient struct {
	mu     sync.Mutex
	acks   map[string]bool
	stream chan Event
	out    []output
}

func newMockClient() *mockClient {
	return &mockClient{
		acks:   make(map[string]bool),
		stream: make(chan Event, 16),
	}
}

func (m *mockClient) AuthTest(context.Context) (string, error) { return "UBOT", nil }
func (m *mockClient) Ack(requestID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acks[requestID] = true
}
func (m *mockClient) Run(ctx context.Context, fn func(Event)) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-m.stream:
			fn(ev)
		}
	}
}
func (m *mockClient) PostMessage(_ context.Context, channelID, threadTS, text string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.out = append(m.out, output{channelID: channelID, threadTS: threadTS, text: text})
	return "1711111111.999999", nil
}
func (m *mockClient) UpdateMessage(_ context.Context, channelID, ts, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.out = append(m.out, output{channelID: channelID, threadTS: ts, text: text})
	return nil
}
func (m *mockClient) pushEvent(ev Event) { m.stream <- ev }
func (m *mockClient) wasAcked(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.acks[id]
}
func (m *mockClient) outputs() []output {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]output, len(m.out))
	copy(out, m.out)
	return out
}
func (m *mockClient) lastOutputText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.out) == 0 {
		return ""
	}
	return m.out[len(m.out)-1].text
}
func (m *mockClient) lastThreadTS() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.out) == 0 {
		return ""
	}
	return m.out[len(m.out)-1].threadTS
}
```

- [ ] **Step 4: Implement the Slack adapter**

Create `gormes/internal/slack/bot.go`, `render.go`, and `real_client.go` with this structure:

```go
package slack

import (
	"context"
	"log/slog"
	"strings"
	"time"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
)

type Config struct {
	AllowedChannelID string
	ReplyInThread    bool
	CoalesceMs       int
	SessionMap       session.Map
}

type Bot struct {
	cfg            Config
	client         Client
	kernel         *kernel.Kernel
	log            *slog.Logger
	activeChannel  string
	activeThreadTS string
	placeholderTS  string
	lastSID        string
}

func New(cfg Config, client Client, k *kernel.Kernel, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	if cfg.CoalesceMs <= 0 {
		cfg.CoalesceMs = 1000
	}
	return &Bot{cfg: cfg, client: client, kernel: k, log: log}
}

func (b *Bot) Run(ctx context.Context) error {
	if _, err := b.client.AuthTest(ctx); err != nil {
		return err
	}
	go b.runOutbound(ctx)
	return b.client.Run(ctx, func(ev Event) { b.handleEvent(ctx, ev) })
}

func (b *Bot) handleEvent(ctx context.Context, ev Event) {
	b.client.Ack(ev.RequestID)
	if ev.ChannelID != b.cfg.AllowedChannelID {
		return
	}

	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return
	}

	b.activeChannel = ev.ChannelID
	if b.cfg.ReplyInThread {
		if ev.ThreadTS != "" {
			b.activeThreadTS = ev.ThreadTS
		} else {
			b.activeThreadTS = ev.Timestamp
		}
	}

	switch text {
	case "/start":
		_, _ = b.client.PostMessage(ctx, ev.ChannelID, b.activeThreadTS, "Gormes is online. Commands: /stop /new")
	case "/stop":
		_ = b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel})
	case "/new":
		if err := b.kernel.ResetSession(); err != nil {
			_, _ = b.client.PostMessage(ctx, ev.ChannelID, b.activeThreadTS, "Cannot reset during active turn — send /stop first.")
			return
		}
		_, _ = b.client.PostMessage(ctx, ev.ChannelID, b.activeThreadTS, "Session reset. Next message starts fresh.")
	default:
		if err := b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: text}); err != nil {
			_, _ = b.client.PostMessage(ctx, ev.ChannelID, b.activeThreadTS, "Busy — try again in a second.")
		}
	}
}

func (b *Bot) runOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case f, ok := <-b.kernel.Render():
			if !ok {
				return
			}
			b.persistIfChanged(ctx, f)
			if b.activeChannel == "" {
				continue
			}
			switch f.Phase {
			case kernel.PhaseConnecting, kernel.PhaseStreaming, kernel.PhaseFinalizing, kernel.PhaseReconnecting:
				if b.placeholderTS == "" {
					ts, err := b.client.PostMessage(ctx, b.activeChannel, b.activeThreadTS, formatPending())
					if err == nil {
						b.placeholderTS = ts
					}
					continue
				}
				_ = b.client.UpdateMessage(ctx, b.activeChannel, b.placeholderTS, formatStream(f))
			case kernel.PhaseIdle:
				if b.placeholderTS != "" {
					_ = b.client.UpdateMessage(ctx, b.activeChannel, b.placeholderTS, formatFinal(f))
					b.placeholderTS = ""
				} else {
					_, _ = b.client.PostMessage(ctx, b.activeChannel, b.activeThreadTS, formatFinal(f))
				}
			case kernel.PhaseFailed, kernel.PhaseCancelling:
				text := formatError(f)
				if b.placeholderTS != "" {
					_ = b.client.UpdateMessage(ctx, b.activeChannel, b.placeholderTS, text)
					b.placeholderTS = ""
				} else {
					_, _ = b.client.PostMessage(ctx, b.activeChannel, b.activeThreadTS, text)
				}
			}
		}
	}
}

func (b *Bot) persistIfChanged(ctx context.Context, f kernel.RenderFrame) {
	if b.cfg.SessionMap == nil || b.activeChannel == "" || f.SessionID == "" || f.SessionID == b.lastSID {
		return
	}
	if err := b.cfg.SessionMap.Put(ctx, SessionKey(b.activeChannel), f.SessionID); err != nil {
		b.log.Warn("slack: failed to persist session_id", "channel_id", b.activeChannel, "err", err)
		return
	}
	b.lastSID = f.SessionID
}

type realClient struct {
	api    *slackapi.Client
	socket *socketmode.Client
}

func NewRealClient(botToken, appToken string) Client {
	api := slackapi.New(botToken, slackapi.OptionAppLevelToken(appToken))
	return &realClient{
		api:    api,
		socket: socketmode.New(api),
	}
}

func (c *realClient) AuthTest(ctx context.Context) (string, error) {
	resp, err := c.api.AuthTestContext(ctx)
	if err != nil {
		return "", err
	}
	return resp.UserID, nil
}

func (c *realClient) Ack(requestID string) {
	_ = requestID
}

func (c *realClient) Run(ctx context.Context, fn func(Event)) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-c.socket.Events:
				if !ok {
					return
				}
				if ev.Type != socketmode.EventTypeEventsAPI || ev.Request == nil {
					continue
				}
				c.socket.Ack(*ev.Request)
				apiEv, ok := ev.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				msg, ok := apiEv.InnerEvent.Data.(*slackevents.MessageEvent)
				if !ok || msg.User == "" {
					continue
				}
				fn(Event{
					RequestID: ev.Request.EnvelopeID,
					ChannelID: msg.Channel,
					UserID:    msg.User,
					Text:      msg.Text,
					Timestamp: msg.TimeStamp,
					ThreadTS:  msg.ThreadTimeStamp,
				})
			}
		}
	}()
	return c.socket.RunContext(ctx)
}

func (c *realClient) PostMessage(ctx context.Context, channelID, threadTS, text string) (string, error) {
	opts := []slackapi.MsgOption{slackapi.MsgOptionText(text, false)}
	if threadTS != "" {
		opts = append(opts, slackapi.MsgOptionTS(threadTS))
	}
	_, ts, err := c.api.PostMessageContext(ctx, channelID, opts...)
	return ts, err
}

func (c *realClient) UpdateMessage(ctx context.Context, channelID, ts, text string) error {
	_, _, _, err := c.api.UpdateMessageContext(ctx, channelID, ts, slackapi.MsgOptionText(text, false))
	return err
}
```

Keep `render.go` deliberately boring but complete:

```go
func formatPending() string { return "thinking..." }

func formatStream(f kernel.RenderFrame) string {
	text := f.DraftText
	if text == "" {
		text = "thinking..."
	}
	return text
}

func formatFinal(f kernel.RenderFrame) string {
	for i := len(f.History) - 1; i >= 0; i-- {
		if f.History[i].Role == "assistant" {
			return f.History[i].Content
		}
	}
	return "(empty reply)"
}

func formatError(f kernel.RenderFrame) string {
	if f.LastError == "" {
		return "cancelled"
	}
	return "error: " + f.LastError
}
```

- [ ] **Step 5: Run the Slack tests**

```bash
cd gormes
go test ./internal/slack -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd ..
git add gormes/internal/slack
git commit -m "feat(gormes): add slack adapter package"
```

## Task 6: Wire `gormes slack` and finish root command registration

**Files:**
- Create: `gormes/cmd/gormes/slack.go`
- Create: `gormes/cmd/gormes/slack_test.go`
- Modify: `gormes/cmd/gormes/main.go`

**Reference anchors:**
- `gormes/cmd/gormes/discord.go`
- `gormes/internal/slack/*`

- [ ] **Step 1: Write the failing Slack command tests**

Create `gormes/cmd/gormes/slack_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

func TestValidateSlackConfig_RejectsMissingTokens(t *testing.T) {
	err := validateSlackConfig(config.Config{})
	if err == nil || !strings.Contains(err.Error(), "Slack bot token") {
		t.Fatalf("err = %v, want missing token error", err)
	}
}

func TestValidateSlackConfig_RejectsMissingChannel(t *testing.T) {
	cfg := config.Config{}
	cfg.Slack.BotToken = "xoxb-test"
	cfg.Slack.AppToken = "xapp-test"
	err := validateSlackConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "allowed_channel_id") {
		t.Fatalf("err = %v, want missing allowed_channel_id error", err)
	}
}

func TestNewRootCmd_RegistersSlack(t *testing.T) {
	root := newRootCmd()
	names := make(map[string]bool)
	for _, cmd := range root.Commands() {
		names[cmd.Name()] = true
	}
	if !names["slack"] {
		t.Fatal("root command missing slack subcommand")
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
cd gormes
go test ./cmd/gormes -run 'TestValidateSlackConfig_|TestNewRootCmd_' -count=1
```

Expected: FAIL with `undefined: validateSlackConfig` or missing Slack subcommand registration.

- [ ] **Step 3: Implement `slack.go`**

Create `gormes/cmd/gormes/slack.go`, then append Slack registration in `newRootCmd()`:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	slackadapter "github.com/TrebuchetDynamics/gormes-agent/gormes/internal/slack"
)

var slackCmd = &cobra.Command{
	Use:          "slack",
	Short:        "Run Gormes as a Slack Socket Mode adapter",
	SilenceUsage: true,
	RunE:         runSlack,
}

func validateSlackConfig(cfg config.Config) error {
	if cfg.Slack.BotToken == "" {
		return fmt.Errorf("no Slack bot token — set GORMES_SLACK_BOT_TOKEN env or [slack].bot_token in config.toml")
	}
	if cfg.Slack.AppToken == "" {
		return fmt.Errorf("no Slack app token — set GORMES_SLACK_APP_TOKEN env or [slack].app_token in config.toml")
	}
	if cfg.Slack.AllowedChannelID == "" {
		return fmt.Errorf("slack requires [slack].allowed_channel_id for Phase 2.B.3")
	}
	return nil
}

func runSlack(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := validateSlackConfig(cfg); err != nil {
		return err
	}

	key := slackadapter.SessionKey(cfg.Slack.AllowedChannelID)
	rt, err := openGatewayRuntime(cfg, gatewayRuntimeOptions{
		ChatKey:        key,
		ResumeOverride: cfg.Resume,
		RecallEnabled:  true,
	}, slog.Default())
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
		defer cancel()
		rt.Close(shutdownCtx)
	}()

	client := slackadapter.NewRealClient(cfg.Slack.BotToken, cfg.Slack.AppToken)
	bot := slackadapter.New(slackadapter.Config{
		AllowedChannelID: cfg.Slack.AllowedChannelID,
		ReplyInThread:    cfg.Slack.ReplyInThread,
		CoalesceMs:       cfg.Slack.CoalesceMs,
		SessionMap:       rt.SessionMap,
	}, client, rt.Kernel, slog.Default())

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	rt.Start(rootCtx)
	return bot.Run(rootCtx)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "gormes",
		Short:        "Go frontend for Hermes Agent",
		SilenceUsage: true,
		RunE:         runTUI,
	}
	root.Flags().Bool("offline", false, "skip startup api_server health check (dev only — turns the TUI into a cosmetic smoke-tester)")
	root.Flags().String("resume", "", "override persisted session_id for the TUI's default key")
	root.AddCommand(doctorCmd, versionCmd, telegramCmd, discordCmd, slackCmd)
	return root
}
```

- [ ] **Step 4: Run the Slack command tests and build**

```bash
cd gormes
go test ./cmd/gormes -run 'TestValidateSlackConfig_|TestNewRootCmd_' -count=1
go build ./cmd/gormes
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd ..
git add gormes/cmd/gormes/slack.go gormes/cmd/gormes/slack_test.go gormes/cmd/gormes/main.go
git commit -m "feat(gormes): wire slack command"
```

## Task 7: Cleanup, isolation guards, docs, and full verification

**Files:**
- Modify: `gormes/internal/buildisolation_test.go`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`
- Modify: `gormes/docs/content/building-gormes/gateway-donor-map/shared-adapter-patterns.md`

**Reference anchors:**
- `gormes/internal/buildisolation_test.go`
- `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`
- `gormes/docs/content/building-gormes/gateway-donor-map/shared-adapter-patterns.md`

- [ ] **Step 1: Add the isolation guard before touching docs**

Append this test to `gormes/internal/buildisolation_test.go`:

```go
func TestKernelHasNoMessagingSDKDeps(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./internal/kernel")
	cmd.Dir = ".."
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out.String())
	}

	for _, d := range strings.Split(out.String(), "\n") {
		if strings.Contains(d, "bwmarrin/discordgo") ||
			strings.Contains(d, "slack-go/slack") {
			t.Errorf("internal/kernel transitively depends on %q — messaging SDK leaked into kernel", d)
		}
	}
}
```

- [ ] **Step 2: Update the Phase 2 docs to point at the landed code**

Make these doc edits:

In `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`, replace the wider gateway row with:

```md
| Phase 2.B.2+ — Wider Gateway Surface | ✅ shipped | P1 | `gormes telegram`, `gormes discord`, and `gormes slack` now share the same runtime boot path in `cmd/gormes/gateway_runtime.go`; Discord reused PicoClaw mention + typing patterns and Slack reused Socket Mode ACK + thread reply patterns without importing PicoClaw's manager runtime. |
```

In `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`, replace the Discord and Slack rows with:

```md
| Discord | `gateway/platforms/discord.py` | 2.B.2 | ✅ shipped | `gormes/internal/discord/*`, `gormes/cmd/gormes/discord.go` |
| Slack | `gateway/platforms/slack.py` | 2.B.3 | ✅ shipped | `gormes/internal/slack/*`, `gormes/cmd/gormes/slack.go` |
```

In `gormes/docs/content/building-gormes/gateway-donor-map/shared-adapter-patterns.md`, add a short implementation map:

```md
## Landed Gormes Mappings

- PicoClaw Discord startup, mention-gate, and typing references:
  `picoclaw/pkg/channels/discord/discord.go`
  → `gormes/internal/discord/real_client.go`
  → `gormes/internal/discord/bot.go`
  → `gormes/cmd/gormes/discord.go`

- PicoClaw Slack Socket Mode, ACK, and thread target references:
  `picoclaw/pkg/channels/slack/slack.go`
  → `gormes/internal/slack/real_client.go`
  → `gormes/internal/slack/bot.go`
  → `gormes/cmd/gormes/slack.go`

- Gormes-only shared runtime extraction:
  `gormes/cmd/gormes/telegram.go`
  → `gormes/cmd/gormes/gateway_runtime.go`
```

- [ ] **Step 3: Run the full verification matrix**

```bash
cd gormes
go test ./internal/config ./internal/discord ./internal/slack ./cmd/gormes ./docs -count=1
go test ./internal ./cmd/gormes ./docs -count=1
go build ./cmd/gormes
```

Expected: all PASS. `go test ./internal` should include the new isolation guard and ensure Discord/Slack did not leak into `internal/kernel`.

- [ ] **Step 4: Commit**

```bash
cd ..
git add gormes/internal/buildisolation_test.go \
        gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md \
        gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md \
        gormes/docs/content/building-gormes/gateway-donor-map/shared-adapter-patterns.md
git commit -m "docs(gormes): mark discord and slack gateway batch shipped"
```

## Notes For The Implementer

- Keep the shared extraction narrow. If a helper is not reused by at least Telegram plus one of Discord or Slack in this batch, it does not belong in `cmd/gormes/gateway_runtime.go`.
- Do not create `internal/gateway/` in this batch. That was an earlier design branch and is no longer the approved direction.
- Discord DM support is intentionally bounded by the current single-kernel model. `AllowedChannelID == ""` means "accept one operative DM channel per process" rather than "fan out across arbitrary DM conversations."
- Slack thread awareness is reply routing only. The session key remains `slack:<channel_id>` in this batch; per-thread kernel isolation is a future chassis problem, not a hidden requirement here.
- Keep PicoClaw as donor material for edge behavior only. Copying its transport tactics is fine; copying its runtime ownership is not.
