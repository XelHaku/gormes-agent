package autoloop

import (
	"encoding/json"
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

func TestDigestLedgerMissingLedgerReturnsZeroCounts(t *testing.T) {
	digest, err := DigestLedger(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil {
		t.Fatalf("DigestLedger() error = %v", err)
	}
	if !strings.Contains(digest, "runs: 0") {
		t.Fatalf("DigestLedger() = %q, want zero-count digest", digest)
	}
}

func TestAuditReportCreatesCursorReportCSVAndSummary(t *testing.T) {
	tmp := t.TempDir()
	ledgerPath := filepath.Join(tmp, "runs", "state", "runs.jsonl")
	auditDir := filepath.Join(tmp, "audit")
	events := []LedgerEvent{
		{TS: time.Unix(10, 0).UTC(), Event: "run_started"},
		{TS: time.Unix(11, 0).UTC(), Event: "worker_claimed"},
		{TS: time.Unix(12, 0).UTC(), Event: "worker_success"},
		{TS: time.Unix(13, 0).UTC(), Event: "worker_promoted"},
		{TS: time.Unix(14, 0).UTC(), Event: "worker_failed", Status: "poisoned", Task: "task@branch"},
	}
	for _, event := range events {
		if err := AppendLedgerEvent(ledgerPath, event); err != nil {
			t.Fatalf("AppendLedgerEvent() error = %v", err)
		}
	}

	summary, err := WriteAuditReport(AuditReportOptions{
		LedgerPath: ledgerPath,
		AuditDir:   auditDir,
		Now:        time.Unix(20, 0).UTC(),
		Lookback:   time.Hour,
	})
	if err != nil {
		t.Fatalf("WriteAuditReport() error = %v", err)
	}

	for _, want := range []string{
		"gormes-orchestrator-audit @ 1970-01-01T00:00:20Z",
		"events:       total=5  run_started=1",
		"workers:      claimed=1  success=1  failed=1  promoted=1",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary = %q, want %q", summary, want)
		}
	}

	cursor, err := os.ReadFile(filepath.Join(auditDir, "cursor"))
	if err != nil {
		t.Fatalf("ReadFile(cursor) error = %v", err)
	}
	if got, want := strings.TrimSpace(string(cursor)), "1970-01-01T00:00:14Z"; got != want {
		t.Fatalf("cursor = %q, want %q", got, want)
	}

	report, err := os.ReadFile(filepath.Join(auditDir, "report.ndjson"))
	if err != nil {
		t.Fatalf("ReadFile(report.ndjson) error = %v", err)
	}
	var line struct {
		Summary struct {
			EventsSinceCursor int `json:"events_since_cursor"`
			WorkerFailed      int `json:"worker_failed"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(report))), &line); err != nil {
		t.Fatalf("Unmarshal(report) error = %v", err)
	}
	if line.Summary.EventsSinceCursor != 5 || line.Summary.WorkerFailed != 1 {
		t.Fatalf("report summary = %+v, want event counts", line.Summary)
	}

	csv, err := os.ReadFile(filepath.Join(auditDir, "report.csv"))
	if err != nil {
		t.Fatalf("ReadFile(report.csv) error = %v", err)
	}
	for _, want := range []string{
		"ts,active,uptime_s,nrestarts,claimed,success,failed,promoted,cherry_pick_failed,productivity_pct,integration_head_short,tokens_total_estimated,dollars_estimated",
		"1970-01-01T00:00:20Z,unknown,0,0,1,1,1,1,0,100,unknown,0,0",
	} {
		if !strings.Contains(string(csv), want) {
			t.Fatalf("csv = %q, want %q", csv, want)
		}
	}
}

func TestAuditReportMissingLedgerCreatesZeroCountArtifacts(t *testing.T) {
	auditDir := filepath.Join(t.TempDir(), "audit")

	summary, err := WriteAuditReport(AuditReportOptions{
		LedgerPath: filepath.Join(t.TempDir(), "missing.jsonl"),
		AuditDir:   auditDir,
		Now:        time.Unix(20, 0).UTC(),
		Lookback:   time.Hour,
	})
	if err != nil {
		t.Fatalf("WriteAuditReport() error = %v", err)
	}
	if !strings.Contains(summary, "events:       total=0  run_started=0") {
		t.Fatalf("summary = %q, want zero counts", summary)
	}
	for _, name := range []string{"cursor", "report.ndjson", "report.csv"} {
		if _, err := os.Stat(filepath.Join(auditDir, name)); err != nil {
			t.Fatalf("Stat(%s) error = %v", name, err)
		}
	}
}

func TestDigestLedgerEmptyLedgerReturnsZeroCounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	digest, err := DigestLedger(path)
	if err != nil {
		t.Fatalf("DigestLedger() error = %v", err)
	}

	for _, want := range []string{
		"runs: 0",
		"claimed: 0",
		"success: 0",
		"promoted: 0",
	} {
		if !strings.Contains(digest, want) {
			t.Fatalf("DigestLedger() = %q, want line containing %q", digest, want)
		}
	}
}
