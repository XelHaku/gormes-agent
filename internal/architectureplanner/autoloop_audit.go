package architectureplanner

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

// AutoloopAudit summarises recent autoloop ledger activity so the planner can
// see which subphases keep failing, claiming-without-promoting, or otherwise
// signalling that a row needs to be split or re-specified.
type AutoloopAudit struct {
	LedgerPath        string                  `json:"ledger_path"`
	WindowStartUTC    string                  `json:"window_start_utc"`
	WindowEndUTC      string                  `json:"window_end_utc"`
	Runs              int                     `json:"runs"`
	Claimed           int                     `json:"claimed"`
	Succeeded         int                     `json:"succeeded"`
	Promoted          int                     `json:"promoted"`
	PromotionFailed   int                     `json:"promotion_failed"`
	Failed            int                     `json:"failed"`
	FailStatusCounts  map[string]int          `json:"fail_status_counts,omitempty"`
	ToxicSubphases    []SubphaseAuditRow      `json:"toxic_subphases,omitempty"`
	HotSubphases      []SubphaseAuditRow      `json:"hot_subphases,omitempty"`
	RecentFailedTasks []TaskAuditRow          `json:"recent_failed_tasks,omitempty"`
	subphaseStats     map[string]*subphaseAcc `json:"-"`
}

type SubphaseAuditRow struct {
	SubphaseID      string `json:"subphase_id"`
	Claimed         int    `json:"claimed"`
	Succeeded       int    `json:"succeeded"`
	Promoted        int    `json:"promoted"`
	Failed          int    `json:"failed"`
	PromotionFailed int    `json:"promotion_failed"`
}

type TaskAuditRow struct {
	TS         string `json:"ts"`
	SubphaseID string `json:"subphase_id"`
	Task       string `json:"task"`
	Status     string `json:"status"`
}

type subphaseAcc struct {
	SubphaseID      string
	Claimed         int
	Succeeded       int
	Promoted        int
	Failed          int
	PromotionFailed int
}

// SummarizeAutoloopAudit reads the autoloop runs ledger and produces a summary
// for the planner. It is safe to call when the ledger is missing — the summary
// will simply report zero activity.
func SummarizeAutoloopAudit(ledgerPath string, window time.Duration, now time.Time) (AutoloopAudit, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	cutoff := now.Add(-window).UTC()
	audit := AutoloopAudit{
		LedgerPath:       ledgerPath,
		WindowStartUTC:   cutoff.Format(time.RFC3339),
		WindowEndUTC:     now.UTC().Format(time.RFC3339),
		FailStatusCounts: map[string]int{},
		subphaseStats:    map[string]*subphaseAcc{},
	}
	if ledgerPath == "" {
		return audit, nil
	}

	file, err := os.Open(ledgerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return audit, nil
		}
		return audit, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<22)
	for scanner.Scan() {
		var event autoloop.LedgerEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if !event.TS.IsZero() && event.TS.Before(cutoff) {
			continue
		}
		switch event.Event {
		case "run_started":
			audit.Runs++
		case "worker_claimed":
			audit.Claimed++
			subphaseAccountFor(audit.subphaseStats, event.Task).Claimed++
		case "worker_success":
			audit.Succeeded++
			subphaseAccountFor(audit.subphaseStats, event.Task).Succeeded++
		case "worker_promoted":
			audit.Promoted++
			subphaseAccountFor(audit.subphaseStats, event.Task).Promoted++
		case "worker_promotion_failed":
			audit.PromotionFailed++
			subphaseAccountFor(audit.subphaseStats, event.Task).PromotionFailed++
		case "worker_failed":
			audit.Failed++
			subphaseAccountFor(audit.subphaseStats, event.Task).Failed++
			status := strings.TrimSpace(event.Status)
			if status == "" {
				status = "unknown"
			}
			audit.FailStatusCounts[status]++
			audit.RecentFailedTasks = append(audit.RecentFailedTasks, TaskAuditRow{
				TS:         event.TS.UTC().Format(time.RFC3339),
				SubphaseID: subphaseFromTask(event.Task),
				Task:       event.Task,
				Status:     status,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return audit, err
	}

	rows := make([]SubphaseAuditRow, 0, len(audit.subphaseStats))
	for _, acc := range audit.subphaseStats {
		rows = append(rows, SubphaseAuditRow{
			SubphaseID:      acc.SubphaseID,
			Claimed:         acc.Claimed,
			Succeeded:       acc.Succeeded,
			Promoted:        acc.Promoted,
			Failed:          acc.Failed,
			PromotionFailed: acc.PromotionFailed,
		})
	}

	toxic := make([]SubphaseAuditRow, 0, len(rows))
	for _, row := range rows {
		if row.Failed > 0 || row.PromotionFailed > 0 {
			toxic = append(toxic, row)
		}
	}
	sort.SliceStable(toxic, func(i, j int) bool {
		li := toxic[i].Failed*10 + toxic[i].PromotionFailed
		lj := toxic[j].Failed*10 + toxic[j].PromotionFailed
		if li != lj {
			return li > lj
		}
		return toxic[i].SubphaseID < toxic[j].SubphaseID
	})
	if len(toxic) > 8 {
		toxic = toxic[:8]
	}
	audit.ToxicSubphases = toxic

	hot := append([]SubphaseAuditRow(nil), rows...)
	sort.SliceStable(hot, func(i, j int) bool {
		if hot[i].Claimed != hot[j].Claimed {
			return hot[i].Claimed > hot[j].Claimed
		}
		return hot[i].SubphaseID < hot[j].SubphaseID
	})
	if len(hot) > 8 {
		hot = hot[:8]
	}
	audit.HotSubphases = hot

	if len(audit.RecentFailedTasks) > 12 {
		audit.RecentFailedTasks = audit.RecentFailedTasks[len(audit.RecentFailedTasks)-12:]
	}

	return audit, nil
}

func subphaseAccountFor(stats map[string]*subphaseAcc, task string) *subphaseAcc {
	id := subphaseFromTask(task)
	acc, ok := stats[id]
	if !ok {
		acc = &subphaseAcc{SubphaseID: id}
		stats[id] = acc
	}
	return acc
}

func subphaseFromTask(task string) string {
	parts := strings.SplitN(task, "/", 3)
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[0]) + "/" + strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(task)
}

// ProductivityPercent returns the percentage of claims that landed (promoted)
// during the audit window, or 0 when the window is empty.
func (a AutoloopAudit) ProductivityPercent() int {
	if a.Claimed == 0 {
		return 0
	}
	return a.Promoted * 100 / a.Claimed
}
