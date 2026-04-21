// Package progress is the single source of truth for Gormes roadmap progress.
// It parses progress.json, derives phase/subphase status from items, and
// renders the canonical markdown sections consumed by README and docs.
package progress

import (
	"encoding/json"
	"fmt"
	"os"
)

type Status string

const (
	StatusComplete   Status = "complete"
	StatusInProgress Status = "in_progress"
	StatusPlanned    Status = "planned"
)

type Meta struct {
	Version     string            `json:"version"`
	LastUpdated string            `json:"last_updated"`
	Links       map[string]string `json:"links"`
}

type Item struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	// Optional, reserved, not rendered yet.
	PR    string `json:"pr,omitempty"`
	Owner string `json:"owner,omitempty"`
	ETA   string `json:"eta,omitempty"`
	Note  string `json:"note,omitempty"`
}

type Subphase struct {
	Name     string `json:"name"`
	Priority string `json:"priority,omitempty"`
	// Exactly one of Items or Status is set. Enforced by Validate.
	Items  []Item `json:"items,omitempty"`
	Status Status `json:"status,omitempty"`
}

type Phase struct {
	Name        string `json:"name"`
	Deliverable string `json:"deliverable"`
	// DependencyNote is a free-form string on some phases.
	DependencyNote string              `json:"dependency_note,omitempty"`
	Subphases      map[string]Subphase `json:"subphases"`
}

type Progress struct {
	Meta   Meta             `json:"meta"`
	Phases map[string]Phase `json:"phases"`
}

// DerivedStatus computes subphase status.
// If explicit Status is set (and no items), returns it.
// Otherwise: all items complete -> complete; any complete or in_progress -> in_progress; else planned.
// Validate guarantees exactly one of Items or Status is set.
func (s Subphase) DerivedStatus() Status {
	if len(s.Items) == 0 {
		return s.Status
	}
	allComplete := true
	anyStarted := false
	for _, it := range s.Items {
		if it.Status != StatusComplete {
			allComplete = false
		}
		if it.Status == StatusComplete || it.Status == StatusInProgress {
			anyStarted = true
		}
	}
	switch {
	case allComplete:
		return StatusComplete
	case anyStarted:
		return StatusInProgress
	default:
		return StatusPlanned
	}
}

// DerivedStatus computes phase status from subphases. Empty phase -> planned.
func (ph Phase) DerivedStatus() Status {
	if len(ph.Subphases) == 0 {
		return StatusPlanned
	}
	allComplete := true
	anyStarted := false
	for _, sp := range ph.Subphases {
		st := sp.DerivedStatus()
		if st != StatusComplete {
			allComplete = false
		}
		if st == StatusComplete || st == StatusInProgress {
			anyStarted = true
		}
	}
	switch {
	case allComplete:
		return StatusComplete
	case anyStarted:
		return StatusInProgress
	default:
		return StatusPlanned
	}
}

// Load reads and parses progress.json from the given path.
func Load(path string) (*Progress, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("progress: read %s: %w", path, err)
	}
	var p Progress
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("progress: parse %s: %w", path, err)
	}
	return &p, nil
}

type Counts struct {
	Total      int
	Complete   int
	InProgress int
	Planned    int
}

type Stats struct {
	Phases    Counts
	Subphases Counts
	Items     Counts
}

// Stats walks all phases/subphases/items and tallies derived status.
// Computed on demand — never stored in progress.json.
func (p *Progress) Stats() Stats {
	var s Stats
	for _, ph := range p.Phases {
		s.Phases.Total++
		tally(&s.Phases, ph.DerivedStatus())
		for _, sp := range ph.Subphases {
			s.Subphases.Total++
			tally(&s.Subphases, sp.DerivedStatus())
			for _, it := range sp.Items {
				s.Items.Total++
				tally(&s.Items, it.Status)
			}
		}
	}
	return s
}

func tally(c *Counts, st Status) {
	switch st {
	case StatusComplete:
		c.Complete++
	case StatusInProgress:
		c.InProgress++
	case StatusPlanned:
		c.Planned++
	}
}
