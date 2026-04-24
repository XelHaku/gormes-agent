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

func readReportFixture(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join("..", "..", "scripts", "orchestrator", "tests", "fixtures", "reports", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(raw)
}
