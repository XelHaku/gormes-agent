// Package cron is the Phase 2.D proactive scheduler. Jobs stored in
// bbolt; per-run audit rows in SQLite; agent turns isolated via an
// ephemeral session id per fire. See spec at
// docs/superpowers/specs/2026-04-20-gormes-phase2d-cron-design.md.
package cron

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	rc "github.com/robfig/cron/v3"
)

// Job is a scheduled agent prompt. Persisted as a JSON blob under its
// ID as key in the cron_jobs bbolt bucket.
type Job struct {
	ID          string `json:"id"`            // 16-byte random hex — unique within one DB
	Name        string `json:"name"`          // operator-friendly label; must be unique
	Schedule    string `json:"schedule"`      // cron expression or @shortcut; validated via ValidateSchedule
	Prompt      string `json:"prompt"`        // user-facing prompt, WITHOUT the [SYSTEM:] prefix
	Paused      bool   `json:"paused"`        // default false; if true, scheduler ignores
	CreatedAt   int64  `json:"created_at"`    // unix seconds
	LastRunUnix int64  `json:"last_run_unix"` // 0 when never run
	LastStatus  string `json:"last_status"`   // "success"|"timeout"|"error"|"suppressed"|""
}

// NewJob constructs a Job with a fresh random ID and the current time
// as CreatedAt. The caller still needs to validate the schedule and
// call Store.Create.
func NewJob(name, schedule, prompt string) Job {
	return Job{
		ID:        newID(),
		Name:      name,
		Schedule:  schedule,
		Prompt:    prompt,
		Paused:    false,
		CreatedAt: time.Now().Unix(),
	}
}

// ValidateSchedule parses the cron expression via robfig/cron/v3's
// standard parser, returning a typed error on rejection. Accepts
// 5-field standard cron and @shortcut forms (@daily, @hourly,
// @every 30m, etc.).
func ValidateSchedule(expr string) error {
	if expr == "" {
		return fmt.Errorf("cron: schedule is empty")
	}
	_, err := rc.ParseStandard(expr)
	if err != nil {
		return fmt.Errorf("cron: invalid schedule %q: %w", expr, err)
	}
	return nil
}

// newID generates a 16-byte (32-hex-char) random ID. Not ULID, not
// UUID — we don't need the timestamp encoding, just uniqueness within
// one bbolt file. crypto/rand is stdlib so no new deps.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
