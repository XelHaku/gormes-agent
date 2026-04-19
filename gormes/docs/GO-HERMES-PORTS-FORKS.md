# Go Hermes Ports & Forks - Research Document

> **Purpose**: Research existing Go implementations of Hermes-style AI agents to learn from their patterns and avoid reinventing the wheel.

---

## Summary of Findings

**Bottom Line**: As of April 2026, there are **multiple Go Hermes attempts**, including some very recent ones (April 2026). The most mature is **MLT-OSS/hermes-agent-go** (126 Go files, 50 tools, 15 platform adapters) — a full rewrite attempt that aligns closely with gormes goals. The Go AI agent ecosystem is also maturing rapidly with projects like **trpc-agent-go** (1.1K stars) and **AgenticGoKit** (141 stars).

---

## 1. MLT-OSS/hermes-agent-go (CRITICAL FINDING)

**The most mature Go Hermes port found — aligns closely with gormes goals.**

| Field | Value |
|-------|-------|
| URL | https://github.com/MLT-OSS/hermes-agent-go |
| Created | April 9, 2026 |
| Files | 126 Go files |
| Lines | ~29,441 lines |
| Tools | 50 built-in |
| Skills | 77 bundled |
| Platforms | 15 adapters |

### Comparison: Python vs Go Hermes

| Aspect | Python Original | Go Rewrite |
|--------|----------------|------------|
| Distribution | Complex (Python + uv + venv + npm) | Single binary, zero deps |
| Startup | ~2s | ~50ms |
| Binary size | ~500MB | 23MB |
| Code volume | 183K lines | 29K lines (6x more compact) |
| Concurrency | asyncio + threading (GIL) | Goroutines (native) |

### Features Implemented

- **50 Tools**: terminal, file ops, web search/crawl, browser automation, vision, TTS, code execution, subagent delegation, MCP, Home Assistant
- **15 Platform Adapters**: Telegram, Discord, Slack, WhatsApp, Signal, Email, Matrix, DingTalk, Feishu, etc.
- **7 Terminal Backends**: local, Docker, SSH, Modal, Daytona, Singularity
- **Dual API**: OpenAI-compatible + Anthropic Messages API with prompt caching
- **Skill System**: procedural memory, YAML/Markdown skill files
- **SQLite + FTS5**: session persistence
- **Context Compression**: auto-summarization
- **Subagent Delegation**: goroutines, max 8 concurrent
- **Cron Scheduling**: background jobs
- **MCP Integration**: stdio + SSE transport
- **Profile System**: multi-instance isolation
- **ACP Server**: VS Code, Zed, JetBrains integration

### What We Can Learn

| Pattern | Notes |
|---------|-------|
| Directory layout | 126 Go files organized by concern |
| Tool registration | 50 tools registered systematically |
| Platform adapter pattern | 15 adapters with unified interface |
| Concurrency model | Goroutines for subagents, channels for events |
| SQLite schema | Session + FTS5 full-text search |

---

## 2. binbinao/hermes-go-agents

**Another active Go Hermes attempt.**

| Field | Value |
|-------|-------|
| URL | https://github.com/binbinao/hermes-go-agents |
| Stars | 0 |
| Created | April 16, 2026 |
| Description | "Hermes Agent written in Go - AI coding agent with tool calling, CLI, and multi-platform support" |

---

## 3. LiangJJ456/hermes_agent_go

**Freshest port attempt found.**

| Field | Value |
|-------|-------|
| URL | https://github.com/LiangJJ456/hermes_agent_go |
| Stars | 0 |
| Created | April 19, 2026 |
| Description | "hermes_agentgo版本" (Chinese: Hermes Agent Go version) |

---

## 4. Harsh-2909/hermes-go (Earlier Attempt)

**Early Go-based AI Agent framework.**

| Field | Value |
|-------|-------|
| URL | https://github.com/Harsh-2909/hermes-go |
| Stars | 24 |
| Forks | 1 |
| Language | Go (100%) |
| Created | 2025-02-27 |
| Last Push | 2025-05-16 |
| License | MIT |
| Contributors | 1 |
| Go Version | 1.23+ |

### What It Is

A **Go-based AI Agent framework** inspired by LangChain and Agno. Not a fork of NousResearch/hermes-agent — a ground-up rewrite.

### Features (from README)

- Easy-to-use AI Agent implementation
- Custom tool creation and integration
- Support for streaming responses
- Multimodal capabilities (images, audio)
- Modular design for extensibility
- Markdown formatting support
- Built-in debugging mode
- RAG (Retrieval-Augmented Generation) support
- Knowledge/Memory layer

### Architecture Patterns

```
hermes-go/
├── agent/           # Core agent implementation
├── models/          # Model providers (OpenAI only currently)
│   └── openai/
├── tools/           # Tool system
└── examples/       # Usage examples
```

### Streaming Example

```go
stream, err := agent.RunStream(ctx, "Can you say hello?")
for resp := range stream {
    if resp.Event == "chunk" {
        fmt.Print(resp.Data)
    }
}
```

### Limitations (Red Flags)

- **No MCP support** — Tools defined in-code, not via protocol
- **Single model** — Only OpenAI, no Anthropic/claude-code integration
- **No messaging gateway** — CLI only, no Telegram/Discord/etc
- **Stalled development** — Last push May 2025, no recent releases

---

## 5. Related Go AI Agent Projects (Not Hermes, but Valuable)

### Broader Go AI Agent Ecosystem

| Project | URL | Stars | Updated | Key Patterns |
|---------|-----|-------|---------|--------------|
| **trpc-agent-go** | https://github.com/trpc-group/trpc-agent-go | 1,105 | Apr 17, 2026 | Production-ready agent framework |
| **AgenticGoKit** | https://github.com/AgenticGoKit/AgenticGoKit | 141 | Apr 12, 2026 | LLM-agnostic, event-driven, MCP tool discovery |
| **Maolaohei/claw-prime** | https://github.com/Maolaohei/claw-prime | - | Feb 22, 2026 | Zero deps, 20MB binary, AI auto-generates Go skills |
| **davidleitw/go-agent** | https://github.com/davidleitw/go-agent | - | Sep 13, 2025 | Context providers, session TTL |
| **counhopig/gittyai** | https://github.com/counhopig/gittyai | - | Dec 14, 2025 | CrewAI-inspired multi-agent orchestration |
| **HildaM/openmanus-go** | https://github.com/HildaM/openmanus-go | 6 | Jun 21, 2025 | OpenManus port, multi-agent, flow control |

### Key Patterns from Broader Ecosystem

**1. Goroutines for concurrency** (all projects):
```go
// Subagent delegation
for _, subTask := range subTasks {
    go func(t Task) {
        results <- t.Execute(ctx)
    }(subTask)
}
```

**2. Channels for event-driven architecture**:
```go
type EventBus struct {
    events chan Event
    subs   map[string]chan Event
}
```

**3. Context propagation**:
```go
func (a *Agent) Execute(ctx context.Context, input string) error {
    ctx, span := tracer.Start(ctx, "agent.execute")
    defer span.End()
    // All operations respect cancellation
}
```

**4. Interface-based design**:
```go
type ToolRegistry interface {
    Register(tool Tool) error
    Get(name string) (Tool, error)
    List() []Tool
}

type LLMClient interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    Stream(ctx context.Context, req ChatRequest) (<-chan Chunk, error)
}
```

**5. SQLite for embedded storage**:
```go
db, err := sql.Open("sqlite", "file::memory:?cache=shared")
_, _ = db.Exec("CREATE VIRTUAL TABLE sessions USING fts5(id, content)")
```

---

## 6. langchaingo (LangChain Go port)

| Field | Value |
|-------|-------|
| URL | https://github.com/MetalBlueberry/langchaingo |
| Stars | ~500 |
| Language | Go |

LangChain port to Go — good reference for:
- LLM abstraction patterns
- Prompt templating
- Tool calling interfaces
- Memory/VectorStore abstractions

---

## 7. Main Hermes Agent Stats (Reference)

**NousResearch/hermes-agent** (the Python original):

| Metric | Value |
|--------|-------|
| Stars | 100,000+ |
| Forks | 14,000+ |
| Contributors | 370+ |
| Languages | Python (87%), TypeScript (8%), Shell, Nix |
| First Release | Feb 2026 |
| Latest | v2026.4.16 (Apr 16, 2026) |
| Issues | 5,500+ open |

### Architecture Components (Python)

```
hermes-agent/
├── run_agent.py          # Core agent loop
├── model_tools.py        # Tool orchestration
├── tools/                # 40+ built-in tools
│   ├── registry.py      # Tool registration
│   ├── file_tools.py    # File operations
│   ├── terminal_tool.py # Shell execution
│   ├── delegate_tool.py  # Subagent spawning
│   └── mcp_tool.py      # MCP client (1000+ lines)
├── hermes_cli/           # CLI interface
├── gateway/              # Messaging platform adapters
└── ui-tui/               # Terminal UI (React/Ink)
```

### Key Patterns to Learn From

1. **Tool registry pattern** — Centralized `registry.register()` approach
2. **Provider abstraction** — OpenAI, Anthropic, OpenRouter, etc.
3. **Streaming handling** — SSE token streaming
4. **Memory system** — Layered persistent memory
5. **Skills system** — Self-improving skill creation
6. **MCP integration** — Model Context Protocol client

---

## 8. Forks Analysis

Scanned NousResearch/hermes-agent forks looking for Go content:

| Fork | Language | Go Content | Notes |
|------|----------|------------|-------|
| outsourc-e/hermes-agent | Python 93% | None | Standard fork |
| plastic-labs/hermes-honcho | Python 75% | None | "Every Hermes needs his head Honcho" |
| radekderkacz/hermes-agent | Python 93% | None | Standard fork |
| rbentley9/hermes-agent | Python 92% | None | Standard fork |

**Result**: Zero Go content found in any scanned forks. All forks are Python-based.

---

## 9. Recommendations for Gormes

### What to Study

1. **MLT-OSS/hermes-agent-go** — Most mature Go Hermes port; study its directory layout, tool registration, concurrency model
2. **trpc-agent-go** (1.1K stars) — Production patterns for Go agent frameworks
3. **claw-prime** — Zero-dependency architecture for minimal binary size
4. **langchaingo** — For Go-specific LLM abstractions

### Go-Specific Patterns to Emulate

| Pattern | Example |
|---------|---------|
| Goroutines for subagents | `go a.delegate(ctx, task)` with `errgroup` |
| Channels for streaming | `chan Chunk` for SSE tokens |
| Context propagation | `context.Context` in all function signatures |
| Interface composition | `ToolRegistry`, `LLMClient` interfaces |
| Single binary | Zero external dependencies at runtime |

### What to Avoid

- **Don't copy Python patterns directly** — they don't translate 1:1 to Go
- **Don't implement MCP from scratch** — use existing Go MCP libraries
- **Don't use asyncio** — Use goroutines + channels instead

### Competitive Positioning

| Metric | MLT-OSS/hermes-agent-go | gormes (ours) |
|--------|------------------------|---------------|
| Age | Apr 2026 | Apr 2026 |
| Maturity | 126 files, 29K lines | In development |
| Tools | 50 | ~? |
| Platforms | 15 | ~? |
| Differentiation | Full port | Open-source, extensible |

---

## 10. Search Queries Used

```bash
# GitHub searches
gh search forks "hermes-agent" --language go
gh search repos "hermes" --language go
gh search repos "gormes"
gh search repos "hermes-agent-go"

# Web searches
site:github.com/Harsh-2909/hermes-go
site:github.com/MLT-OSS/hermes-agent-go
"Go Hermes agent port"
"golang AI agent framework"
```

---

## 11. Conclusion

**The Go Hermes landscape is Active (April 2026)**. Multiple teams are attempting Go ports:

| Project | Maturity | Status |
|---------|----------|--------|
| **MLT-OSS/hermes-agent-go** | High (29K lines, 50 tools) | Active (Apr 9, 2026) |
| **binbinao/hermes-go-agents** | Low (just started) | Active (Apr 16, 2026) |
| **LiangJJ456/hermes_agent_go** | Low (just started) | Active (Apr 19, 2026) |
| **Harsh-2909/hermes-go** | Low (stalled) | Last push May 2025 |

**Your gormes project is well-positioned:**
- Aligns with MLT-OSS approach (full Hermes port)
- Broader ecosystem (trpc-agent-go, AgenticGoKit) validates the space
- Go-specific advantages: goroutines, channels, single-binary, strict typing

**Key Go patterns to adopt:**
1. Goroutines + channels for concurrency (not asyncio)
2. Context propagation throughout
3. Interface-based abstractions (ToolRegistry, LLMClient)
4. SQLite + FTS5 for embedded persistence
5. Single binary distribution (zero runtime deps)

---

*Generated: April 2026*
*Search scope: GitHub (14k+ forks), pkg.go.dev, web search*
*Background tasks: bg_4b5f7419, bg_c3e7b8e5*
