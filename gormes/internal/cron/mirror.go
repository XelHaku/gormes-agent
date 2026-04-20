package cron

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MirrorConfig holds the live deps + rendering knobs.
type MirrorConfig struct {
	JobStore *Store
	RunStore *RunStore
	Path     string        // target file; e.g. ~/.local/share/gormes/cron/CRON.md
	Interval time.Duration // default 30s when <= 0
}

func (c *MirrorConfig) withDefaults() {
	if c.Interval <= 0 {
		c.Interval = 30 * time.Second
	}
}

// Mirror writes a human-readable Markdown snapshot of the cron state.
// Mirrors the Phase 3.D.5 USER.md pattern: background goroutine,
// atomic temp-file + rename, no partial reads.
type Mirror struct {
	cfg MirrorConfig
	log *slog.Logger
}

func NewMirror(cfg MirrorConfig, log *slog.Logger) *Mirror {
	cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Mirror{cfg: cfg, log: log}
}

// Run blocks until ctx is cancelled. Writes on start + then every
// Interval.
func (m *Mirror) Run(ctx context.Context) {
	m.tick(ctx)
	t := time.NewTicker(m.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.tick(ctx)
		}
	}
}

func (m *Mirror) tick(ctx context.Context) {
	body, err := m.render(ctx)
	if err != nil {
		m.log.Warn("cron mirror: render failed", "err", err)
		return
	}
	if err := atomicWrite(m.cfg.Path, body); err != nil {
		m.log.Warn("cron mirror: write failed", "path", m.cfg.Path, "err", err)
	}
}

func (m *Mirror) render(ctx context.Context) (string, error) {
	jobs, err := m.cfg.JobStore.List()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintln(&b, "# Gormes Cron")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "_Last refreshed: %s_\n\n", time.Now().UTC().Format(time.RFC3339))

	var active, paused []Job
	for _, j := range jobs {
		if j.Paused {
			paused = append(paused, j)
		} else {
			active = append(active, j)
		}
	}

	fmt.Fprintf(&b, "## Active Jobs (%d)\n\n", len(active))
	for _, j := range active {
		renderJob(&b, &j, m.cfg.RunStore, ctx)
	}

	if len(paused) > 0 {
		fmt.Fprintf(&b, "\n## Paused Jobs (%d)\n\n", len(paused))
		for _, j := range paused {
			renderJob(&b, &j, m.cfg.RunStore, ctx)
		}
	}

	return b.String(), nil
}

func renderJob(b *strings.Builder, j *Job, rs *RunStore, ctx context.Context) {
	fmt.Fprintf(b, "### %s — `%s`\n", j.Name, j.Schedule)
	fmt.Fprintf(b, "- **ID:** `%s`\n", j.ID)
	fmt.Fprintf(b, "- **Prompt:** %s\n", oneLine(j.Prompt, 140))
	if j.LastRunUnix > 0 {
		ts := time.Unix(j.LastRunUnix, 0).UTC().Format(time.RFC3339)
		fmt.Fprintf(b, "- **Last run:** %s — %s\n", ts, j.LastStatus)
	} else {
		fmt.Fprintln(b, "- **Last run:** _never_")
	}

	runs, err := rs.LatestRuns(ctx, j.ID, 3)
	if err == nil && len(runs) > 0 {
		fmt.Fprintln(b, "- **Recent:**")
		for _, r := range runs {
			ts := time.Unix(r.StartedAt, 0).UTC().Format(time.RFC3339)
			preview := oneLine(r.OutputPreview, 80)
			if preview == "" {
				preview = "—"
			}
			fmt.Fprintf(b, "  - %s — %s (delivered=%v) %s\n",
				ts, r.Status, r.Delivered, preview)
		}
	}
	fmt.Fprintln(b)
}

func oneLine(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// atomicWrite writes body to path via a temp file in the same dir,
// then renames. Readers never see a partial file.
func atomicWrite(path, body string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op if Rename succeeded
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
