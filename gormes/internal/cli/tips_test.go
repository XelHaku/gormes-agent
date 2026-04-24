package cli

import (
	"math/rand/v2"
	"testing"
)

// Tests freeze the Go port of hermes_cli/tips.py (Phase 5.O).
//
// Upstream exposes a flat corpus (TIPS, list[str]) and a pure selection
// helper (get_random_tip(exclude_recent: int = 0) -> str) that picks one
// entry uniformly via random.choice. The Go port mirrors both the corpus
// contents and the signature so callers never have to re-embed tips on
// the Go side. Determinism in tests is achieved by passing an explicit
// *math/rand/v2.Rand into PickRandomTip; GetRandomTip wraps the
// package-level source for parity with the Python global.

// expectedTipsCount pins the upstream TIPS length. Bumping this requires
// a matching edit to internal/cli/tips.go.
const expectedTipsCount = 279

// Known-identifying entries that must survive any corpus edit — these
// guard against accidental drop-outs during mechanical reformatting.
const (
	expectedFirstTip = "/btw <question> asks a quick side question without tools or history — great for clarifications."
	expectedLastTip  = "The skills quarantine at ~/.hermes/skills/.hub/quarantine/ holds skills pending security review."
	// A mid-corpus entry that names a specific config key — a useful
	// canary because it is easy to mangle during bulk edits.
	expectedMidTip = "Environment variable substitution works in config.yaml: use ${VAR_NAME} syntax."
)

func TestTips_CountMatchesUpstream(t *testing.T) {
	if got := len(Tips); got != expectedTipsCount {
		t.Fatalf("len(Tips) = %d, want %d (upstream hermes_cli.tips.TIPS)", got, expectedTipsCount)
	}
}

func TestTips_NoDuplicateEntries(t *testing.T) {
	seen := make(map[string]int, len(Tips))
	for i, tip := range Tips {
		if prev, dup := seen[tip]; dup {
			t.Fatalf("duplicate tip at indices %d and %d: %q", prev, i, tip)
		}
		seen[tip] = i
	}
}

func TestTips_NoEmptyEntries(t *testing.T) {
	for i, tip := range Tips {
		if tip == "" {
			t.Fatalf("Tips[%d] is empty", i)
		}
	}
}

func TestTips_FirstEntryPinnedToUpstream(t *testing.T) {
	if got := Tips[0]; got != expectedFirstTip {
		t.Fatalf("Tips[0] = %q, want %q", got, expectedFirstTip)
	}
}

func TestTips_LastEntryPinnedToUpstream(t *testing.T) {
	if got := Tips[len(Tips)-1]; got != expectedLastTip {
		t.Fatalf("Tips[last] = %q, want %q", got, expectedLastTip)
	}
}

func TestTips_MidCorpusCanaryPresent(t *testing.T) {
	for _, tip := range Tips {
		if tip == expectedMidTip {
			return
		}
	}
	t.Fatalf("expected canary tip %q missing from corpus", expectedMidTip)
}

func TestGetRandomTip_ReturnsMemberOfCorpus(t *testing.T) {
	members := make(map[string]struct{}, len(Tips))
	for _, tip := range Tips {
		members[tip] = struct{}{}
	}
	// Sample several times to reduce the chance that a future buggy
	// implementation passes only because of one lucky draw.
	for i := 0; i < 20; i++ {
		got := GetRandomTip(0)
		if _, ok := members[got]; !ok {
			t.Fatalf("GetRandomTip(0) = %q, not in Tips corpus", got)
		}
	}
}

func TestGetRandomTip_AcceptsExcludeRecentForParity(t *testing.T) {
	// Signature parity with upstream: exclude_recent is reserved for
	// future deduplication across sessions but does not influence the
	// current selection. The Go surface must still accept the int so
	// callers ported from Python keep compiling.
	for _, n := range []int{0, 1, 5, 100} {
		got := GetRandomTip(n)
		if got == "" {
			t.Fatalf("GetRandomTip(%d) returned empty string", n)
		}
	}
}

func TestPickRandomTip_DeterministicWithSeededRand(t *testing.T) {
	// Injecting *rand.Rand gives tests deterministic access without
	// touching the package-level source. Two independent sources with
	// the same seed must produce identical picks.
	seedA := rand.New(rand.NewPCG(42, 0xDEADBEEF))
	seedB := rand.New(rand.NewPCG(42, 0xDEADBEEF))
	for i := 0; i < 5; i++ {
		a := PickRandomTip(seedA, 0)
		b := PickRandomTip(seedB, 0)
		if a != b {
			t.Fatalf("iteration %d: seeded picks diverged: %q vs %q", i, a, b)
		}
	}
}

func TestPickRandomTip_NilRandUsesGlobalSource(t *testing.T) {
	// Passing a nil *rand.Rand must fall back to the package-level
	// source so library callers that do not care about determinism
	// behave exactly like upstream random.choice.
	members := make(map[string]struct{}, len(Tips))
	for _, tip := range Tips {
		members[tip] = struct{}{}
	}
	got := PickRandomTip(nil, 0)
	if _, ok := members[got]; !ok {
		t.Fatalf("PickRandomTip(nil, 0) = %q, not in Tips corpus", got)
	}
}
