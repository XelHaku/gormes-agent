// Package doctor runs diagnostic checks against a constructed Gormes runtime.
// Each Check returns a CheckResult that cmd/gormes/doctor renders to stdout.
package doctor

import (
	"fmt"
	"strings"
)

// Status enumerates the possible outcomes of a diagnostic check.
type Status int

const (
	StatusPass Status = iota
	StatusFail
	StatusWarn
)

func (s Status) String() string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusFail:
		return "FAIL"
	case StatusWarn:
		return "WARN"
	}
	return "UNKNOWN"
}

// Symbol returns a compact glyph for console output.
func (s Status) Symbol() string {
	switch s {
	case StatusPass:
		return "✓"
	case StatusFail:
		return "✗"
	case StatusWarn:
		return "!"
	}
	return "?"
}

// CheckResult is the output of one diagnostic check.
type CheckResult struct {
	Name    string // short label, e.g. "Toolbox"
	Status  Status
	Summary string     // one-line headline
	Items   []ItemInfo // optional per-entry detail
}

// ItemInfo is a per-tool (or per-entry) row rendered under the headline.
type ItemInfo struct {
	Name   string
	Status Status
	Note   string // description on pass; error detail on fail
}

// Format renders the CheckResult as a multi-line string suitable for
// `gormes doctor` stdout.
func (r CheckResult) Format() string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s: %s\n", r.Status.String(), r.Name, r.Summary)
	if len(r.Items) == 0 {
		return b.String()
	}
	// Two-column formatting: widest name + status column.
	nameW := 0
	for _, it := range r.Items {
		if n := len(it.Name); n > nameW {
			nameW = n
		}
	}
	for _, it := range r.Items {
		fmt.Fprintf(&b, "  %s %-*s  %s\n", it.Status.Symbol(), nameW, it.Name, it.Note)
	}
	return b.String()
}
