package autoloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDigestLedgerCountsLastEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	events := []LedgerEvent{
		{TS: time.Unix(1, 0).UTC(), Event: "run_started"},
		{TS: time.Unix(2, 0).UTC(), Event: "worker_claimed"},
		{TS: time.Unix(3, 0).UTC(), Event: "worker_claimed"},
		{TS: time.Unix(4, 0).UTC(), Event: "worker_success"},
		{TS: time.Unix(5, 0).UTC(), Event: "worker_promoted"},
		{TS: time.Unix(6, 0).UTC(), Event: "worker_promoted"},
		{TS: time.Unix(7, 0).UTC(), Event: "worker_promoted"},
		{TS: time.Unix(8, 0).UTC(), Event: "worker_failed"},
	}

	for _, event := range events {
		if err := AppendLedgerEvent(path, event); err != nil {
			t.Fatalf("AppendLedgerEvent() error = %v", err)
		}
	}

	digest, err := DigestLedger(path)
	if err != nil {
		t.Fatalf("DigestLedger() error = %v", err)
	}

	for _, want := range []string{
		"runs: 1",
		"claimed: 2",
		"success: 1",
		"promoted: 3",
	} {
		if !strings.Contains(digest, want) {
			t.Fatalf("DigestLedger() = %q, want line containing %q", digest, want)
		}
	}

	if err := os.WriteFile(path, []byte("{bad-json}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := DigestLedger(path); err == nil {
		t.Fatal("DigestLedger() error = nil, want parse error")
	}
}
