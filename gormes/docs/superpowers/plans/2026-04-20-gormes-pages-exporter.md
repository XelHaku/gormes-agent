# Gormes Pages Exporter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Pages-compatible static exporter for `gormes/www.gormes.ai` that writes `dist/index.html` and `dist/static/*` from the same rendering core used by the HTTP server.

**Architecture:** Move homepage rendering and static asset access behind shared helpers in `www.gormes.ai/internal/site`. Keep `cmd/www-gormes` as HTTP-only delivery and add `cmd/www-gormes-export` as filesystem-only delivery. Export output must be deterministic, recreate `dist` cleanly, and preserve served asset paths.

**Tech Stack:** Go, `embed`, `html/template`, `io/fs`, `net/http`, filesystem I/O, `go test`.

---

## File Structure Map

```
gormes/www.gormes.ai/
├── cmd/
│   ├── www-gormes/
│   │   └── main.go                         # MODIFY — keep HTTP entrypoint thin
│   └── www-gormes-export/
│       └── main.go                         # NEW — build dist/ from shared site package
├── internal/site/
│   ├── assets.go                           # MODIFY — expose embedded asset helpers
│   ├── assets_test.go                      # MODIFY — keep asset-path guarantees
│   ├── export_test.go                      # NEW — dist/index.html + dist/static smoke coverage
│   ├── render_test.go                      # MODIFY — cover shared render primitives
│   └── server.go                           # MODIFY — build handler from shared render helpers
└── docs/superpowers/plans/
    └── 2026-04-20-gormes-pages-exporter.md # THIS FILE
```

---

### Task 1: Lock shared render/export behavior with tests

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/render_test.go`
- Add: `gormes/www.gormes.ai/internal/site/export_test.go`
- Test: `gormes/www.gormes.ai/internal/site/render_test.go`
- Test: `gormes/www.gormes.ai/internal/site/export_test.go`

- [ ] **Step 1: Write failing render and export tests**

Add a shared-render test that asserts the homepage can be rendered without `httptest`, and an exporter test that expects:

```text
<tempdir>/dist/index.html
<tempdir>/dist/static/site.css
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes/www.gormes.ai
go test ./internal/site -run 'Test(Render|Export)'
```

Expected: compile failure or missing symbol failures for the new shared render/export APIs.

### Task 2: Implement shared site render/export primitives

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/server.go`
- Modify: `gormes/www.gormes.ai/internal/site/assets.go`

- [ ] **Step 1: Add minimal shared helpers**

Expose helpers that let both delivery modes share one rendering truth:

```go
func RenderIndex() ([]byte, error)
func ExportDir(root string) error
```

Keep the implementation small: parse embedded templates, render `DefaultPage()` once per call, recreate `root`, write `index.html`, and copy embedded static files to `root/static`.

- [ ] **Step 2: Run focused tests to verify they pass**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes/www.gormes.ai
go test ./internal/site -run 'Test(Render|Export)'
```

Expected: PASS.

### Task 3: Add the Pages exporter command and keep HTTP thin

**Files:**
- Add: `gormes/www.gormes.ai/cmd/www-gormes-export/main.go`
- Modify: `gormes/www.gormes.ai/cmd/www-gormes/main.go`

- [ ] **Step 1: Add the exporter command**

Create a command that writes `dist` by default and exits non-zero on any export error.

- [ ] **Step 2: Keep the server command thin**

Keep `cmd/www-gormes` responsible only for listen flag parsing and `http.ListenAndServe`, delegating render work to `internal/site`.

- [ ] **Step 3: Verify the command path works**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes/www.gormes.ai
go run ./cmd/www-gormes-export
test -f dist/index.html
test -f dist/static/site.css
```

Expected: exit code `0` and both files present.

### Task 4: Full verification

**Files:**
- Verify only

- [ ] **Step 1: Run the site package tests**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes/www.gormes.ai
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Record Pages dashboard settings**

Use:

```text
Root directory: gormes/www.gormes.ai
Build command: go run ./cmd/www-gormes-export
Build output directory: dist
```
