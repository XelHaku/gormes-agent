// Package progressctl owns the regeneration logic for the building-gormes
// progress control plane: validating progress.json, rewriting the markered
// docs (README rollup, agent queue, blocked slices, etc.), and mirroring
// progress.json into the www.gormes.ai data directory.
//
// The package is consumed by both cmd/progress (the dedicated binary) and
// cmd/builder-loop (which keeps its `progress` subcommand for back-compat).
// Both pass an io.Writer for human-facing output and a root string for the
// repo location; no other state is shared.
package progressctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// Validate parses progress.json under root, runs progress.Validate, and emits
// a single line summarizing the phase count. format may be "text" (default)
// or "json".
func Validate(stdout io.Writer, root, format string) error {
	p, err := loadValidProgress(root)
	if err != nil {
		return err
	}
	if format == "json" {
		return json.NewEncoder(stdout).Encode(struct {
			OK     bool `json:"ok"`
			Phases int  `json:"phases"`
		}{OK: true, Phases: len(p.Phases)})
	}
	_, err = fmt.Fprintf(stdout, "progress: validated %d phases\n", len(p.Phases))
	return err
}

// Write regenerates every markered section in the docs tree from
// progress.json and mirrors the JSON into the www.gormes.ai data directory.
// Errors from individual rewrites are collected, surfaced one per line, and
// returned via errors.Join so the caller fails the whole run while still
// telling the operator which markers updated and which did not.
func Write(stdout io.Writer, root string) error {
	p, err := loadValidProgress(root)
	if err != nil {
		return err
	}
	paths := progressPaths(root)

	markers := []marker{
		{pathOf: func(s pathSet) string { return s.docsIndex }, kind: "docs-full-checklist", label: "_index.md regenerated", render: progress.RenderDocsChecklist},
		{pathOf: func(s pathSet) string { return s.readme }, kind: "readme-rollup", label: "README.md regenerated", render: progress.RenderReadmeRollup},
		{pathOf: func(s pathSet) string { return s.contractReadiness }, kind: "contract-readiness", label: "contract readiness regenerated", render: progress.RenderContractReadiness},
		{pathOf: func(s pathSet) string { return s.builderLoopHandoff }, kind: "builder-loop-handoff", label: "builder-loop handoff regenerated", render: progress.RenderBuilderLoopHandoff},
		{pathOf: func(s pathSet) string { return s.agentQueue }, kind: "agent-queue", label: "agent queue regenerated", render: func(p *progress.Progress) string { return progress.RenderAgentQueue(p, 10) }},
		{pathOf: func(s pathSet) string { return s.nextSlices }, kind: "next-slices", label: "next slices regenerated", render: func(p *progress.Progress) string { return progress.RenderNextSlices(p, 10) }},
		{pathOf: func(s pathSet) string { return s.blockedSlices }, kind: "blocked-slices", label: "blocked slices regenerated", render: progress.RenderBlockedSlices},
		{pathOf: func(s pathSet) string { return s.umbrellaCleanup }, kind: "umbrella-cleanup", label: "umbrella cleanup regenerated", render: progress.RenderUmbrellaCleanup},
		{pathOf: func(s pathSet) string { return s.progressSchema }, kind: "progress-schema", label: "progress schema regenerated", render: func(*progress.Progress) string { return progress.RenderProgressSchema() }},
	}

	var errs []error
	for _, m := range markers {
		if err := rewriteMarker(m.pathOf(paths), m.kind, m.render(p)); err != nil {
			errs = append(errs, err)
		} else {
			fmt.Fprintln(stdout, "progress:", m.label)
		}
	}
	if err := syncFile(paths.progressJSON, paths.siteProgress); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(stdout, "progress: site progress data refreshed")
	}
	return joinErrors(stdout, errs)
}

type marker struct {
	pathOf func(pathSet) string
	kind   string
	label  string
	render func(*progress.Progress) string
}

type pathSet struct {
	progressJSON       string
	readme             string
	docsIndex          string
	contractReadiness  string
	builderLoopHandoff string
	agentQueue         string
	nextSlices         string
	blockedSlices      string
	umbrellaCleanup    string
	progressSchema     string
	siteProgress       string
}

func progressPaths(root string) pathSet {
	buildingGormes := filepath.Join(root, "docs", "content", "building-gormes")
	builderLoopDir := filepath.Join(buildingGormes, "builder-loop")
	return pathSet{
		progressJSON:       filepath.Join(buildingGormes, "architecture_plan", "progress.json"),
		readme:             filepath.Join(root, "README.md"),
		docsIndex:          filepath.Join(buildingGormes, "architecture_plan", "_index.md"),
		contractReadiness:  filepath.Join(buildingGormes, "contract-readiness.md"),
		builderLoopHandoff: filepath.Join(builderLoopDir, "builder-loop-handoff.md"),
		agentQueue:         filepath.Join(builderLoopDir, "agent-queue.md"),
		nextSlices:         filepath.Join(builderLoopDir, "next-slices.md"),
		blockedSlices:      filepath.Join(builderLoopDir, "blocked-slices.md"),
		umbrellaCleanup:    filepath.Join(builderLoopDir, "umbrella-cleanup.md"),
		progressSchema:     filepath.Join(builderLoopDir, "progress-schema.md"),
		siteProgress:       filepath.Join(root, "www.gormes.ai", "internal", "site", "data", "progress.json"),
	}
}

func loadValidProgress(root string) (*progress.Progress, error) {
	p, err := progress.Load(progressPaths(root).progressJSON)
	if err != nil {
		return nil, err
	}
	if err := progress.Validate(p); err != nil {
		return nil, err
	}
	return p, nil
}

func rewriteMarker(path, kind, body string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	out, err := progress.ReplaceMarker(string(b), kind, body)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func syncFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

func joinErrors(stdout io.Writer, errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	for _, err := range errs {
		fmt.Fprintln(stdout, "progress:", err)
	}
	return errors.Join(errs...)
}
