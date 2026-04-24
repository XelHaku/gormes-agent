// Command progress-gen validates the canonical architecture progress.json,
// regenerates marker-bounded roadmap regions, and refreshes generated site
// progress data. Run from the main Makefile.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func main() {
	validate := flag.Bool("validate", false, "validate progress.json only (read-only)")
	write := flag.Bool("write", false, "regenerate marker regions and generated progress docs")
	flag.Parse()

	if !*validate && !*write {
		fmt.Fprintln(os.Stderr, "progress-gen: pass -validate or -write")
		os.Exit(2)
	}

	// Resolve paths relative to the main module root.
	root, err := os.Getwd()
	if err != nil {
		die(err)
	}
	progressPath := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	readmePath := filepath.Join(root, "README.md")
	docsIndexPath := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "_index.md")
	contractReadinessPath := filepath.Join(root, "docs", "content", "building-gormes", "contract-readiness.md")
	autoloopHandoffPath := filepath.Join(root, "docs", "content", "building-gormes", "autoloop-handoff.md")
	agentQueuePath := filepath.Join(root, "docs", "content", "building-gormes", "agent-queue.md")
	nextSlicesPath := filepath.Join(root, "docs", "content", "building-gormes", "next-slices.md")
	blockedSlicesPath := filepath.Join(root, "docs", "content", "building-gormes", "blocked-slices.md")
	umbrellaCleanupPath := filepath.Join(root, "docs", "content", "building-gormes", "umbrella-cleanup.md")
	progressSchemaPath := filepath.Join(root, "docs", "content", "building-gormes", "progress-schema.md")
	siteProgressPath := filepath.Join(root, "www.gormes.ai", "internal", "site", "data", "progress.json")

	p, err := progress.Load(progressPath)
	if err != nil {
		die(err)
	}
	if err := progress.Validate(p); err != nil {
		die(err)
	}

	if *validate {
		fmt.Printf("progress-gen: validated %d phases\n", len(p.Phases))
		return
	}

	// Attempt both files independently. Accumulate errors so a missing
	// marker in one file does not prevent the other from being regenerated.
	// _index.md runs first because it lands before README in the rollout
	// (Task 11 lands _index markers, Task 12 lands README markers).
	var errs []error
	if err := rewrite(docsIndexPath, "docs-full-checklist", progress.RenderDocsChecklist(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: _index.md regenerated")
	}
	if err := rewrite(readmePath, "readme-rollup", progress.RenderReadmeRollup(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: README.md regenerated")
	}
	if err := rewrite(contractReadinessPath, "contract-readiness", progress.RenderContractReadiness(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: contract readiness regenerated")
	}
	if err := rewrite(autoloopHandoffPath, "autoloop-handoff", progress.RenderAutoloopHandoff(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: autoloop handoff regenerated")
	}
	if err := rewrite(agentQueuePath, "agent-queue", progress.RenderAgentQueue(p, 10)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: agent queue regenerated")
	}
	if err := rewrite(nextSlicesPath, "next-slices", progress.RenderNextSlices(p, 10)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: next slices regenerated")
	}
	if err := rewrite(blockedSlicesPath, "blocked-slices", progress.RenderBlockedSlices(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: blocked slices regenerated")
	}
	if err := rewrite(umbrellaCleanupPath, "umbrella-cleanup", progress.RenderUmbrellaCleanup(p)); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: umbrella cleanup regenerated")
	}
	if err := rewrite(progressSchemaPath, "progress-schema", progress.RenderProgressSchema()); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: progress schema regenerated")
	}
	if err := syncFile(progressPath, siteProgressPath); err != nil {
		errs = append(errs, err)
	} else {
		fmt.Println("progress-gen: site progress data refreshed")
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "progress-gen:", e)
		}
		os.Exit(1)
	}
}

func syncFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

func rewrite(path, kind, body string) error {
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

func die(err error) {
	fmt.Fprintln(os.Stderr, "progress-gen:", err)
	os.Exit(1)
}
