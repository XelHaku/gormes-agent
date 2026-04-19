package kernel

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
)

// TestKernel_HandlesMidStreamNetworkDrop exercises the Route-B reconnect
// feature (spec §9.2 of 2026-04-18-gormes-frontend-adapter-design.md).
//
// Asserts the four invariants:
//
//  1. PhaseReconnecting transition after TCP drop (within 500ms).
//  2. Draft preserved during the reconnect window (5 tokens stay visible).
//  3. Automatic recovery back to PhaseStreaming then PhaseIdle after backoff.
//  4. Final history contains exactly ONE clean assistant message from the
//     successful retry (no Frankenstein concatenation).
func TestKernel_HandlesMidStreamNetworkDrop(t *testing.T) {
	srv1 := httptest.NewServer(fiveTokenHandler())
	defer srv1.Close()

	proxy := newStableProxy(t)
	defer proxy.Close()
	proxy.Rebind(srv1.URL)

	k := newRealKernel(t, proxy.URL())

	// Run kernel in a goroutine with a generous timeout covering worst-case
	// backoff (1+2+4+8+16 = 31s) plus two streams of 10 tokens each.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	go k.Run(ctx)

	// Drain initial idle frame.
	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial = %v, want PhaseIdle", initial.Phase)
	}

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Wait for streaming to accumulate 5 "x" tokens.
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseStreaming && f.DraftText == "xxxxx"
	}, 3*time.Second)

	// CHAOS MONKEY: close the first server's client connections mid-stream.
	srv1.CloseClientConnections()

	// ASSERT 1: phase transitions to PhaseReconnecting within 500ms (bounded
	// by a 2s envelope to tolerate scheduler noise).
	reconnecting := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseReconnecting
	}, 2*time.Second)

	// ASSERT 2: draft still contains "xxxxx" during reconnect window.
	if reconnecting.DraftText != "xxxxx" {
		t.Errorf("draft during PhaseReconnecting = %q, want xxxxx (visual continuity)", reconnecting.DraftText)
	}

	// Bring up a second server emitting 10 "y" tokens + done, rebind the proxy.
	srv2 := httptest.NewServer(tenTokenHandler())
	defer srv2.Close()
	proxy.Rebind(srv2.URL)

	// ASSERT 3: phase returns to PhaseStreaming then PhaseIdle.
	// First new-stream frame should have draft starting with "y" (retry replaced old draft).
	// Note: coalescing may merge multiple token frames into one; an idle frame with
	// a y-only history is the terminal observation — accept either mid- or end-state.
	final := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		if f.Phase == PhaseStreaming && strings.HasPrefix(f.DraftText, "y") {
			t.Logf("observed mid-stream y-frame: seq=%d draft=%q", f.Seq, f.DraftText)
			return false // keep waiting for idle
		}
		return f.Phase == PhaseIdle && len(f.History) >= 2
	}, 30*time.Second)

	// ASSERT 4: final history has exactly one assistant message == "yyyyyyyyyy".
	var assistants []hermes.Message
	for _, m := range final.History {
		if m.Role == "assistant" {
			assistants = append(assistants, m)
		}
	}
	if len(assistants) != 1 {
		t.Fatalf("final history has %d assistant msgs, want 1", len(assistants))
	}
	if assistants[0].Content != "yyyyyyyyyy" {
		t.Errorf("assistant content = %q, want yyyyyyyyyy (retry replaces preserved draft)", assistants[0].Content)
	}
}
