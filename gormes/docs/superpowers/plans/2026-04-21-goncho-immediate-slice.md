# Goncho Immediate Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the first in-binary Goncho slice inside Gormes: contract parity docs/tests, schema for peer cards and conclusions, a minimal `internal/goncho` service, and `honcho_*` tools wired into the live-memory runtime.

**Architecture:** Keep Goncho as a thin domain layer over the existing SQLite memory store rather than building a second memory engine. The first slice should be deterministic, test-covered, and intentionally small: manual profile/conclusion management plus lexical retrieval and context assembly, with projector and advanced reasoning deferred.

**Tech Stack:** Go 1.25, SQLite via `internal/memory`, Cobra CLI, in-process tools, `go test`

---

### Task 1: Write The Approved Docs

**Files:**
- Create: `gormes/docs/superpowers/specs/2026-04-21-goncho-architecture-design.md`
- Create: `gormes/docs/superpowers/plans/2026-04-21-goncho-immediate-slice.md`

- [ ] **Step 1: Save the approved architecture decisions**

Write the spec with these concrete sections:

```md
## Final Naming Decision
- internal package: internal/goncho
- external tools stay honcho_*
- CLI stays gormes honcho ...

## Artifact Model
- workspace_id required
- peer_id required
- session_key optional
- peer_card global by peer
- representation derived on read

## Immediate Slice
1. contract parity
2. schema migration
3. minimal service
4. honcho_* tools
```

- [ ] **Step 2: Save the task plan**

Write the plan with the same task decomposition used below so implementation can proceed without re-discovery.

- [ ] **Step 3: Verify docs exist**

Run: `test -f <repo>/gormes/docs/superpowers/specs/2026-04-21-goncho-architecture-design.md && test -f <repo>/gormes/docs/superpowers/plans/2026-04-21-goncho-immediate-slice.md`

Expected: exit code `0`

### Task 2: Goncho Contract Tests First

**Files:**
- Create: `gormes/internal/goncho/types.go`
- Create: `gormes/internal/goncho/contracts_test.go`

- [ ] **Step 1: Write the failing contract tests**

Add tests that lock the external response shapes:

```go
func TestProfileResultJSONShape(t *testing.T) {
    raw, err := json.Marshal(ProfileResult{
        WorkspaceID: "default",
        Peer:        "telegram:6586915095",
        Card:        []string{"Likes exact reports"},
    })
    if err != nil {
        t.Fatal(err)
    }
    want := `{"workspace_id":"default","peer":"telegram:6586915095","card":["Likes exact reports"]}`
    if string(raw) != want {
        t.Fatalf("profile json = %s, want %s", raw, want)
    }
}
```

```go
func TestContextResultIncludesStableFields(t *testing.T) {
    raw, err := json.Marshal(ContextResult{
        WorkspaceID:    "default",
        Peer:           "telegram:6586915095",
        SessionKey:     "telegram:6586915095",
        PeerCard:       []string{"Blind", "Prefers exact outputs"},
        Representation: "The user prefers exact outputs.",
    })
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(string(raw), `"workspace_id":"default"`) {
        t.Fatalf("missing workspace_id in %s", raw)
    }
    if !strings.Contains(string(raw), `"representation":"The user prefers exact outputs."`) {
        t.Fatalf("missing representation in %s", raw)
    }
}
```

- [ ] **Step 2: Run the tests to watch them fail**

Run: `cd <repo>/gormes && go test ./internal/goncho -run Contract -v`

Expected: FAIL because `internal/goncho` does not exist yet

- [ ] **Step 3: Add the minimal types**

Create `types.go` with the public structs used by both service and tools:

```go
type ProfileResult struct {
    WorkspaceID string   `json:"workspace_id"`
    Peer        string   `json:"peer"`
    Card        []string `json:"card"`
}

type ContextResult struct {
    WorkspaceID    string         `json:"workspace_id"`
    Peer           string         `json:"peer"`
    SessionKey     string         `json:"session_key,omitempty"`
    PeerCard       []string       `json:"peer_card"`
    Representation string         `json:"representation"`
    Summary        string         `json:"summary,omitempty"`
    Conclusions    []string       `json:"conclusions,omitempty"`
    RecentMessages []MessageSlice `json:"recent_messages,omitempty"`
}
```

- [ ] **Step 4: Re-run the tests**

Run: `cd <repo>/gormes && go test ./internal/goncho -run Contract -v`

Expected: PASS

### Task 3: Schema Migration For Peer Cards And Conclusions

**Files:**
- Modify: `gormes/internal/memory/schema.go`
- Modify: `gormes/internal/memory/migrate.go`
- Modify: `gormes/internal/memory/migrate_test.go`

- [ ] **Step 1: Write the failing migration tests**

Add tests like:

```go
func TestMigrate_3eTo3f_AddsGonchoTables(t *testing.T) {
    path := filepath.Join(t.TempDir(), "memory.db")
    s, err := OpenSqlite(path, 0, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer s.Close(context.Background())

    for _, table := range []string{"goncho_peer_cards", "goncho_conclusions"} {
        var n int
        if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
            t.Fatalf("table %s missing: %v", table, err)
        }
    }
}
```

```go
func TestMigrate_3eTo3f_AddsGonchoConclusionsFTS(t *testing.T) {
    path := filepath.Join(t.TempDir(), "memory.db")
    s, err := OpenSqlite(path, 0, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer s.Close(context.Background())

    var name string
    err = s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='goncho_conclusions_fts'`).Scan(&name)
    if err != nil {
        t.Fatalf("goncho_conclusions_fts missing: %v", err)
    }
}
```

- [ ] **Step 2: Run the migration tests to verify RED**

Run: `cd <repo>/gormes && go test ./internal/memory -run 'Migrate.*Goncho|OpenSqlite_FreshDBIsV3f' -v`

Expected: FAIL because schema version and Goncho tables are not present yet

- [ ] **Step 3: Add the schema migration**

Update the memory schema to:

```sql
CREATE TABLE IF NOT EXISTS goncho_peer_cards (
    workspace_id TEXT NOT NULL,
    peer_id      TEXT NOT NULL,
    card_json    TEXT NOT NULL,
    updated_at   INTEGER NOT NULL,
    PRIMARY KEY(workspace_id, peer_id)
);

CREATE TABLE IF NOT EXISTS goncho_conclusions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id      TEXT NOT NULL,
    observer_peer_id  TEXT NOT NULL,
    peer_id           TEXT NOT NULL,
    session_key       TEXT,
    content           TEXT NOT NULL,
    kind              TEXT NOT NULL DEFAULT 'manual',
    status            TEXT NOT NULL CHECK(status IN ('pending','processed','dead_letter')),
    source            TEXT NOT NULL DEFAULT 'manual',
    idempotency_key   TEXT NOT NULL,
    evidence_json     TEXT NOT NULL DEFAULT '[]',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
```

Also add the FTS table and triggers, then bump the canonical schema version to `3f`.

- [ ] **Step 4: Re-run the migration tests**

Run: `cd <repo>/gormes && go test ./internal/memory -run 'Migrate.*Goncho|OpenSqlite_FreshDBIsV3f' -v`

Expected: PASS

### Task 4: Minimal `internal/goncho` Service

**Files:**
- Create: `gormes/internal/goncho/service.go`
- Create: `gormes/internal/goncho/sql.go`
- Create: `gormes/internal/goncho/service_test.go`

- [ ] **Step 1: Write failing service tests**

Add tests for the minimum behaviors:

```go
func TestService_ProfileRoundTrip(t *testing.T) {
    svc := newTestService(t)
    ctx := context.Background()

    if err := svc.SetProfile(ctx, "telegram:6586915095", []string{"Blind", "Prefers exact outputs"}); err != nil {
        t.Fatal(err)
    }

    got, err := svc.Profile(ctx, "telegram:6586915095")
    if err != nil {
        t.Fatal(err)
    }
    want := []string{"Blind", "Prefers exact outputs"}
    if !slices.Equal(got.Card, want) {
        t.Fatalf("card = %#v, want %#v", got.Card, want)
    }
}
```

```go
func TestService_ConcludeAndSearch(t *testing.T) {
    svc := newTestService(t)
    ctx := context.Background()

    _, err := svc.Conclude(ctx, ConcludeParams{
        Peer:       "telegram:6586915095",
        Conclusion: "The user prefers exact evidence-first reports.",
        SessionKey: "telegram:6586915095",
    })
    if err != nil {
        t.Fatal(err)
    }

    got, err := svc.Search(ctx, SearchParams{
        Peer:       "telegram:6586915095",
        Query:      "evidence-first",
        MaxTokens:  200,
        SessionKey: "telegram:6586915095",
    })
    if err != nil {
        t.Fatal(err)
    }
    if len(got.Results) == 0 {
        t.Fatal("want at least one search result")
    }
}
```

- [ ] **Step 2: Run the service tests to verify RED**

Run: `cd <repo>/gormes && go test ./internal/goncho -run 'ProfileRoundTrip|ConcludeAndSearch|Context' -v`

Expected: FAIL because the service does not exist yet

- [ ] **Step 3: Implement the minimal service**

Use these constraints:

```go
type Service struct {
    db          *sql.DB
    workspaceID string
    observer    string
    recentLimit int
}

func (s *Service) SetProfile(ctx context.Context, peer string, card []string) error
func (s *Service) Profile(ctx context.Context, peer string) (ProfileResult, error)
func (s *Service) Conclude(ctx context.Context, params ConcludeParams) (ConcludeResult, error)
func (s *Service) Search(ctx context.Context, params SearchParams) (SearchResultSet, error)
func (s *Service) Context(ctx context.Context, params ContextParams) (ContextResult, error)
```

Implementation rules:

- peer cards are global per peer
- conclusions are durable and idempotent
- representation is assembled on read from peer card + search results
- raw turns are only used as fallback when `session_key` is supplied

- [ ] **Step 4: Re-run the service tests**

Run: `cd <repo>/gormes && go test ./internal/goncho -v`

Expected: PASS

### Task 5: Expose `honcho_*` Tools

**Files:**
- Create: `gormes/internal/tools/honcho_tools.go`
- Create: `gormes/internal/tools/honcho_tools_test.go`
- Modify: `gormes/cmd/gormes/telegram.go`

- [ ] **Step 1: Write failing tool tests**

Add tests to lock names and behavior:

```go
func TestHonchoTools_RegisterExpectedNames(t *testing.T) {
    reg := NewRegistry()
    svc := goncho.NewService(testDB(t), goncho.Config{WorkspaceID: "default", ObserverPeerID: "gormes"}, nil)
    RegisterHonchoTools(reg, svc)

    for _, name := range []string{
        "honcho_profile",
        "honcho_search",
        "honcho_context",
        "honcho_reasoning",
        "honcho_conclude",
    } {
        if _, ok := reg.Get(name); !ok {
            t.Fatalf("%s not registered", name)
        }
    }
}
```

```go
func TestHonchoProfileTool_UsesService(t *testing.T) {
    // prime profile, execute tool, assert JSON includes the card
}
```

- [ ] **Step 2: Run the tool tests to verify RED**

Run: `cd <repo>/gormes && go test ./internal/tools -run Honcho -v`

Expected: FAIL because the tools are not implemented yet

- [ ] **Step 3: Implement the tools**

Keep compatibility names and argument names:

```go
type HonchoProfileTool struct { Service *goncho.Service }
type HonchoSearchTool struct { Service *goncho.Service }
type HonchoContextTool struct { Service *goncho.Service }
type HonchoReasoningTool struct { Service *goncho.Service }
type HonchoConcludeTool struct { Service *goncho.Service }
```

Rules:

- `honcho_reasoning` may return deterministic context-based synthesis in this slice
- `session_key` stays optional
- tool schemas should prefer `peer`, `query`, `max_tokens`, `reasoning_level`

- [ ] **Step 4: Wire the tools into the live-memory runtime**

In `cmd/gormes/telegram.go`, construct the Goncho service from `mstore.DB()` and register the tools against the runtime registry after `buildDefaultRegistry(...)`.

- [ ] **Step 5: Re-run the tool tests**

Run: `cd <repo>/gormes && go test ./internal/tools -run Honcho -v`

Expected: PASS

### Task 6: Verification And Repo Metadata

**Files:**
- Modify: none required for repo settings

- [ ] **Step 1: Run the focused verification suite**

Run:

```bash
cd <repo>/gormes
go test ./internal/goncho ./internal/tools ./internal/memory ./cmd/gormes -run 'Honcho|Goncho|Migrate|BuildDefaultRegistry' -v
```

Expected: exit code `0`

- [ ] **Step 2: Inspect `gh repo edit` help before changing settings**

Run: `gh repo edit --help`

Expected: output includes the flags for topics, issues, delete-branch-on-merge, auto-merge, update-branch, and wiki

- [ ] **Step 3: Apply repo settings**

Run the exact commands:

```bash
gh repo edit TrebuchetDynamics/gormes-agent --add-topic gormes --add-topic hermes --add-topic go --add-topic ai-agent --add-topic cli --add-topic terminal-ui
gh repo edit TrebuchetDynamics/gormes-agent --enable-issues --delete-branch-on-merge --enable-auto-merge --enable-update-branch --enable-wiki=false
```

- [ ] **Step 4: Record exact outputs**

Capture stdout and stderr exactly for the final report.

- [ ] **Step 5: Commit the implementation slice**

```bash
git add gormes/docs/superpowers/specs/2026-04-21-goncho-architecture-design.md \
        gormes/docs/superpowers/plans/2026-04-21-goncho-immediate-slice.md \
        gormes/internal/goncho \
        gormes/internal/tools/honcho_tools.go \
        gormes/internal/tools/honcho_tools_test.go \
        gormes/internal/memory/schema.go \
        gormes/internal/memory/migrate.go \
        gormes/internal/memory/migrate_test.go \
        gormes/cmd/gormes/telegram.go
git commit -m "feat: add goncho parity foundation"
```

---

Per user instruction, execute this plan inline in the current session rather than pausing for plan selection.
