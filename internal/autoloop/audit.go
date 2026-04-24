package autoloop

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func DigestLedger(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return formatDigest(map[string]int{}), nil
		}
		return "", err
	}
	defer file.Close()

	counts := map[string]int{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event LedgerEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return "", err
		}
		counts[event.Event]++
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return formatDigest(counts), nil
}

func formatDigest(counts map[string]int) string {
	return fmt.Sprintf(
		"runs: %d\nclaimed: %d\nsuccess: %d\npromoted: %d\n",
		counts["run_started"],
		counts["worker_claimed"],
		counts["worker_success"],
		counts["worker_promoted"],
	)
}

type AuditReportOptions struct {
	LedgerPath string
	AuditDir   string
	Now        time.Time
	Lookback   time.Duration
}

type auditReportLine struct {
	TS         string            `json:"ts"`
	CursorFrom string            `json:"cursor_from"`
	CursorTo   string            `json:"cursor_to"`
	Service    auditServiceBlock `json:"service"`
	Summary    auditSummary      `json:"summary"`
	Cost       auditCostBlock    `json:"cost"`
	Extra      map[string]string `json:"extra,omitempty"`
}

type auditServiceBlock struct {
	Active        string `json:"active"`
	UptimeSeconds int    `json:"uptime_seconds"`
	NRestarts     int    `json:"nrestarts"`
}

type auditCostBlock struct {
	TokensTotalEstimated int `json:"tokens_total_estimated"`
	DollarsEstimated     int `json:"dollars_estimated"`
}

type auditSummary struct {
	EventsSinceCursor     int            `json:"events_since_cursor"`
	RunStarted            int            `json:"run_started"`
	RunCompleted          int            `json:"run_completed"`
	WorkerClaimed         int            `json:"worker_claimed"`
	WorkerSuccess         int            `json:"worker_success"`
	WorkerFailed          int            `json:"worker_failed"`
	WorkerPromoted        int            `json:"worker_promoted"`
	WorkerPromotionFailed int            `json:"worker_promotion_failed"`
	FailStatusBreakdown   map[string]int `json:"fail_status_breakdown"`
	LastEventTS           string         `json:"last_event_ts,omitempty"`
	LastEvent             string         `json:"last_event,omitempty"`
	LastEventDetail       string         `json:"last_event_detail,omitempty"`
}

const auditCSVHeader = "ts,active,uptime_s,nrestarts,claimed,success,failed,promoted,cherry_pick_failed,productivity_pct,integration_head_short,tokens_total_estimated,dollars_estimated"

func WriteAuditReport(opts AuditReportOptions) (string, error) {
	if opts.AuditDir == "" {
		return "", fmt.Errorf("audit dir is required")
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lookback := opts.Lookback
	if lookback == 0 {
		lookback = 20 * time.Minute
	}

	if err := os.MkdirAll(opts.AuditDir, 0o755); err != nil {
		return "", err
	}

	cursorPath := filepath.Join(opts.AuditDir, "cursor")
	cursor := now.Add(-lookback).UTC()
	if raw, err := os.ReadFile(cursorPath); err == nil {
		if parsed, parseErr := time.Parse(time.RFC3339, stringTrimSpace(raw)); parseErr == nil {
			cursor = parsed
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	summary, newCursor, err := summarizeLedgerSince(opts.LedgerPath, cursor, now)
	if err != nil {
		return "", err
	}

	ts := now.UTC().Format(time.RFC3339)
	cursorFrom := cursor.UTC().Format(time.RFC3339)
	cursorTo := newCursor.UTC().Format(time.RFC3339)
	line := auditReportLine{
		TS:         ts,
		CursorFrom: cursorFrom,
		CursorTo:   cursorTo,
		Service:    auditServiceBlock{Active: "unknown"},
		Summary:    summary,
		Cost:       auditCostBlock{},
	}
	rawLine, err := json.Marshal(line)
	if err != nil {
		return "", err
	}

	if err := appendLine(filepath.Join(opts.AuditDir, "report.ndjson"), string(rawLine)); err != nil {
		return "", err
	}
	if err := writeAuditCSV(filepath.Join(opts.AuditDir, "report.csv"), line); err != nil {
		return "", err
	}
	if err := os.WriteFile(cursorPath, []byte(cursorTo+"\n"), 0o644); err != nil {
		return "", err
	}

	return formatAuditSummary(line), nil
}

func summarizeLedgerSince(path string, cursor time.Time, now time.Time) (auditSummary, time.Time, error) {
	summary := auditSummary{FailStatusBreakdown: map[string]int{}}
	newCursor := now.UTC()
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return summary, newCursor, nil
		}
		return summary, newCursor, err
	}
	defer file.Close()

	var lastSeen time.Time
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event LedgerEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return summary, newCursor, err
		}
		if event.TS.IsZero() || !event.TS.After(cursor) {
			continue
		}
		summary.EventsSinceCursor++
		switch event.Event {
		case "run_started":
			summary.RunStarted++
		case "run_completed":
			summary.RunCompleted++
		case "worker_claimed":
			summary.WorkerClaimed++
		case "worker_success":
			summary.WorkerSuccess++
		case "worker_failed":
			summary.WorkerFailed++
			status := event.Status
			if status == "" {
				status = "unknown"
			}
			summary.FailStatusBreakdown[status]++
		case "worker_promoted":
			summary.WorkerPromoted++
		case "worker_promotion_failed":
			summary.WorkerPromotionFailed++
		}
		summary.LastEventTS = event.TS.UTC().Format(time.RFC3339)
		summary.LastEvent = event.Event
		summary.LastEventDetail = event.Task
		if event.TS.After(lastSeen) {
			lastSeen = event.TS.UTC()
		}
	}
	if err := scanner.Err(); err != nil {
		return summary, newCursor, err
	}
	if !lastSeen.IsZero() {
		newCursor = lastSeen
	}

	return summary, newCursor, nil
}

func appendLine(path string, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, line)
	return err
}

func writeAuditCSV(path string, line auditReportLine) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(auditCSVHeader+"\n"), 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	productivity := 0
	if line.Summary.WorkerClaimed > 0 {
		productivity = line.Summary.WorkerPromoted * 100 / line.Summary.WorkerClaimed
	}
	row := fmt.Sprintf("%s,%s,%d,%d,%d,%d,%d,%d,%d,%d,%s,%d,%d",
		line.TS,
		line.Service.Active,
		line.Service.UptimeSeconds,
		line.Service.NRestarts,
		line.Summary.WorkerClaimed,
		line.Summary.WorkerSuccess,
		line.Summary.WorkerFailed,
		line.Summary.WorkerPromoted,
		line.Summary.WorkerPromotionFailed,
		productivity,
		"unknown",
		line.Cost.TokensTotalEstimated,
		line.Cost.DollarsEstimated,
	)
	return appendLine(path, row)
}

func formatAuditSummary(line auditReportLine) string {
	productivity := 0
	if line.Summary.WorkerClaimed > 0 {
		productivity = line.Summary.WorkerPromoted * 100 / line.Summary.WorkerClaimed
	}
	return fmt.Sprintf(`gormes-orchestrator-audit @ %s
  service:      %s  uptime=%ds  nrestarts=%d
  window:       %s -> %s
  events:       total=%d  run_started=%d
  workers:      claimed=%d  success=%d  failed=%d  promoted=%d  cherry_pick_failed=%d
  productivity: %d%% of claims landed this window
  cost (window):   tokens=%d  dollars=$%d
  last ledger:  %s @ %s
`,
		line.TS,
		line.Service.Active,
		line.Service.UptimeSeconds,
		line.Service.NRestarts,
		line.CursorFrom,
		line.CursorTo,
		line.Summary.EventsSinceCursor,
		line.Summary.RunStarted,
		line.Summary.WorkerClaimed,
		line.Summary.WorkerSuccess,
		line.Summary.WorkerFailed,
		line.Summary.WorkerPromoted,
		line.Summary.WorkerPromotionFailed,
		productivity,
		line.Cost.TokensTotalEstimated,
		line.Cost.DollarsEstimated,
		valueOr(line.Summary.LastEvent, "none"),
		valueOr(line.Summary.LastEventTS, "none"),
	)
}

func stringTrimSpace(raw []byte) string {
	for len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\n' || raw[0] == '\r' || raw[0] == '\t') {
		raw = raw[1:]
	}
	for len(raw) > 0 {
		last := raw[len(raw)-1]
		if last != ' ' && last != '\n' && last != '\r' && last != '\t' {
			break
		}
		raw = raw[:len(raw)-1]
	}
	return string(raw)
}

func valueOr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
