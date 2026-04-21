// gormes/internal/subagent/ids_test.go
package subagent

import (
	"strings"
	"testing"
)

func TestNewSubagentIDPrefix(t *testing.T) {
	id := newSubagentID()
	if !strings.HasPrefix(id, "sa_") {
		t.Errorf("newSubagentID: want prefix %q, got %q", "sa_", id)
	}
}

func TestNewSubagentIDLengthAndCharset(t *testing.T) {
	id := newSubagentID()
	body := strings.TrimPrefix(id, "sa_")
	// 8 bytes → ceil(8*8 / 5) = 13 base32 chars (no padding).
	if len(body) != 13 {
		t.Errorf("newSubagentID body length: want 13, got %d (id=%q)", len(body), id)
	}
	for _, r := range body {
		if !((r >= 'A' && r <= 'Z') || (r >= '2' && r <= '7')) {
			t.Errorf("newSubagentID body charset: want base32 (A-Z, 2-7), got %q in %q", r, id)
		}
	}
}

func TestNewSubagentIDUniqueness(t *testing.T) {
	const N = 1000
	seen := make(map[string]struct{}, N)
	for i := 0; i < N; i++ {
		id := newSubagentID()
		if _, dup := seen[id]; dup {
			t.Fatalf("newSubagentID collision after %d calls: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}
