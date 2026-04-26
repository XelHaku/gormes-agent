package cli

import (
	"bytes"
	"os"
)

// LogClass identifies which Gormes log file a snapshot section belongs to.
type LogClass string

const (
	LogClassMain      LogClass = "main"
	LogClassToolAudit LogClass = "tool_audit"
)

// LogSnapshotRoots carries the resolved log file paths supplied by the caller.
// SnapshotLogs does not resolve XDG paths itself so it stays unit-testable
// against a t.TempDir().
type LogSnapshotRoots struct {
	LogPath       string
	ToolAuditPath string
}

// SnapshotOpts controls how many bytes from the head and tail of each log
// file are kept. Zero or negative values fall back to the defaults
// (64 KiB head, 16 KiB tail).
type SnapshotOpts struct {
	HeadBytes int64
	TailBytes int64
}

// LogSection records the redacted head/tail of a single log file plus
// degraded-mode evidence flags so doctor/status can render output without
// failing when a file is missing or unreadable.
type LogSection struct {
	Class      LogClass
	Path       string
	Head       []byte
	Tail       []byte
	Missing    bool
	Truncated  bool
	Redacted   int
	Unreadable string
}

// Snapshot is the ordered collection of LogSections SnapshotLogs returns.
// The order is always [LogClassMain, LogClassToolAudit].
type Snapshot struct {
	Sections []LogSection
}

const (
	defaultHeadBytes int64 = 64 * 1024
	defaultTailBytes int64 = 16 * 1024
)

// SnapshotLogs reads the configured log files, applies head/tail truncation,
// and runs each non-empty line through RedactLine. It always returns one
// section per LogClass in the fixed order [main, tool_audit] and never
// returns an error or panics — every failure is recorded on the section as
// Missing or Unreadable so callers can keep rendering.
func SnapshotLogs(roots LogSnapshotRoots, opts SnapshotOpts) Snapshot {
	headBytes := opts.HeadBytes
	if headBytes <= 0 {
		headBytes = defaultHeadBytes
	}
	tailBytes := opts.TailBytes
	if tailBytes <= 0 {
		tailBytes = defaultTailBytes
	}

	targets := []struct {
		class LogClass
		path  string
	}{
		{LogClassMain, roots.LogPath},
		{LogClassToolAudit, roots.ToolAuditPath},
	}

	sections := make([]LogSection, 0, len(targets))
	for _, t := range targets {
		sections = append(sections, snapshotSection(t.class, t.path, headBytes, tailBytes))
	}
	return Snapshot{Sections: sections}
}

func snapshotSection(class LogClass, path string, headBytes, tailBytes int64) LogSection {
	section := LogSection{Class: class, Path: path}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			section.Missing = true
			return section
		}
		// Other stat errors fall through; ReadFile will surface a useful message.
	}

	data, err := os.ReadFile(path)
	if err != nil {
		section.Unreadable = err.Error()
		return section
	}

	size := int64(len(data))
	if size <= headBytes+tailBytes {
		section.Head = redactBlock(data, &section.Redacted)
		section.Tail = nil
		section.Truncated = false
		return section
	}

	section.Truncated = true
	section.Head = redactBlock(data[:headBytes], &section.Redacted)
	section.Tail = redactBlock(data[size-tailBytes:], &section.Redacted)
	return section
}

func redactBlock(block []byte, redacted *int) []byte {
	if len(block) == 0 {
		return block
	}
	lines := bytes.Split(block, []byte("\n"))
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}
		redactedLine, count := RedactLine(line)
		lines[i] = redactedLine
		*redacted += count
	}
	return bytes.Join(lines, []byte("\n"))
}
