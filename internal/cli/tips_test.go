package cli

import (
	"strings"
	"testing"
)

func TestTips_CorpusInvariants(t *testing.T) {
	if len(Tips) < 30 {
		t.Fatalf("len(Tips) = %d, want >= 30", len(Tips))
	}

	seen := make(map[string]struct{}, len(Tips))
	for i, tip := range Tips {
		if tip == "" {
			t.Errorf("Tips[%d] is empty", i)
			continue
		}
		if strings.Contains(tip, "\n") {
			t.Errorf("Tips[%d] contains a newline: %q", i, tip)
		}
		if _, ok := seen[tip]; ok {
			t.Errorf("Tips[%d] duplicates an earlier entry: %q", i, tip)
		}
		seen[tip] = struct{}{}
	}
}

func TestTipFor_DeterministicForFixedSeed(t *testing.T) {
	first := TipFor(42)
	second := TipFor(42)
	if first != second {
		t.Fatalf("TipFor(42) returned %q then %q; want stable output", first, second)
	}

	want := Tips[42%int64(len(Tips))]
	if first != want {
		t.Fatalf("TipFor(42) = %q, want Tips[42 mod len(Tips)] = %q", first, want)
	}
}

func TestTipFor_HandlesNegativeSeed(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("TipFor(-1) panicked: %v", r)
		}
	}()

	got := TipFor(-1)
	if got == "" {
		t.Fatalf("TipFor(-1) returned empty string")
	}

	found := false
	for _, tip := range Tips {
		if tip == got {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("TipFor(-1) = %q, which is not present in Tips", got)
	}
}

func TestTipFor_HandlesZeroSeed(t *testing.T) {
	if got := TipFor(0); got != Tips[0] {
		t.Fatalf("TipFor(0) = %q, want Tips[0] = %q", got, Tips[0])
	}
}

func TestTipFor_DistributesAcrossCorpus(t *testing.T) {
	counts := make(map[string]int, len(Tips))
	for i := 0; i < len(Tips); i++ {
		counts[TipFor(int64(i))]++
	}

	for _, tip := range Tips {
		switch counts[tip] {
		case 1:
			// expected
		case 0:
			t.Errorf("tip %q was never returned by TipFor(int64(i)) for i in [0, len(Tips))", tip)
		default:
			t.Errorf("tip %q was returned %d times; want exactly 1", tip, counts[tip])
		}
	}
}
