package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func runProgress(root string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf(usage)
	}

	switch args[0] {
	case "validate":
		return validateProgress(root)
	case "write":
		return writeProgress(root)
	default:
		return fmt.Errorf(usage)
	}
}

func validateProgress(root string) error {
	p, err := loadValidProgress(root)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(commandStdout, "progress: validated %d phases\n", len(p.Phases))
	return err
}

func writeProgress(root string) error {
	p, err := loadValidProgress(root)
	if err != nil {
		return err
	}

	paths := progressPaths(root)
	var errs []error
	if err := rewriteProgressMarker(paths.docsIndex, "docs-full-checklist", progress.RenderDocsChecklist(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: _index.md regenerated")
	}
	if err := rewriteProgressMarker(paths.readme, "readme-rollup", progress.RenderReadmeRollup(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: README.md regenerated")
	}
	if err := rewriteProgressMarker(paths.contractReadiness, "contract-readiness", progress.RenderContractReadiness(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: contract readiness regenerated")
	}
	if err := rewriteProgressMarker(paths.autoloopHandoff, "autoloop-handoff", progress.RenderAutoloopHandoff(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: autoloop handoff regenerated")
	}
	if err := rewriteProgressMarker(paths.agentQueue, "agent-queue", progress.RenderAgentQueue(p, 10)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: agent queue regenerated")
	}
	if err := rewriteProgressMarker(paths.nextSlices, "next-slices", progress.RenderNextSlices(p, 10)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: next slices regenerated")
	}
	if err := rewriteProgressMarker(paths.blockedSlices, "blocked-slices", progress.RenderBlockedSlices(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: blocked slices regenerated")
	}
	if err := rewriteProgressMarker(paths.umbrellaCleanup, "umbrella-cleanup", progress.RenderUmbrellaCleanup(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: umbrella cleanup regenerated")
	}
	if err := rewriteProgressMarker(paths.progressSchema, "progress-schema", progress.RenderProgressSchema()); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: progress schema regenerated")
	}
	if err := syncProgressFile(paths.progressJSON, paths.siteProgress); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Fprintln(commandStdout, "progress: site progress data refreshed")
	}
	return joinProgressErrors(errs)
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

type progressPathSet struct {
	progressJSON      string
	readme            string
	docsIndex         string
	contractReadiness string
	autoloopHandoff   string
	agentQueue        string
	nextSlices        string
	blockedSlices     string
	umbrellaCleanup   string
	progressSchema    string
	siteProgress      string
}

func progressPaths(root string) progressPathSet {
	buildingGormes := filepath.Join(root, "docs", "content", "building-gormes")
	autoloopDir := filepath.Join(buildingGormes, "autoloop")
	return progressPathSet{
		progressJSON:      filepath.Join(buildingGormes, "architecture_plan", "progress.json"),
		readme:            filepath.Join(root, "README.md"),
		docsIndex:         filepath.Join(buildingGormes, "architecture_plan", "_index.md"),
		contractReadiness: filepath.Join(buildingGormes, "contract-readiness.md"),
		autoloopHandoff:   filepath.Join(autoloopDir, "autoloop-handoff.md"),
		agentQueue:        filepath.Join(autoloopDir, "agent-queue.md"),
		nextSlices:        filepath.Join(autoloopDir, "next-slices.md"),
		blockedSlices:     filepath.Join(autoloopDir, "blocked-slices.md"),
		umbrellaCleanup:   filepath.Join(autoloopDir, "umbrella-cleanup.md"),
		progressSchema:    filepath.Join(autoloopDir, "progress-schema.md"),
		siteProgress:      filepath.Join(root, "www.gormes.ai", "internal", "site", "data", "progress.json"),
	}
}

func rewriteProgressMarker(path, kind, body string) error {
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

func syncProgressFile(src, dst string) error {
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

func joinProgressErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	for _, err := range errs {
		fmt.Fprintln(commandStdout, "progress:", err)
	}
	return errors.Join(errs...)
}
