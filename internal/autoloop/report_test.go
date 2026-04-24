package autoloop

import (
	"os"
	"path/filepath"
	"reflect"
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
		"- RED: go test ./internal/autoloop -run TestThing exited with exit 1",
		"- GREEN: go test ./internal/autoloop exited with exit 0",
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
			"RED: go test ./internal/autoloop -run TestThing exited with exit 1",
			"GREEN: go test ./internal/autoloop exited with exit 0",
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

func legacyReportFixture(skipSection string) string {
	sections := []struct {
		number string
		title  string
		body   []string
	}{
		{"1", "Selected task", []string{"Task: 1 / 1.A / Item A2"}},
		{"2", "Pre-doc baseline", []string{"Files:", "- docs/progress.json"}},
		{"3", "RED proof", []string{"Command: go test ./internal/foo", "Exit: 1", "Snippet: FAIL: TestBar"}},
		{"4", "GREEN proof", []string{"Command: go test ./internal/foo", "Exit: 0", "Snippet: PASS"}},
		{"5", "REFACTOR proof", []string{"Command: go test ./internal/foo", "Exit: 0", "Snippet: PASS"}},
		{"6", "Regression proof", []string{"Command: go test ./...", "Exit: 0", "Snippet: ok"}},
		{"7", "Post-doc closeout", []string{"Files:", "- docs/progress.json"}},
		{"8", "Commit", []string{"Branch: codexu/test-run/worker1", "Commit: abc1234def5678", "Files:", "- internal/foo/foo.go"}},
		{"9", "Acceptance check", []string{
			"Criterion: TestBar fails before implementation — PASS",
			"Criterion: TestBar passes after implementation — PASS",
			"Criterion: progress.json entry marked in_progress with symbol note — PASS",
		}},
	}

	var lines []string
	for _, section := range sections {
		if section.number == skipSection {
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

	path := filepath.Join("..", "..", "scripts", "orchestrator", "tests", "fixtures", "reports", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(raw)
}
