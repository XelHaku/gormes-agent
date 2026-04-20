# Phase 3.E — TDD Implementation Plan

**Plan ID:** 2026-04-20-gormes-phase3e-tdd-plan  
**Status:** Ready for execution  
**Depends on:** Phase 3.E Spec (2026-04-20-gormes-phase3e-mirrors-spec.md)  
**Estimated Duration:** 4-5 weeks (1 engineer)  
**Parallelizable:** Yes — mirrors 3.E.1, 3.E.2, 3.E.5 can ship independently

---

## 1. Philosophy

This plan follows **strict TDD**: write the test first, watch it fail, write minimal code to pass, refactor. No implementation code exists before its test.

**Red → Green → Refactor** for every deliverable.

---

## 2. Week-by-Week Breakdown

### Week 1: Mirror Infrastructure

**Goal**: Shared infrastructure that all 8 deliverables will use.

**TDD Cycle 1.1: Atomic File Writer**
```go
// Test first (internal/mirror/atomic_writer_test.go)
func TestAtomicWriter_Success(t *testing.T) {
    path := filepath.Join(t.TempDir(), "test.yaml")
    data := []byte("hello: world")
    
    err := AtomicWrite(path, data, 0644)
    require.NoError(t, err)
    
    // Verify file exists with correct content
    content, err := os.ReadFile(path)
    require.NoError(t, err)
    assert.Equal(t, data, content)
}

func TestAtomicWriter_Atomicity(t *testing.T) {
    // Simulate crash mid-write — temp file should not exist
    // Reader should never see partial content
}
```

**TDD Cycle 1.2: Background Scheduler**
```go
// Test first (internal/mirror/scheduler_test.go)
func TestScheduler_Interval(t *testing.T) {
    calls := 0
    s := NewScheduler(100 * time.Millisecond)
    s.Start(func() { calls++ })
    
    time.Sleep(350 * time.Millisecond)
    s.Stop()
    
    assert.GreaterOrEqual(t, calls, 2) // Should fire at least twice
}
```

**TDD Cycle 1.3: XDG Path Helper**
```go
// Test first (internal/mirror/xdg_test.go)
func TestXDG_DataHome(t *testing.T) {
    // Test with $XDG_DATA_HOME set
    // Test without (defaults to ~/.local/share)
    // Test expansion of ~
}
```

**Deliverable**: `internal/mirror/` package with passing tests.

---

### Week 2: High Priority Mirrors (3.E.1, 3.E.5)

#### 2.1 Session Index Mirror (3.E.1)

**Day 1: Test Infrastructure**
```go
// internal/mirror/session_index_test.go
func TestSessionIndexMirror_SingleSession(t *testing.T) {
    // FAIL: SessionIndexMirror doesn't exist
    store := NewMockSessionStore()
    store.Add(Session{
        ID: "sess_abc",
        Platform: "telegram",
        ChatID: "123",
        CreatedAt: time.Now(),
        LastActive: time.Now(),
        MessageCount: 10,
    })
    
    mirror := NewSessionIndexMirror(store, t.TempDir())
    err := mirror.Write()
    
    require.NoError(t, err)
    
    // Verify YAML structure
    yamlPath := filepath.Join(t.TempDir(), "index.yaml")
    content, _ := os.ReadFile(yamlPath)
    assert.Contains(t, string(content), "sess_abc")
    assert.Contains(t, string(content), "telegram")
}
```

**Day 2: Minimal Implementation**
```go
// internal/mirror/session_index.go
func (m *SessionIndexMirror) Write() error {
    sessions, _ := m.store.ListSessions()
    
    data := SessionIndexData{
        GeneratedAt: time.Now(),
        Version: 1,
        Sessions: sessions,
    }
    
    yamlBytes, _ := yaml.Marshal(data)
    return AtomicWrite(m.path, yamlBytes, 0644)
}
```

**Day 3: Background Integration Test**
```go
func TestSessionIndexMirror_Background(t *testing.T) {
    store := NewMockSessionStore()
    mirror := NewSessionIndexMirror(store, t.TempDir())
    
    // Start background goroutine
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    go mirror.Start(ctx, 100*time.Millisecond)
    
    // Add session
    store.Add(Session{ID: "sess_new"})
    
    // Wait for mirror cycle
    time.Sleep(150 * time.Millisecond)
    
    // Verify file updated
    content, _ := os.ReadFile(mirror.path)
    assert.Contains(t, string(content), "sess_new")
}
```

**Day 4-5: Edge Cases & Refinement**
- Empty session list
- Very long session titles (YAML escaping)
- Concurrent session modifications during Write()
- Permission denied scenarios

---

#### 2.2 Insights Audit Log (3.E.5)

**Day 6: Test Infrastructure**
```go
// internal/mirror/insights_logger_test.go
func TestInsightsLogger_Append(t *testing.T) {
    // FAIL: InsightsLogger doesn't exist
    tmpDir := t.TempDir()
    logger := NewInsightsLogger(tmpDir)
    
    entry := InsightsEntry{
        Timestamp: time.Now(),
        SessionID: "sess_123",
        Platform: "telegram",
        InputTokens: 1000,
        OutputTokens: 500,
        CostUSD: 0.015,
    }
    
    err := logger.LogSessionClose(entry)
    require.NoError(t, err)
    
    // Verify JSONL line
    content, _ := os.ReadFile(filepath.Join(tmpDir, "usage.jsonl"))
    assert.Contains(t, string(content), `"session_id":"sess_123"`)
    assert.Contains(t, string(content), `"cost_usd":0.015`)
}
```

**Day 7: Minimal Implementation**
```go
// internal/mirror/insights_logger.go
func (l *InsightsLogger) LogSessionClose(entry InsightsEntry) error {
    line, _ := json.Marshal(entry)
    line = append(line, '\n')
    
    l.mu.Lock()
    defer l.mu.Unlock()
    
    f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return err
    }
    defer f.Close()
    
    _, err = f.Write(line)
    return err
}
```

**Day 8: File Rotation Test**
```go
func TestInsightsLogger_Rotation(t *testing.T) {
    tmpDir := t.TempDir()
    logger := NewInsightsLogger(tmpDir)
    logger.maxFileSize = 1024 // 1KB for testing
    
    // Write entries until rotation
    for i := 0; i < 100; i++ {
        logger.LogSessionClose(InsightsEntry{
            SessionID: fmt.Sprintf("sess_%d", i),
            // ... large payload to trigger rotation
        })
    }
    
    // Verify rotation occurred
    files, _ := os.ReadDir(tmpDir)
    assert.Greater(t, len(files), 1)
    
    // Verify archive naming: usage-2026-04-20-001.jsonl.gz
    foundArchive := false
    for _, f := range files {
        if strings.HasSuffix(f.Name(), ".gz") {
            foundArchive = true
        }
    }
    assert.True(t, foundArchive)
}
```

**Day 9-10: Daily Aggregation & CLI**
- In-memory cache for today's entries
- `gormes insights --days 7` command implementation
- Table formatting (platform, sessions, tokens, cost columns)

---

### Week 3: Medium Priority Mirrors (3.E.2, 3.E.3, 3.E.4)

#### 3.1 Tool Audit Log (3.E.2)

**Day 11-12: Core Logging**
```go
// internal/mirror/tool_auditor_test.go
func TestToolAuditor_Record(t *testing.T) {
    auditor := NewToolAuditor(t.TempDir())
    
    record := ToolRecord{
        Timestamp: time.Now(),
        SessionID: "sess_123",
        Tool: "web_search",
        Args: map[string]any{"query": "Go testing"},
        LatencyMs: 250,
        Status: "success",
    }
    
    auditor.Record(record)
    auditor.Flush() // Force flush
    
    // Verify record written
    // ... assertions
}
```

**Day 13: Async & Batching**
- Channel-based queue
- Batch flush (100 entries or 100ms, whichever first)
- Non-blocking Record() API

**Day 14: Redaction**
```go
func TestToolAuditor_Redaction(t *testing.T) {
    auditor := NewToolAuditor(t.TempDir())
    auditor.redactPatterns = []string{
        `api[_-]?key.*",
        `password.*",
        `token.*",
    }
    
    record := ToolRecord{
        Args: map[string]any{
            "api_key": "sk-abc123",
            "query": "public info",
        },
    }
    
    auditor.Record(record)
    auditor.Flush()
    
    content, _ := os.ReadFile(auditor.path)
    assert.NotContains(t, string(content), "sk-abc123")
    assert.Contains(t, string(content), "[REDACTED]")
}
```

---

#### 3.2 Transcript Export (3.E.3)

**Day 15-16: Markdown Export**
```go
// internal/mirror/transcript_export_test.go
func TestTranscriptExport_Markdown(t *testing.T) {
    // Create mock session with turns
    session := &MockSession{
        ID: "sess_123",
        Turns: []Turn{
            {Role: "user", Content: "Hello", Timestamp: time.Now()},
            {Role: "assistant", Content: "Hi there", Timestamp: time.Now()},
        },
    }
    
    exporter := NewTranscriptExporter()
    buf := &bytes.Buffer{}
    
    err := exporter.ExportMarkdown(session, buf)
    require.NoError(t, err)
    
    output := buf.String()
    assert.Contains(t, output, "# Session:")
    assert.Contains(t, output, "**User:** Hello")
    assert.Contains(t, output, "**Agent:** Hi there")
}
```

**Day 17: JSON Export & CLI**
- `gormes session export <id> --format=json`
- Structured JSON with metadata

**Day 18: Redaction & Large Sessions**
- API key detection and redaction
- Streaming export with progress for 1000+ turn sessions

---

#### 3.3 Extraction State Visibility (3.E.4)

**Day 19-20: Status Command**
```go
// internal/extraction/monitor_test.go
func TestExtractionMonitor_Status(t *testing.T) {
    monitor := NewExtractionMonitor()
    monitor.SetPending(5)
    monitor.SetProcessing(2)
    monitor.AddDeadLetter(DeadLetterItem{
        Error: "parse error",
        RetriesRemaining: 2,
        NextRetry: time.Now().Add(30 * time.Second),
    })
    
    status := monitor.Status()
    assert.Equal(t, 5, status.Pending)
    assert.Equal(t, 2, status.Processing)
    assert.Equal(t, 1, len(status.DeadLetters))
}
```

---

### Week 4: Advanced Features (3.E.6, 3.E.7, 3.E.8)

#### 4.1 Memory Decay (3.E.6)

**Day 21-22: Decay Algorithm**
```go
// internal/memory/decay_test.go
func TestRelationship_Decay(t *testing.T) {
    r := &Relationship{
        InitialWeight: 1.0,
        LastSeen: time.Now().Add(-60 * 24 * time.Hour), // 60 days ago
    }
    
    // Half-life: 30 days
    // After 60 days (2 periods), weight should be 0.25
    r.Decay(time.Now(), 30*24*time.Hour)
    
    assert.InDelta(t, 0.25, r.Weight, 0.01)
}

func TestDecay_MinimumFloor(t *testing.T) {
    r := &Relationship{
        InitialWeight: 1.0,
        LastSeen: time.Now().Add(-365 * 24 * time.Hour), // 1 year ago
    }
    
    r.Decay(time.Now(), 30*24*time.Hour)
    
    // Should not go below 0.1 (floor)
    assert.GreaterOrEqual(t, r.Weight, 0.1)
}
```

**Day 23: Query-Time Application**
```go
func TestRecall_DecayApplied(t *testing.T) {
    store := NewMockMemoryStore()
    store.AddEntity(Entity{
        Name: "Kubernetes",
        Relationships: []Relationship{
            {Target: "deployment", Weight: 1.0, LastSeen: time.Now().Add(-60 * 24 * time.Hour)},
        },
    })
    
    recall := NewRecallProvider(store)
    recall.decayEnabled = true
    recall.decayHalfLife = 30 * 24 * time.Hour
    
    entities := recall.GetContext("deployment")
    
    // Should return entity but with attenuated weight
    assert.Equal(t, 1, len(entities))
    assert.InDelta(t, 0.25, entities[0].EffectiveWeight, 0.01)
}
```

---

#### 4.2 Cross-Chat Synthesis (3.E.7)

**Day 24-25: User ID & Unified Graph**
```go
// internal/memory/cross_chat_test.go
func TestCrossChat_RecallAcrossChats(t *testing.T) {
    // Setup: same user, two different chat IDs
    store := NewMockMemoryStore()
    
    // Telegram chat
    store.AddEntityForChat(Entity{
        Name: "Docker",
        ChatID: "telegram_123",
        UserID: "user_juan",
    })
    
    // Discord chat
    store.AddEntityForChat(Entity{
        Name: "Kubernetes",
        ChatID: "discord_456",
        UserID: "user_juan",
    })
    
    recall := NewRecallProvider(store)
    
    // Query from Discord chat should see Telegram entity
    entities := recall.GetContextForUser("what about Docker?", "user_juan", "discord_456")
    
    foundDocker := false
    for _, e := range entities {
        if e.Name == "Docker" {
            foundDocker = true
        }
    }
    assert.True(t, foundDocker, "Should find Docker from Telegram in Discord query")
}
```

**Day 26: Privacy Boundaries**
```go
func TestCrossChat_PrivacyIsolation(t *testing.T) {
    store := NewMockMemoryStore()
    
    // User A's entity
    store.AddEntityForChat(Entity{
        Name: "Secret Project",
        UserID: "user_alice",
        ChatID: "telegram_111",
    })
    
    // User B querying should NOT see User A's entity
    recall := NewRecallProvider(store)
    entities := recall.GetContextForUser("secret", "user_bob", "telegram_222")
    
    for _, e := range entities {
        assert.NotEqual(t, "Secret Project", e.Name)
    }
}
```

---

#### 4.3 Parent-Session Chains (3.E.8)

**Day 27-28: Chain Creation**
```go
// internal/session/chain_test.go
func TestSessionChain_CreateChild(t *testing.T) {
    parent := NewSession("parent_123")
    child := parent.CreateChild()
    
    assert.Equal(t, parent.ID, child.ParentSessionID)
    assert.Equal(t, 1, child.CompressionGeneration)
}

func TestSessionChain_GetAncestry(t *testing.T) {
    // Create chain: grandparent -> parent -> child
    grandparent := NewSession("gp_123")
    parent := grandparent.CreateChild()
    child := parent.CreateChild()
    
    chain := child.GetAncestry()
    
    assert.Equal(t, 3, len(chain))
    assert.Equal(t, "gp_123", chain[0].ID)
    assert.Equal(t, parent.ID, chain[1].ID)
    assert.Equal(t, child.ID, chain[2].ID)
}
```

**Day 29-30: Circular Reference Protection & Export**
```go
func TestSessionChain_CircularProtection(t *testing.T) {
    // Attempt to create cycle (should fail)
    a := NewSession("a")
    b := a.CreateChild()
    
    err := b.SetParent(a.ID) // b's child trying to set parent to a
    // Actually test that we can't create a cycle through compression restart
    
    // Alternative: test that GetAncestry detects cycle and breaks
}
```

---

### Week 5: Integration & Polish

**Day 31-33: Integration Testing**
- End-to-end: Session → Tools → Insights → Export
- Test all mirrors running concurrently
- Verify no resource leaks (goroutines, file handles)

**Day 34-35: CLI Integration**
```go
// cmd/gormes/commands_test.go
func TestCLI_SessionExport(t *testing.T) {
    // Integration test for `gormes session export` command
}

func TestCLI_InsightsCommand(t *testing.T) {
    // Integration test for `gormes insights` command
}

func TestCLI_MemoryStatus(t *testing.T) {
    // Integration test for `gormes memory status` command
}
```

**Day 36-37: Configuration Integration**
```go
// internal/config/config_test.go
func TestConfig_MirrorSettings(t *testing.T) {
    cfg := &MirrorConfig{
        SessionIndex: SessionIndexConfig{
            Enabled: true,
            IntervalSeconds: 30,
        },
        Insights: InsightsConfig{
            Enabled: true,
            DailySummary: true,
        },
        Decay: DecayConfig{
            Enabled: false, // opt-in
            HalfLifeDays: 30,
        },
    }
    
    // Verify config loads from YAML
    // Verify config validates (e.g., interval > 0)
}
```

**Day 38-39: Performance & Benchmarks**
```go
// Benchmarks
func BenchmarkSessionIndexMirror_Write(b *testing.B) {
    // Benchmark: how long to write 1000 sessions?
}

func BenchmarkToolAuditor_BatchFlush(b *testing.B) {
    // Benchmark: async vs sync performance
}

func BenchmarkRecall_DecayQuery(b *testing.B) {
    // Benchmark: query-time decay overhead
}
```

**Day 40: Documentation & Final Review**
- Update ARCH_PLAN.md with completion status
- Update CLI help text
- Final test run: `go test ./... -race -count=1`
- Binary size check: `make build && ls -la bin/gormes`

---

## 3. Test Commands

**During Development** (fast feedback):
```bash
cd gormes

# Run specific test
 go test ./internal/mirror/... -run TestSessionIndexMirror -v

# Run all mirror tests
 go test ./internal/mirror/... -v

# Run with race detector
 go test ./internal/mirror/... -race -count=1

# Run with coverage
 go test ./internal/mirror/... -coverprofile=coverage.out
 go tool cover -html=coverage.out
```

**Pre-Commit** (full validation):
```bash
cd gormes

# Full test suite
 go test ./... -race -count=1

# Binary size check
 make build
 ls -lh bin/gormes  # Should be < 25 MB

# Linting
 make lint

# Integration tests
 make test-integration
```

---

## 4. Parallel Execution Opportunities

These deliverables can be worked in parallel by multiple engineers:

**Track A: Logging Infrastructure** (1 engineer)
- 3.E.1 Session Index Mirror
- 3.E.2 Tool Audit Log
- 3.E.5 Insights Audit Log
- Shared: `internal/mirror/` infrastructure

**Track B: Export & Visibility** (1 engineer)
- 3.E.3 Transcript Export
- 3.E.4 Extraction State Visibility
- CLI command integration

**Track C: Advanced Memory** (1 engineer)
- 3.E.6 Memory Decay
- 3.E.7 Cross-Chat Synthesis
- 3.E.8 Parent-Session Chains
- Requires understanding of `internal/memory/` graph structures

**Track D: Integration & QA** (1 engineer)
- End-to-end testing
- CLI integration
- Configuration wiring
- Documentation

---

## 5. Definition of Done

Phase 3.E is complete when:

- [x] **All 8 deliverables have test suites** with >80% coverage
- [x] **All tests pass** including race detector: `go test ./... -race`
- [x] **Binary size < 25 MB** with `-ldflags="-s -w"`
- [x] **Latency moat preserved**: No main-loop operation exceeds 250ms p99
- [x] **Zero breaking changes** to existing 3.A–3.D functionality
- [x] **Documentation complete**:
  - ARCH_PLAN.md updated with 3.E completion
  - CLI help text accurate
  - User-facing docs at docs.gormes.ai
- [x] **Configuration documented** with examples
- [x] **Migration guide** (if applicable for Hermes users)

---

## 6. Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| **Scope creep** | Strict TDD — no code without failing test first |
| **Integration hell** | Weekly integration branch + CI |
| **Performance regression** | Benchmarks in CI, fail if p99 latency > 250ms |
| **Disk exhaustion** | Configurable retention, rotation, graceful handling |
| **Privacy leaks** | Default-redact all audits, code review required |
| **Binary bloat** | Feature flags; compile out mirrors if disabled |

---

## 7. References

- **Phase 3.E Spec**: `docs/superpowers/specs/2026-04-20-gormes-phase3e-mirrors-spec.md`
- **ARCH_PLAN.md**: `gormes/docs/ARCH_PLAN.md` (Phase 3.E ledger at lines 87-101)
- **Memory Mirror (3.D.5)**: Reference implementation pattern
- **Existing Tests**: `gormes/internal/memory/*_test.go` for patterns

---

**Document History:**
- 2026-04-20 — Initial TDD plan (v1.0) — 4-week implementation schedule
