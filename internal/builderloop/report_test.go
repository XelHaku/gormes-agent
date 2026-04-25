package builderloop

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParseFinalReportRequiresAcceptanceAndCommit(t *testing.T) {
	report := strings.Join([]string{
		"Summary:",
		"Commit: abc123def",
		"Acceptance:",
		"- claim cleanup removes stale locks",
		"- failures are recorded",
		"- RED: go test ./internal/builderloop -run TestThing exited with exit 1",
		"- GREEN: go test ./internal/builderloop exited with exit 0",
		"",
	}, "\n")

	got, err := ParseFinalReport(report)
	if err != nil {
		t.Fatalf("ParseFinalReport() error = %v", err)
	}

	want := FinalReport{
		Commit: "abc123def",
		Acceptance: []string{
			"claim cleanup removes stale locks",
			"failures are recorded",
			"RED: go test ./internal/builderloop -run TestThing exited with exit 1",
			"GREEN: go test ./internal/builderloop exited with exit 0",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseFinalReport() = %#v, want %#v", got, want)
	}

	_, err = ParseFinalReport("Acceptance:\n- item\nRED exit 1\nGREEN exit 0\n")
	if err == nil {
		t.Fatal("ParseFinalReport() without commit error = nil, want error")
	}

	_, err = ParseFinalReport("Commit: abc123\nRED exit 1\nGREEN exit 0\n")
	if err == nil {
		t.Fatal("ParseFinalReport() without acceptance error = nil, want error")
	}
}

func TestParseFinalReportRejectsMissingRed(t *testing.T) {
	report := strings.Join([]string{
		"Commit: abc123",
		"GREEN: go test exited with exit 0",
		"Acceptance:",
		"- all good",
	}, "\n")

	_, err := ParseFinalReport(report)
	if err == nil {
		t.Fatal("ParseFinalReport() error = nil, want missing RED error")
	}
}

func TestParseFinalReportRejectsEvidenceOutsideAcceptance(t *testing.T) {
	report := strings.Join([]string{
		"Commit: abc123",
		"RED: go test exited with exit 1",
		"GREEN: go test exited with exit 0",
		"Acceptance:",
		"- implementation is complete",
		"- tests were considered",
	}, "\n")

	_, err := ParseFinalReport(report)
	if err == nil {
		t.Fatal("ParseFinalReport() error = nil, want missing acceptance evidence error")
	}
}

func TestParseFinalReportRejectsMisleadingAcceptanceEvidence(t *testing.T) {
	report := strings.Join([]string{
		"Commit: abc123",
		"Acceptance:",
		"- not red, no exit 1 evidence",
		"- not green, no exit 0 evidence",
	}, "\n")

	_, err := ParseFinalReport(report)
	if err == nil {
		t.Fatal("ParseFinalReport() error = nil, want missing labeled evidence error")
	}
}

func TestParseFinalReportAcceptsLegacyGoodFixture(t *testing.T) {
	got, err := ParseFinalReport(readReportFixture(t, "good.final.md"))
	if err != nil {
		t.Fatalf("ParseFinalReport() error = %v", err)
	}

	want := FinalReport{
		Commit: "abc1234def5678",
		Acceptance: []string{
			"TestBar fails before implementation — PASS",
			"TestBar passes after implementation — PASS",
			"progress.json entry marked in_progress with symbol note — PASS",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseFinalReport() = %#v, want %#v", got, want)
	}
}

func TestParseFinalReportRejectsLegacyBadFixtures(t *testing.T) {
	fixtures := []string{
		"bad-no-acceptance.final.md",
		"bad-acceptance-fail.final.md",
		"bad-no-red-exit.final.md",
		"bad-no-commit-hash.final.md",
		"bad-all-zero-exits.final.md",
		"bad-empty.final.md",
		"bad-missing-branch.final.md",
		"bad-missing-section.final.md",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			_, err := ParseFinalReport(readReportFixture(t, fixture))
			if err == nil {
				t.Fatal("ParseFinalReport() error = nil, want error")
			}
		})
	}
}

func TestParseFinalReportRejectsLegacyMissingBranchWithCompleteEvidence(t *testing.T) {
	report := legacyReportFixture("")
	report = strings.Replace(report, "Branch: codexu/test-run/worker1\n", "", 1)

	_, err := ParseFinalReport(report)
	if err == nil {
		t.Fatal("ParseFinalReport() error = nil, want missing branch error")
	}
	if !strings.Contains(err.Error(), "Branch") {
		t.Fatalf("ParseFinalReport() error = %q, want Branch message", err)
	}
}

func TestParseFinalReportRejectsLegacyMissingSectionWithCompleteEvidence(t *testing.T) {
	report := legacyReportFixture("5")

	_, err := ParseFinalReport(report)
	if err == nil {
		t.Fatal("ParseFinalReport() error = nil, want missing section error")
	}
	if !strings.Contains(err.Error(), "section") {
		t.Fatalf("ParseFinalReport() error = %q, want section message", err)
	}
}

func TestParseFinalReportRejectsLegacyWrongSectionExitEvidence(t *testing.T) {
	cases := map[string]struct {
		section int
		exit    int
	}{
		"red zero with later nonzero": {section: 3, exit: 0},
		"green nonzero":               {section: 4, exit: 1},
		"refactor nonzero":            {section: 5, exit: 1},
		"regression nonzero":          {section: 6, exit: 1},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			extraExit := 0
			if tc.section == 3 {
				extraExit = 1
			}
			report := legacyReportFixtureWithOptions(legacyReportOptions{
				exits: map[int]int{
					3: exitForSection(tc.section, tc.exit, 3, 1),
					4: exitForSection(tc.section, tc.exit, 4, 0),
					5: exitForSection(tc.section, tc.exit, 5, 0),
					6: exitForSection(tc.section, tc.exit, 6, 0),
					7: extraExit,
				},
			})

			_, err := ParseFinalReport(report)
			if err == nil {
				t.Fatal("ParseFinalReport() error = nil, want section-specific exit error")
			}
		})
	}
}

func exitForSection(changedSection, changedExit, section, defaultExit int) int {
	if section == changedSection {
		return changedExit
	}
	return defaultExit
}

func legacyReportFixture(skipSection string) string {
	return legacyReportFixtureWithOptions(legacyReportOptions{skipSection: skipSection})
}

type legacyReportOptions struct {
	skipSection string
	exits       map[int]int
}

func legacyReportFixtureWithOptions(options legacyReportOptions) string {
	exitFor := func(section int, fallback int) string {
		exitCode := fallback
		if options.exits != nil {
			if override, ok := options.exits[section]; ok {
				exitCode = override
			}
		}
		return "Exit: " + strconv.Itoa(exitCode)
	}

	sections := []struct {
		number string
		title  string
		body   []string
	}{
		{"1", "Selected task", []string{"Task: 1 / 1.A / Item A2"}},
		{"2", "Pre-doc baseline", []string{"Files:", "- docs/progress.json"}},
		{"3", "RED proof", []string{"Command: go test ./internal/foo", exitFor(3, 1), "Snippet: FAIL: TestBar"}},
		{"4", "GREEN proof", []string{"Command: go test ./internal/foo", exitFor(4, 0), "Snippet: PASS"}},
		{"5", "REFACTOR proof", []string{"Command: go test ./internal/foo", exitFor(5, 0), "Snippet: PASS"}},
		{"6", "Regression proof", []string{"Command: go test ./...", exitFor(6, 0), "Snippet: ok"}},
		{"7", "Post-doc closeout", []string{"Files:", "- docs/progress.json", exitFor(7, 0)}},
		{"8", "Commit", []string{"Branch: codexu/test-run/worker1", "Commit: abc1234def5678", "Files:", "- internal/foo/foo.go"}},
		{"9", "Acceptance check", []string{
			"Criterion: TestBar fails before implementation — PASS",
			"Criterion: TestBar passes after implementation — PASS",
			"Criterion: progress.json entry marked in_progress with symbol note — PASS",
		}},
	}

	var lines []string
	for _, section := range sections {
		if section.number == options.skipSection {
			lines = append(lines, section.body...)
			continue
		}
		lines = append(lines, section.number+") "+section.title)
		lines = append(lines, section.body...)
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func readReportFixture(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join("..", "..", "testdata", "legacy-shell", "scripts", "orchestrator", "tests", "fixtures", "reports", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(raw)
}

// setupRepoWithCommit initializes a git repo at dir with one commit on
// `main`, then creates a `worker` branch with one additional commit checked
// out as HEAD. Returns the worktree path (== dir) and the base branch name
// (`main`). The worker commit is reachable from HEAD but not from main, so
// `main..HEAD` yields exactly one commit and a non-empty diff.
func setupRepoWithCommit(t *testing.T) (workdir string, baseBranch string) {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	mustGit(t, dir, "add", "README.md")
	mustGit(t, dir, "commit", "-m", "init")
	// Branch off main and add a worker commit so main..HEAD is non-empty.
	mustGit(t, dir, "checkout", "-b", "worker")
	if err := os.WriteFile(filepath.Join(dir, "worker.txt"), []byte("worker change\n"), 0o644); err != nil {
		t.Fatalf("write worker.txt: %v", err)
	}
	mustGit(t, dir, "add", "worker.txt")
	mustGit(t, dir, "commit", "-m", "worker change")
	return dir, "main"
}

// setupRepoNoCommits initializes an empty git repo (no commits).
func setupRepoNoCommits(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test User")
	return dir
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestTryRepairReport_NoCommitFails(t *testing.T) {
	dir := setupRepoNoCommits(t)
	rep, _, err := TryRepairReport(RepairContext{
		WorkerStdout: "PASS\nok",
		WorktreePath: dir,
		BaseBranch:   "main",
	})
	if err == nil {
		t.Fatalf("expected repair to fail with no commit; got rep=%+v", rep)
	}
}

func TestTryRepairReport_NoPassFails(t *testing.T) {
	dir, base := setupRepoWithCommit(t)
	rep, _, err := TryRepairReport(RepairContext{
		WorkerStdout: "FAIL: foo broke",
		WorktreePath: dir,
		BaseBranch:   base,
	})
	if err == nil {
		t.Fatalf("expected repair to fail without PASS token; got rep=%+v", rep)
	}
}

func TestTryRepairReport_AcceptanceMissingFails(t *testing.T) {
	dir, base := setupRepoWithCommit(t)
	rep, _, err := TryRepairReport(RepairContext{
		WorkerStdout:    "ok\nPASS",
		WorktreePath:    dir,
		BaseBranch:      base,
		AcceptanceLines: []string{"acceptance-line-A"}, // not in stdout
	})
	if err == nil {
		t.Fatalf("expected repair to fail when acceptance line missing; got rep=%+v", rep)
	}
}

func TestTryRepairReport_AcceptanceEmptyAcceptsOnPassEvidence(t *testing.T) {
	dir, base := setupRepoWithCommit(t)
	rep, notes, err := TryRepairReport(RepairContext{
		WorkerStdout: "all good\nPASS\nok\n",
		WorktreePath: dir,
		BaseBranch:   base,
		// AcceptanceLines empty → fallback rule: accept on PASS evidence.
	})
	if err != nil || rep == nil {
		t.Fatalf("expected repair to succeed, got err=%v rep=%v", err, rep)
	}
	if rep.Commit == "" {
		t.Fatal("expected reconstructed commit")
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one RepairNote")
	}
	// Synthesized acceptance must satisfy the existing acceptanceEvidence
	// contract (one RED with exit 1, one GREEN with exit 0).
	hasRed, hasGreen := acceptanceEvidence(rep.Acceptance)
	if !hasRed {
		t.Fatalf("synthesized acceptance lacks RED evidence: %v", rep.Acceptance)
	}
	if !hasGreen {
		t.Fatalf("synthesized acceptance lacks GREEN evidence: %v", rep.Acceptance)
	}
}

func TestTryRepairReport_AllAcceptanceLinesPresentAccepts(t *testing.T) {
	dir, base := setupRepoWithCommit(t)
	rep, _, err := TryRepairReport(RepairContext{
		WorkerStdout:    "acceptance-A done\nacceptance-B done\nPASS\nok",
		WorktreePath:    dir,
		BaseBranch:      base,
		AcceptanceLines: []string{"acceptance-A", "acceptance-B"},
	})
	if err != nil || rep == nil {
		t.Fatalf("expected repair to succeed, got err=%v", err)
	}
	if rep.Commit == "" {
		t.Fatal("expected reconstructed commit")
	}
}

func TestTryRepairReport_EmptyDiffFails(t *testing.T) {
	dir, base := setupRepoWithCommit(t)
	// Reset to the base branch so HEAD..base diff is empty.
	mustGit(t, dir, "reset", "--hard", base)
	rep, _, err := TryRepairReport(RepairContext{
		WorkerStdout: "PASS\nok",
		WorktreePath: dir,
		BaseBranch:   base,
	})
	if err == nil {
		t.Fatalf("expected repair to fail with empty diff; got rep=%+v", rep)
	}
}

func TestWriteRepairArtifact_WritesJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repairs", "run-1-worker-2.json")
	rep := &FinalReport{Commit: "abc123", Acceptance: []string{"GREEN: ok"}}
	notes := []RepairNote{{Field: "commit", Source: "git_log", Detail: "abc123"}}
	if err := writeRepairArtifact(path, Candidate{ItemName: "row-x"}, rep, "diff body\n", notes, "PASS\nok\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(body)
	for _, want := range []string{`"commit": "abc123"`, `"ItemName": "row-x"`, `"PASS\nok\n"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("artifact missing %q\n%s", want, got)
		}
	}
}
