# Gormes Phase 1.5 TDD Rig — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install the three TDD artifacts from spec `2026-04-19-gormes-phase1-5-tdd-rig-design.md`: a Go 1.22 compatibility probe, a red chaos test for Route-B reconnect (ships `t.Skip`'d), and a green test proving the kernel's capacity-1 replace-latest render mailbox does not deadlock under a stalled consumer.

**Architecture:** Tests-only plus one enum value. The red test references a named `PhaseReconnecting` constant that is added to `kernel/frame.go` as a TDD seed — the kernel never transitions to that state yet; the future Route-B plan flips the red test green. The compat script is stand-alone bash, invoked manually.

**Tech Stack:** Go 1.22+ (with `toolchain go1.26.1`), bash, Docker (preferred) or `golang.org/dl/go1.22.10` fallback, stdlib `net/http/httptest`, existing `MockClient` from `gormes/internal/hermes/`.

---

## Prerequisites

- Go 1.22+ and the current `toolchain go1.26.1`
- Working directory: repository root. All new files under `gormes/`.
- Docker **or** ability to `go install golang.org/dl/go1.22.10@latest` (not both required — script detects)
- Phase 1 must be on `main` (latest commit in the `20aa89d4..HEAD` range)

## File Structure Map

```
gormes/
├── scripts/
│   └── check-go1.22-compat.sh                  # NEW — portability probe
├── internal/
│   └── kernel/
│       ├── frame.go                            # MODIFY — add PhaseReconnecting enum value
│       ├── reconnect_test.go                   # NEW — red chaos test (t.Skip'd)
│       ├── reconnect_helpers_test.go           # NEW — httptest server helpers for the red test
│       └── stall_test.go                       # NEW — green replace-latest test
└── docs/
    └── superpowers/
        └── plans/
            └── 2026-04-19-gormes-phase1-5-tdd-rig.md   # this file
```

---

## Task 1: Go 1.22 Compatibility Script

**Files:**
- Create: `gormes/scripts/check-go1.22-compat.sh`

- [ ] **Step 1:** Create the directory if it doesn't exist and write the script.

```bash
mkdir -p gormes/scripts
```

Then create `gormes/scripts/check-go1.22-compat.sh` with exactly this content:

```bash
#!/usr/bin/env bash
#
# Check whether the Gormes binary builds under Go 1.22.
#
# Exit codes:
#   0 — build succeeded; Termux/LTS portability preserved
#   1 — build failed under Go 1.22; offending packages listed
#   2 — neither Docker nor `go1.22.10` toolchain is available
#
# Preferred path: Docker (golang:1.22-alpine).
# Fallback path:  golang.org/dl/go1.22.10 downloadable toolchain.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LOG="$(mktemp)"
trap 'rm -f "${LOG}"' EXIT

print_decision_summary() {
    local status="$1"
    echo
    echo "=== Decision data for 'Portability vs. Progress' ==="
    if [[ "${status}" -eq 0 ]]; then
        echo "  ✓ Go 1.22 builds cleanly — no action needed"
        return
    fi
    # Extract "requires go1.23" / "requires go1.24" lines and the
    # package paths that preceded them.
    local offenders
    offenders=$(grep -E 'requires go1\.[0-9]+' "${LOG}" | sort -u || true)
    if [[ -n "${offenders}" ]]; then
        echo "Incompatible dependencies:"
        echo "${offenders}" | sed 's/^/  /'
    fi
    # Also surface undefined-symbol lines from the build failure.
    local symbols
    symbols=$(grep -E '^.*: undefined: .+' "${LOG}" | sort -u || true)
    if [[ -n "${symbols}" ]]; then
        echo
        echo "Undefined symbols in 1.22 toolchain:"
        echo "${symbols}" | sed 's/^/  /'
    fi
    echo
    echo "Options:"
    echo "  a) Accept Go 1.24 floor as the Gormes minimum"
    echo "  b) Downgrade the offending dependencies to 1.22-compatible versions"
    echo
    echo "Raw build log (last 40 lines):"
    tail -40 "${LOG}" | sed 's/^/  /'
}

try_docker() {
    if ! command -v docker >/dev/null 2>&1; then
        return 2
    fi
    echo "=== Go 1.22 compatibility check (Docker path) ==="
    # Run the build in an ephemeral container. Mount repo read-only;
    # use a tmpfs cache for the build to stay inert against host state.
    docker run --rm \
        -v "${REPO_ROOT}:/src:ro" \
        -w /src/gormes \
        --tmpfs /tmp \
        -e GOCACHE=/tmp/gocache \
        -e GOMODCACHE=/tmp/gomodcache \
        golang:1.22-alpine \
        sh -c 'go build ./cmd/gormes' \
        > "${LOG}" 2>&1
    return $?
}

try_fallback() {
    if ! command -v go >/dev/null 2>&1; then
        return 2
    fi
    if ! command -v go1.22.10 >/dev/null 2>&1; then
        echo "Installing golang.org/dl/go1.22.10 …"
        GO111MODULE=on go install golang.org/dl/go1.22.10@latest >> "${LOG}" 2>&1 || return 2
        go1.22.10 download >> "${LOG}" 2>&1 || return 2
    fi
    echo "=== Go 1.22 compatibility check (fallback path) ==="
    (cd "${REPO_ROOT}/gormes" && go1.22.10 build ./cmd/gormes) >> "${LOG}" 2>&1
    return $?
}

main() {
    local status
    try_docker
    status=$?
    if [[ "${status}" -eq 2 ]]; then
        try_fallback
        status=$?
    fi

    case "${status}" in
        0)
            echo "PASS: gormes builds under Go 1.22"
            print_decision_summary 0
            exit 0
            ;;
        2)
            echo "UNAVAILABLE: neither Docker nor golang.org/dl/go1.22.10 could be used."
            echo "  Install one:"
            echo "    - Docker Desktop / docker.io"
            echo "    - go install golang.org/dl/go1.22.10@latest && go1.22.10 download"
            exit 2
            ;;
        *)
            echo "FAIL: gormes does NOT build under Go 1.22 (exit ${status})"
            print_decision_summary "${status}"
            exit 1
            ;;
    esac
}

main "$@"
```

- [ ] **Step 2:** Make it executable.

```bash
chmod +x gormes/scripts/check-go1.22-compat.sh
```

- [ ] **Step 3:** Run it once to confirm it runs without syntax errors. Expected outcome depends on host:
  - Docker present: script runs, either passes (unlikely given bubbletea@latest needs 1.24) or fails with named offenders.
  - Docker absent + no `go1.22.10`: exits 2 with install instructions.
  - Docker absent + `go1.22.10` installable: fallback runs.

```bash
./gormes/scripts/check-go1.22-compat.sh || echo "exit=$?"
```

Record the exit code for the commit message.

- [ ] **Step 4:** Commit.

```bash
git add gormes/scripts/check-go1.22-compat.sh
git commit -m "$(cat <<'EOF'
feat(gormes/scripts): Go 1.22 compatibility probe

Standalone bash script that attempts a Gormes build under Go 1.22
(Docker preferred; golang.org/dl/go1.22.10 fallback). On failure,
parses the build log for "requires goX.Y" + "undefined: <symbol>"
lines and prints a structured report:

  - offending dependencies + required Go versions
  - specific undefined symbols encountered under 1.22
  - last 40 lines of raw build log

Exit codes:
  0 = builds under 1.22 (portability preserved)
  1 = fails under 1.22 (offenders named, options listed)
  2 = neither Docker nor go1.22.10 toolchain available

Decision data for "accept 1.24 floor vs downgrade deps" is now
mechanical, not judgment.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Seed `PhaseReconnecting` Enum Value

**Files:**
- Modify: `gormes/internal/kernel/frame.go` (add one enum value + update String method)

- [ ] **Step 1:** Read `gormes/internal/kernel/frame.go` to confirm current state.

The current enum block:
```go
const (
    PhaseIdle Phase = iota
    PhaseConnecting
    PhaseStreaming
    PhaseFinalizing
    PhaseCancelling
    PhaseFailed
)

func (p Phase) String() string {
    return [...]string{"Idle", "Connecting", "Streaming", "Finalizing", "Cancelling", "Failed"}[p]
}
```

- [ ] **Step 2:** Edit the const block via Edit tool. Change:

```go
const (
    PhaseIdle Phase = iota
    PhaseConnecting
    PhaseStreaming
    PhaseFinalizing
    PhaseCancelling
    PhaseFailed
)
```

to:

```go
const (
    PhaseIdle Phase = iota
    PhaseConnecting
    PhaseStreaming
    PhaseFinalizing
    PhaseCancelling
    PhaseFailed
    // PhaseReconnecting is the TDD seed for Phase-1.5 Route-B resilience
    // (spec §9.2 of 2026-04-18-gormes-frontend-adapter-design.md). No
    // transitions to this state exist yet — the future reconnect plan
    // flips reconnect_test.go from Skip to real pass by wiring this up.
    PhaseReconnecting
)
```

- [ ] **Step 3:** Edit the String method. Change:

```go
func (p Phase) String() string {
    return [...]string{"Idle", "Connecting", "Streaming", "Finalizing", "Cancelling", "Failed"}[p]
}
```

to:

```go
func (p Phase) String() string {
    return [...]string{"Idle", "Connecting", "Streaming", "Finalizing", "Cancelling", "Failed", "Reconnecting"}[p]
}
```

- [ ] **Step 4:** Verify the whole repo still builds and tests pass.

```bash
cd gormes
go build ./...
go vet ./...
go test ./... -timeout 60s
```

Expected: all clean. The new enum value is unreferenced but that's intentional — it's a TDD seed.

- [ ] **Step 5:** Commit.

```bash
cd ..
git add gormes/internal/kernel/frame.go
git commit -m "$(cat <<'EOF'
feat(gormes/kernel): seed PhaseReconnecting for Route-B TDD

One-line enum addition. No kernel behaviour change — no code path
transitions to this state yet. Exists so the Phase-1.5 reconnect
red test (next task) references the production-vocabulary named
constant instead of a magic integer.

The future Route-B plan (spec §9.2) is the code that actually
moves the kernel into this state.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Red Chaos Test — `reconnect_test.go`

**Files:**
- Create: `gormes/internal/kernel/reconnect_helpers_test.go`
- Create: `gormes/internal/kernel/reconnect_test.go`

- [ ] **Step 1:** Create `gormes/internal/kernel/reconnect_helpers_test.go` with the scaffolding shared between the red test and any future Route-B tests:

```go
package kernel

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// stableProxy listens on a fixed local port and forwards traffic to whatever
// backend URL is currently registered. Used by the reconnect red test so the
// kernel can keep pointing at one stable URL across a simulated server restart.
//
// Not used by the Task-3 t.Skip'd test — future Route-B green test will need it.
// Shipping here so Task 3's helpers file is complete for future reuse.
type stableProxy struct {
	listener net.Listener
	backend  atomic.Value // string
	srv      *http.Server
}

func newStableProxy(t *testing.T) *stableProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("stableProxy listen: %v", err)
	}
	p := &stableProxy{listener: ln}
	p.backend.Store("")
	p.srv = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			backend, _ := p.backend.Load().(string)
			if backend == "" {
				http.Error(w, "no backend registered", http.StatusServiceUnavailable)
				return
			}
			// Simple forwarder: rewrite URL, hand to the default transport.
			outReq := r.Clone(r.Context())
			outReq.URL.Scheme = "http"
			outReq.URL.Host = backendHost(backend)
			outReq.RequestURI = ""
			resp, err := http.DefaultTransport.RoundTrip(outReq)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			for k, v := range resp.Header {
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
			w.WriteHeader(resp.StatusCode)
			// Stream the response body line-by-line so SSE isn't buffered.
			br := bufio.NewReader(resp.Body)
			flusher, _ := w.(http.Flusher)
			buf := make([]byte, 1024)
			for {
				n, err := br.Read(buf)
				if n > 0 {
					_, _ = w.Write(buf[:n])
					if flusher != nil {
						flusher.Flush()
					}
				}
				if err != nil {
					return
				}
			}
		}),
	}
	go func() { _ = p.srv.Serve(ln) }()
	return p
}

func (p *stableProxy) URL() string {
	return fmt.Sprintf("http://%s", p.listener.Addr().String())
}

func (p *stableProxy) Rebind(backendURL string) {
	p.backend.Store(backendURL)
}

func (p *stableProxy) Close() { _ = p.srv.Close() }

func backendHost(u string) string {
	// "http://127.0.0.1:12345" -> "127.0.0.1:12345"
	const prefix = "http://"
	if len(u) > len(prefix) && u[:len(prefix)] == prefix {
		return u[len(prefix):]
	}
	return u
}

// fiveTokenHandler returns an httptest.HandlerFunc that flushes 5 SSE
// "content" frames and then hangs open (no [DONE]) so the client sees a
// chaos-monkey disconnect, not a normal end-of-stream.
func fiveTokenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Hermes-Session-Id", "sess-reconnect")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 5; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(10 * time.Millisecond)
		}
		// Hang. The test triggers a chaos-monkey disconnect before we return.
		<-r.Context().Done()
	}
}

// tenTokenHandler returns an httptest.HandlerFunc that emits 10 "y" tokens +
// finish_reason=stop + [DONE]. Used for the post-reconnect retry server.
func tenTokenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Hermes-Session-Id", "sess-reconnect")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 10; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"y\"}}]}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":10}}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// healthyHandler answers /health with 200 so the kernel's initial POST to
// /v1/chat/completions is not blocked by the main.go health check (tests
// bypass main but may call Client.Health directly).
func healthyHandler(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// newRealKernelFromURL constructs a kernel against a real hermes.NewHTTPClient
// pointed at the given base URL. Used only by tests that specifically need
// the HTTP path (not MockClient).
func newRealKernelFromURL(t *testing.T, baseURL string) *Kernel {
	t.Helper()
	// Imports pulled through the shared hermes package live with the rest
	// of the kernel's own imports; this helper only needs Kernel + Config.
	return nil // placeholder; real impl below wraps hermes.NewHTTPClient
}
```

Note: `newRealKernelFromURL` is a placeholder in this file because the red test is `t.Skip`'d and never calls it. If you want to be maximally clean, delete that stub — but leaving it is harmless dead code marked for the future Route-B test.

- [ ] **Step 2:** Create `gormes/internal/kernel/reconnect_test.go` with the red chaos test:

```go
package kernel

import (
	"testing"
)

// TestKernel_HandlesMidStreamNetworkDrop is the Phase-1.5 red chaos test for
// the Route-B reconnect feature documented in spec §9.2 of
// 2026-04-18-gormes-frontend-adapter-design.md.
//
// Currently t.Skip'd. CURRENTLY FAILS (by design) against the shipped kernel
// at four distinct assertions:
//
//  1. PhaseReconnecting transition after TCP drop
//     → current kernel transitions to PhaseFailed
//
//  2. Draft preserved during reconnect window (5 tokens stay visible)
//     → current kernel leaves draft in place but phase is already wrong,
//       so the invariant is not coherent
//
//  3. Automatic recovery back to PhaseStreaming → PhaseIdle after backoff
//     → current kernel has no retry loop
//
//  4. Final history contains exactly ONE clean assistant message from the
//     successful retry (no Frankenstein concatenation)
//     → current kernel never retries, so no such history entry exists
//
// The future Route-B implementation plan flips this test from Skip to real
// pass by wiring PhaseReconnecting into runTurn's error path with jittered
// exponential backoff (1s, 2s, 4s, 8s, 16s caps).
func TestKernel_HandlesMidStreamNetworkDrop(t *testing.T) {
	t.Skip("RED TEST: Route B Resilience — Implementation pending (see spec §9.2 of 2026-04-18-gormes-frontend-adapter-design.md)")

	// When the implementation lands, delete the Skip above and implement the
	// assertions using the helpers in reconnect_helpers_test.go:
	//
	//   1. p := newStableProxy(t); defer p.Close()
	//   2. srv1 := httptest.NewServer(fiveTokenHandler()); p.Rebind(srv1.URL)
	//   3. k := newRealKernelFromURL(t, p.URL()); go k.Run(ctx)
	//   4. k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"})
	//   5. Wait for draft of length 5 + PhaseStreaming
	//   6. srv1.CloseClientConnections()  // chaos monkey
	//   7. ASSERT 1: phase == PhaseReconnecting within 500ms
	//   8. ASSERT 2: draft still contains "xxxxx"
	//   9. srv2 := httptest.NewServer(tenTokenHandler()); p.Rebind(srv2.URL)
	//  10. ASSERT 3: phase transitions Streaming → Idle within 20s
	//  11. ASSERT 4: final history has one assistant msg == "yyyyyyyyyy"
}
```

- [ ] **Step 3:** Verify the test runs as a skip (not a failure) and the rest of the suite still passes.

```bash
cd gormes
go test ./internal/kernel/... -v -run TestKernel_HandlesMidStreamNetworkDrop
```

Expected output includes:
```
=== RUN   TestKernel_HandlesMidStreamNetworkDrop
--- SKIP: TestKernel_HandlesMidStreamNetworkDrop (0.00s)
    reconnect_test.go:XX: RED TEST: Route B Resilience — Implementation pending ...
PASS
```

Then the whole kernel suite:
```bash
go test ./internal/kernel/... -v
```

All existing discipline tests still PASS; new test shows SKIP.

- [ ] **Step 4:** Also confirm the full repo with `-race`.

```bash
go test -race ./... -timeout 60s
```

All PASS. No DATA RACE.

- [ ] **Step 5:** Commit.

```bash
cd ..
git add gormes/internal/kernel/reconnect_test.go gormes/internal/kernel/reconnect_helpers_test.go
git commit -m "$(cat <<'EOF'
test(gormes/kernel): red chaos test for Route-B reconnect (t.Skip'd)

Technical-Debt Beacon for the Phase-1.5 Route-B resilience
feature (spec §9.2 of 2026-04-18-gormes-frontend-adapter-design.md).
Ships with t.Skip + a precise reason so CI stays green ("green is
sacred"); the Route-B implementation plan flips this test from
Skip to a real pass.

The test is fully documented in a long comment listing the four
assertions that CURRENTLY FAIL against the shipped kernel:
  1. No PhaseReconnecting transition on TCP drop
  2. No draft preservation during reconnect window
  3. No auto-retry back to Streaming → Idle
  4. No clean single-message final history

Helpers in reconnect_helpers_test.go (stableProxy for rebindable
URLs, fiveTokenHandler, tenTokenHandler, healthyHandler) will be
reused by the green test when the feature lands.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Green Stall Test — `stall_test.go`

**Files:**
- Create: `gormes/internal/kernel/stall_test.go`

- [ ] **Step 1:** Create `gormes/internal/kernel/stall_test.go`:

```go
package kernel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
)

// TestKernel_NonBlockingUnderTUIStall proves the capacity-1 replace-latest
// render mailbox invariant (spec §7.8) holds under a maliciously-stalled
// consumer. If the kernel ever blocks on an emitFrame send — e.g. if a
// future refactor changed the mailbox from capacity-1 + drain-then-send to
// an unbuffered or blocking channel — this test deadlocks and fails the
// 5-second timeout.
//
// Treats the kernel as a black box: we inspect the render channel only,
// never internal state. No test-only accessors on production types.
func TestKernel_NonBlockingUnderTUIStall(t *testing.T) {
	// Build a kernel with a MockClient scripted to emit 1000 tokens + Done.
	mc := hermes.NewMockClient()
	events := make([]hermes.Event, 0, 1001)
	for i := 0; i < 1000; i++ {
		events = append(events, hermes.Event{
			Kind: hermes.EventToken, Token: "t", TokensOut: i + 1,
		})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: 1000})
	mc.Script(events, "sess-stall")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go k.Run(ctx)

	// Read only the initial idle frame so the kernel enters its main select,
	// then submit and stop reading. The replace-latest invariant says the
	// kernel must keep making progress even though nobody consumes frames.
	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial = %v, want PhaseIdle", initial.Phase)
	}
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// STALL: do NOT drain k.Render() for 2 seconds. If the kernel blocks
	// on emit, it will not complete the 1000-token turn in this window.
	time.Sleep(2 * time.Second)

	// Now peek the single frame that sits in the capacity-1 mailbox. It
	// must be the LATEST state — the replace-latest invariant says a
	// stale mid-stream frame is forbidden.
	var peeked RenderFrame
	select {
	case peeked = <-k.Render():
	default:
		t.Fatal("no frame available after 2s stall — kernel may have deadlocked on emit")
	}

	// The peeked frame is valid if:
	//   (a) the turn completed: Phase == PhaseIdle AND history has the
	//       assistant message "t" x 1000, OR
	//   (b) streaming is still in progress but the draft has caught up
	//       to the full 1000 tokens (a very-late mid-stream frame — can
	//       happen if the pump and finalize race on the edge of the sleep).
	//
	// A stale mid-stream frame with a partial draft is a FAILURE — it
	// means the kernel stopped at the stale frame and never wrote a newer one.
	ok := false
	wantAssistant := strings.Repeat("t", 1000)

	if peeked.Phase == PhaseIdle {
		assistant := lastAssistantMessage(peeked.History)
		if assistant != nil && assistant.Content == wantAssistant {
			ok = true
		}
	}
	if peeked.Phase == PhaseStreaming && peeked.DraftText == wantAssistant {
		ok = true
	}
	if peeked.Phase == PhaseFinalizing && peeked.DraftText == wantAssistant {
		ok = true
	}

	if !ok {
		t.Fatalf("replace-latest invariant violated — peeked stale frame: phase=%v seq=%d draftLen=%d historyLen=%d",
			peeked.Phase, peeked.Seq, len(peeked.DraftText), len(peeked.History))
	}

	// Drain any remaining frames so the kernel can exit cleanly on ctx
	// timeout. Non-blocking — we're not asserting on these.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range k.Render() {
		}
	}()
	<-ctx.Done()
	<-done
}

// lastAssistantMessage returns a pointer to the last hermes.Message with
// Role "assistant", or nil if none exist. Helper shared with future tests.
func lastAssistantMessage(history []hermes.Message) *hermes.Message {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "assistant" {
			return &history[i]
		}
	}
	return nil
}
```

- [ ] **Step 2:** Run the new test.

```bash
cd gormes
go test ./internal/kernel/... -v -run TestKernel_NonBlockingUnderTUIStall -timeout 30s
```

Expected: `--- PASS: TestKernel_NonBlockingUnderTUIStall`. If it times out or fails, the replace-latest invariant is broken — the kernel is blocking on emit. That's a kernel bug, NOT a test bug. Report `DONE_WITH_CONCERNS` if this happens.

- [ ] **Step 3:** Confirm it's also `-race` clean.

```bash
go test -race ./internal/kernel/... -run TestKernel_NonBlockingUnderTUIStall -timeout 60s
```

PASS. No DATA RACE.

- [ ] **Step 4:** Full-repo sweep.

```bash
go test -race ./... -timeout 60s
go vet ./...
```

All PASS. All clean.

- [ ] **Step 5:** Commit.

```bash
cd ..
git add gormes/internal/kernel/stall_test.go
git commit -m "$(cat <<'EOF'
test(gormes/kernel): prove replace-latest mailbox invariant under stalled TUI

Green kernel-discipline test. Scripts 1000 tokens via MockClient,
intentionally does NOT drain k.Render() for 2 seconds, then peeks
the capacity-1 mailbox and asserts the frame is the LATEST state
(Idle with full assistant history, OR a very-late streaming frame
with the full draft).

A stale mid-stream frame = failure = kernel blocked on emit,
which would mean the replace-latest drain-then-send in emitFrame
broke.

Treats the kernel as a black box via the render channel only —
no test-only accessors on production types. If a future refactor
ever changes the render mailbox from capacity-1 + drain-then-send
to an unbuffered or blocking channel, this test deadlocks and the
5-second timeout fires.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Verification Sweep + Doc Update

**Files:**
- No new files. Updates the Phase-1 success-criteria picture.

- [ ] **Step 1:** Run the full repo sweep and capture output for the commit.

```bash
cd gormes
go test ./... -v -timeout 60s 2>&1 | tail -60
go test -race ./... -timeout 60s 2>&1 | tail -15
go vet ./...
```

All PASS. No DATA RACE. `vet` clean.

- [ ] **Step 2:** Run the compat script locally (may pass, fail, or be unavailable depending on host).

```bash
./gormes/scripts/check-go1.22-compat.sh || echo "exit=$?"
```

Record the outcome.

- [ ] **Step 3:** Confirm the red test skips visibly.

```bash
go test ./internal/kernel/... -v -run TestKernel_HandlesMidStreamNetworkDrop | grep -E "(SKIP|PASS)"
```

Expected: `--- SKIP: TestKernel_HandlesMidStreamNetworkDrop`.

- [ ] **Step 4:** Nothing to commit here — the sweep is read-only. This task closes the plan; subsequent work is the Route-B implementation plan.

---

## Appendix A: Self-Review

**Spec coverage:**
- §3.1 compat script with symbol-identification → Task 1 (log-parses `requires go1.XX` + `undefined:`).
- §3.2 PhaseReconnecting enum value → Task 2.
- §3.3 red chaos test → Task 3 (ships t.Skip'd per §2 locked decision).
- §3.4 green stall test → Task 4 (asserts replace-latest invariant with mid-stream-frame staleness FAIL case).
- §4 success criteria → Task 5 verifies each programmatic item.
- §5 out-of-scope → honored (no Route-B impl in any task).
- §6 risks → acknowledged (`stableProxy` helper shipped as dead code for future use, 2s sleep is acceptable slack).

**Placeholder scan:** `newRealKernelFromURL` in `reconnect_helpers_test.go` returns `nil` with a comment — it's a deliberate future-use stub, never called because the red test is `t.Skip`'d. Acceptable scoping decision; an alternative (delete the stub) is equally correct. Not a placeholder failure.

**Type consistency:** `PhaseReconnecting` is referenced only in comments in this plan (Task 3's test body is Skip'd); type alignment with `Phase` enum is trivial. `RenderFrame` and `PlatformEvent` names match kernel.go. `hermes.Event`, `hermes.Message`, `hermes.NewMockClient` — all match existing exports.

**Scope:** One cohesive plan; 5 tasks; each produces a self-contained commit that advances the Phase-1.5 readiness state.
