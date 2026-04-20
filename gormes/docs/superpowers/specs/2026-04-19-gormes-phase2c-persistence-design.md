# Gormes Phase 2.C — Thin Mapping Persistence (bbolt) Design

**Status:** Approved 2026-04-19 · execution pending plan
**Depends on:** Phase 2.B.1 (Telegram) green on `main`

## Related Documents

- [`gormes/docs/ARCH_PLAN.md`](../../ARCH_PLAN.md) — executive roadmap; Phase 3 ("The Black Box") owns the eventual SQLite + FTS5 memory layer. Phase 2.C is a surgical predecessor that solves the restart-UX bug without anticipating Phase 3.
- Phase 2.A — [`2026-04-19-gormes-phase2-tools-design.md`](2026-04-19-gormes-phase2-tools-design.md) — tool registry + kernel tool loop. Unchanged by this spec.
- Phase 2.B.1 — [`2026-04-19-gormes-phase2b-telegram.md`](2026-04-19-gormes-phase2b-telegram.md) — Telegram adapter + `cmd/gormes-telegram` binary. This spec extends that binary's startup/shutdown path.

---

## 1. Goal

When `cmd/gormes-telegram` (or `cmd/gormes`) restarts, the user's conversation must resume — the *next* message continues the same Python-server session instead of starting a fresh one.

## 2. Non-Goals

- **Gormes is not taking ownership of conversation history.** Python's `SessionDB` remains the canonical transcript store. Gormes persists only the handle required to re-attach to that transcript.
- **No token counting, no sliding-window pruning.** Because Gormes still sends only the latest user turn plus the `X-Hermes-Session-Id` header, token budgeting is a server-side concern. It becomes a Gormes concern in Phase 3 when Gormes owns the prompt.
- **No cross-platform session merging.** Resuming a `telegram:123` chat on the TUI is not supported in this phase — the keys intentionally partition state per platform.
- **No FTS5, no ontological graph.** Those are Phase 3.

## 3. Scope

Persist exactly one mapping, `(platform, chat_id) → session_id`, in a pure-Go embedded key-value store. Expose a thin interface so the adapters (Telegram bot, TUI) can prime the kernel at startup and record updates after each turn.

## 4. Architecture at a Glance

```
                         ┌─────────────────────────────┐
                         │  $XDG_DATA_HOME/gormes/     │
                         │    sessions.db (bbolt)      │
                         └──────────────┬──────────────┘
                                        │
                  ┌─────────────────────┴─────────────────────┐
                  │         internal/session.Map              │
                  │   (BoltMap production · MemMap tests)     │
                  └───────┬───────────────────────────┬───────┘
                          │                           │
            ┌─────────────┴──────────────┐  ┌─────────┴────────────────┐
            │      cmd/gormes-telegram    │  │       cmd/gormes          │
            │  key = "telegram:<chatID>"  │  │  key = "tui:default"      │
            └─────────────┬──────────────┘  └─────────┬────────────────┘
                          │                           │
                          ▼                           ▼
                  kernel.Config{InitialSessionID: "<loaded>"}
                          │
                          ▼
                  k.sessionID primes first turn → Python resumes the session
```

Each binary lifecycle:

1. **Startup**: open the bbolt file, `Get(key)` its session_id, pass it as `kernel.Config.InitialSessionID`.
2. **Runtime**: on every `RenderFrame` whose `SessionID` differs from the last persisted value, `Put(key, frame.SessionID)`.
3. **`/new` reset** (Telegram) or equivalent: kernel wipes its `sessionID` → next render frame carries `""` → adapter calls `Put(key, "")` which deletes the key.
4. **Shutdown**: `Close()` the `Map` before exit; bbolt releases its file lock.

## 5. Data Model

### 5.1 Bucket

- Single top-level bucket: `sessions_v1`.
- `_v1` suffix reserves namespace for Phase-3 schema evolution without clobber.

### 5.2 Key Encoding

UTF-8 bytes of `"<platform>:<chat_id>"`.

| Platform literal | chat_id | Example key |
|---|---|---|
| `tui` | always `default` | `"tui:default"` |
| `telegram` | decimal of `int64` | `"telegram:5551234567"` |

Helper constructors in `internal/session`:

```go
func TUIKey() string                  { return "tui:default" }
func TelegramKey(chatID int64) string { return "telegram:" + strconv.FormatInt(chatID, 10) }
```

**Validation:** both `platform` and `chat_id` segments must not contain `:` — the builder functions guarantee this by construction, so callers constructing keys by hand are not supported.

### 5.3 Value Encoding

Raw UTF-8 bytes of the server session_id. No JSON envelope, no timestamp, no version byte.

**`Put(key, "")` deletes the key** (clean "session cleared" semantics). Missing keys return `("", nil)` from `Get` — not `ErrNotFound`, since "no prior session" is the expected startup state for new users.

### 5.4 Storage File

`$XDG_DATA_HOME/gormes/sessions.db`

- Linux default: `~/.local/share/gormes/sessions.db`
- macOS/BSD default: `~/.local/share/gormes/sessions.db` (we deliberately do **not** use `~/Library/Application Support` — matches existing `config.LogPath()` convention).
- File mode: `0600`. Parent dir mode: `0700`.

Parent directory is auto-created on first Open if missing.

## 6. Interface

### 6.1 `internal/session` package

```go
package session

type Map interface {
    Get(ctx context.Context, key string) (sessionID string, err error)
    Put(ctx context.Context, key string, sessionID string) error
    Close() error
}
```

Contract:

- `Get` on a missing key returns `("", nil)`.
- `Put(key, "")` deletes the key (logical "session cleared"). Returns `nil` whether or not the key previously existed.
- `ctx` cancellation is honored at the boundary: both methods first check `ctx.Err()` and return it if already canceled. Mid-flight bbolt I/O is synchronous and not interruptible — callers must not expect sub-millisecond cancellation latency.
- `Close` is idempotent — second call returns `nil` without error.
- All methods are safe to call concurrently from multiple goroutines.

### 6.2 Production implementation — `BoltMap`

```go
type BoltMap struct { db *bolt.DB }

func OpenBolt(path string) (*BoltMap, error)
```

- Opens the bbolt file with `bolt.Options{Timeout: 100 * time.Millisecond}`.
- Creates parent dir (`0700`) and the `sessions_v1` bucket on first open (both idempotent via `os.MkdirAll` + `CreateBucketIfNotExists`).
- `Close()` calls `db.Close()` and nils the handle so future calls are no-ops.

### 6.3 Test implementation — `MemMap`

```go
type MemMap struct {
    mu sync.Mutex
    m  map[string]string
}

func NewMemMap() *MemMap
```

- Pure in-memory, pure Go.
- Honors the same `Put(key, "")` = delete contract.
- `Close()` is a no-op.

### 6.4 Kernel seam

One additive field on `kernel.Config`:

```go
type Config struct {
    // ... existing fields ...
    InitialSessionID string // optional; primes k.sessionID at New()
}
```

Implementation: `kernel.New` copies `cfg.InitialSessionID` into the unexported `k.sessionID` before the Run loop starts. Zero behavior change when the field is the zero value. No existing test modifications required.

### 6.5 Adapter persistence loops

**Telegram** (`cmd/gormes-telegram/main.go`):

```go
key := session.TelegramKey(cfg.Telegram.AllowedChatID)
sid, _ := smap.Get(ctx, key) // tolerate errors — log WARN and continue with ""
k := kernel.New(kernel.Config{..., InitialSessionID: sid}, ...)
```

The bot's existing render consumer (the outbound goroutine in `internal/telegram/bot.go`) gains one extra check per frame:

```go
if frame.SessionID != lastSID {
    _ = smap.Put(ctx, key, frame.SessionID)
    lastSID = frame.SessionID
}
```

No additional goroutines, no tee of the render channel. `lastSID` lives on the `Bot` struct. The **bot package** owns this loop — `cmd/` only wires the `smap` and `key` via `telegram.Config` (new optional fields `SessionMap` and `SessionKey`; nil `SessionMap` disables the hook, preserving all Phase-2.B.1 tests unchanged).

**TUI** (`cmd/gormes/main.go`): same pattern with `key := session.TUIKey()`. The existing TUI render consumer gains the same one-if-block persistence hook.

Kernel never imports `internal/session`. Verified by an extension to the T6 build-isolation test.

## 7. Error Handling

### 7.1 Startup (open the map)

| Condition | `OpenBolt` returns | Binary behavior |
|---|---|---|
| Happy path | `*BoltMap, nil` | proceed |
| Another process holds the file lock | `ErrDBLocked` | exit 1 with `"sessions.db locked — is another gormes process using $path?"` |
| File exists but bbolt magic invalid / header corrupt | `ErrDBCorrupt` | exit 1 with `"sessions.db appears corrupted; delete $path to reset (conversation resume will be lost)"` |
| Permission denied / disk full / ENOSPC | wrapped `fmt.Errorf` | exit 1 with the wrapped error |
| Parent dir not writable | wrapped `fmt.Errorf` | exit 1 |

No retry loops. No in-memory fallback. Persistence is an infrastructure contract — silent degradation is worse than loud failure.

### 7.2 Startup (read the initial session_id)

If `smap.Get(ctx, key)` returns a non-nil error at startup (e.g., transient I/O hiccup, bucket read failure after Open succeeded):

- Log `slog.Warn("could not load initial session_id", "key", key, "err", err)`.
- Treat as empty (`InitialSessionID = ""`).
- Binary continues startup — user gets a fresh session this boot.

Rationale: the bot must come up even against a flaky filesystem; losing *one* session_id read is acceptable.

### 7.3 Mid-run (Put failures)

`Put` can fail mid-conversation (fs unmount, disk full, file deleted out from under us). The persistence hook:

- Logs `slog.Warn("failed to persist session_id", "key", key, "err", err)`.
- Does **not** block the render goroutine.
- Does **not** fail the turn — the user's turn already succeeded against Python; only *resume* after the next restart is at risk.

### 7.4 Sentinel errors

Exported by `internal/session`:

```go
var ErrDBLocked  = errors.New("session: database locked by another process")
var ErrDBCorrupt = errors.New("session: database appears corrupted")
```

Consumers use `errors.Is` for classification. `BoltMap` translates bbolt's internal errors into these sentinels at the `OpenBolt` boundary.

## 8. CLI — `--resume` and related

### 8.1 Behavior

`cmd/gormes --resume <session_id>` and `cmd/gormes-telegram --resume <session_id>`:

1. Open the session map.
2. `Put(key, <session_id>)` where `key` is the binary's default (`tui:default` or `telegram:<allowed_chat_id>`).
3. Continue normal startup; the overwritten value is loaded as `InitialSessionID`.

**Net effect:** the next turn sends `X-Hermes-Session-Id: <session_id>` to Python and resumes that transcript.

### 8.2 Discovery

Session IDs are revealed at:

- **Log**: each turn's provenance line already contains `server_session_id=<id>` (see `internal/kernel/provenance.go`). Users grep their `gormes.log`.

`gormes sessions list` (listing persisted keys) and TUI status-bar indicators are explicitly **out of scope** for Phase 2.C — see §14.

### 8.3 Flag plumbing

`internal/config/config.go` gains a `Resume string` field (not a `toml` field — flag-only; TOML would be inappropriate for a one-shot override). Wired through the existing `loadFlags`:

```go
resume := fs.String("resume", "", "override persisted session_id for this binary's default key")
```

**Call-site change required:** `cmd/gormes/main.go` and `cmd/gormes-telegram/main.go` currently invoke `config.Load(nil)`, which skips flag parsing entirely. Both binaries must switch to `config.Load(os.Args[1:])` so the `--resume` flag actually binds.

After the map is opened, each binary does:

```go
key := ... // session.TUIKey() or session.TelegramKey(cfg.Telegram.AllowedChatID)
if cfg.Resume != "" {
    _ = smap.Put(ctx, key, cfg.Resume)
}
sid, _ := smap.Get(ctx, key)
// ... kernel.Config{InitialSessionID: sid}
```

## 9. Dependency Posture

- **New module dependency:** `go.etcd.io/bbolt` (latest 1.x). Pure Go, zero CGO, single-file binary.
- **Expected binary-size delta:** ~0.5 MB stripped per binary.
- **Budget check (post-Phase 2.C targets):**
  - `bin/gormes` ≤ **10 MB** (budget revised upward from 8.5 MB for Phase 2.C)
  - `bin/gormes-telegram` ≤ **12 MB** (unchanged)
- **Moat preservation:** the Phase 2.B.1 build-isolation test (T6) remains green — bbolt is pure Go, not a transport adapter. The test is extended to assert `internal/session` is not transitively imported by `internal/kernel`.

## 10. Security

- **File mode 0600, directory mode 0700.** Sessions.db contains opaque session handles; a leaked session_id lets an attacker resume *that* Python session against the same `X-Hermes-Session-Id` header, which is a privilege-escalation risk equivalent to leaking an auth token. 0600 matches how we'd treat credentials.
- **No secrets in keys.** `platform:chat_id` is not sensitive on its own, but the *values* (session_ids) are. Do not log values at INFO — the existing provenance line logs them at INFO already, but log rotation + 0600 config-dir permissions mitigate. This spec does not regress that posture.
- **No cross-platform disclosure:** reading `telegram:*` keys from the TUI binary is possible (same file), but there is no code path in this phase that does so. A future `sessions list` command will need an explicit `--include-other-platforms` flag.

## 11. Testing

### 11.1 Unit (no disk)

- Existing kernel + telegram tests: zero changes. They already inject `store.NewNoop()` and now *optionally* a `MemMap` where relevant.
- New: `TestBot_PersistsSessionIDToMap` (telegram pkg) — scripted `hermes.MockClient` yields a session_id; assert the bot calls `mem.Put(telegramKey, "<id>")` exactly once.
- New: `TestKernel_InitialSessionIDPrimesFirstRequest` (kernel pkg) — `kernel.New(Config{InitialSessionID: "sess-x"}, ...)`, submit a turn, assert the outbound `hermes.ChatRequest.SessionID == "sess-x"`.

### 11.2 Adapter (real disk via `t.TempDir()`)

Tests in `internal/session/bolt_test.go`:

- `TestBolt_PutGetRoundTrip` — write, read, assert byte-equal.
- `TestBolt_PutEmptyDeletes` — Put with `""` removes the key; subsequent Get returns `("", nil)`.
- `TestBolt_GetMissingReturnsEmpty` — Get on never-written key returns `("", nil)`.
- `TestBolt_CloseIdempotent` — double Close returns nil on second call.
- `TestBolt_ConcurrentPutGet` (under `-race`) — 100 goroutines × mixed Put/Get over 1000 ops; no data loss, no races.
- `TestBolt_AutoCreatesParentDir` — point at `<tmpdir>/newdir/sessions.db`; assert dir created with `0700`.

### 11.3 Failure modes (real disk)

- `TestBolt_LockContention` — open the same path twice; second returns `ErrDBLocked` within 200 ms.
- `TestBolt_CorruptFile` — write 4 KB of garbage to a temp file, OpenBolt; assert `errors.Is(err, ErrDBCorrupt)`.
- `TestBolt_PermissionDenied` — chmod parent dir to `0000`, OpenBolt; assert error is not one of the two sentinels and is surfaced to the caller unchanged. **Skip when `os.Getuid() == 0`** (root bypasses file-mode checks; common in CI containers).

### 11.4 End-to-end (mocked wiring)

- `TestBot_ResumesSessionIDAcrossRestart` (telegram pkg): single `MemMap`; boot a bot, submit a turn, tear it down; boot a new bot with the same `MemMap`; assert the new kernel's first outbound request carries the prior session_id.
- `TestTUI_ResumesSessionIDAcrossRestart` (tui pkg): same shape, TUI context.

### 11.5 Build-isolation (extends T6)

Extend `gormes/internal/buildisolation_test.go`:

- `TestTUIBinaryHasNoTelegramDep` — unchanged (still passes).
- **New** `TestKernelHasNoSessionDep` — `go list -deps ./internal/kernel` must not contain `/internal/session` or `go.etcd.io/bbolt`.

### 11.6 Full sweep

- `go test -race ./... -count=1 -timeout 120s` — all green.
- `go vet ./...` — clean.
- `go build -o bin/gormes ./cmd/gormes && go build -o bin/gormes-telegram ./cmd/gormes-telegram` — both succeed.
- `ls -lh bin/` — assertions on size budgets from §9.

## 12. Verification Checklist

- [ ] `go build` still produces static binaries (`CGO_ENABLED=0` honored).
- [ ] `bin/gormes-telegram` survives a `SIGTERM` and resumes context on restart (manual smoke — documented in §13).
- [ ] `go list -deps ./cmd/gormes` shows zero database-specific logic in `internal/kernel`.
- [ ] `go list -deps ./internal/kernel` does not contain `go.etcd.io/bbolt` or `internal/session`.
- [ ] Binary sizes within the §9 budgets.
- [ ] All tests in §11 green under `-race`.
- [ ] `go vet ./...` clean.

## 13. Manual Smoke Test (Phase 2.C close-out)

Requires a working Telegram bot token + running Python `api_server`.

```bash
# Terminal 1
API_SERVER_ENABLED=true hermes gateway start

# Terminal 2
export GORMES_TELEGRAM_TOKEN=<your:token>
export GORMES_TELEGRAM_CHAT_ID=<your:chat:id>
./bin/gormes-telegram &   # first run
# DM the bot: "remember the word 'asparagus'"
# bot replies; note the server_session_id line in stderr
kill %1                    # SIGTERM

./bin/gormes-telegram      # second run — cold boot
# DM the bot: "what word did you remember?"
# bot should reply with 'asparagus' — PROOF the mapping persisted
```

Verify: `cat ~/.local/share/gormes/sessions.db` (binary; use `bbolt keys ~/.local/share/gormes/sessions.db sessions_v1` or `strings` to spot-check the session_id).

## 14. Out of Scope (explicit deferrals)

- Multi-chat support (one bot, many allowlisted chats) — Phase 2.B.2.
- `gormes sessions list` / `gormes sessions delete` commands — follow-up task if UX demand appears.
- Cross-platform session linking (resume `telegram:123` on the TUI) — Phase 3.
- Token counting / sliding window pruning — Phase 3 (when Gormes owns the prompt).
- FTS5 / ontological graph — Phase 3.

## 15. Rollout

- Ships as a single commit series under one PR (no feature flag).
- First boot on existing installs: no `sessions.db` file exists → `OpenBolt` creates it, `Get` returns `("", nil)`, binary starts a fresh session — identical to today's behavior. **Zero-regression launch.**
- No migration story required. Phase 3 will migrate bbolt → SQLite via a one-time iterate-and-insert script shipped as `cmd/gormes-migrate-sessions`.
