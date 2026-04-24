package autoloop

import (
	"fmt"
	"regexp"
	"strings"
)

type FinalReport struct {
	Commit     string
	Acceptance []string
}

var commitLinePattern = regexp.MustCompile(`^Commit:\s*([0-9a-fA-F]+)\s*$`)

func ParseFinalReport(text string) (FinalReport, error) {
	var report FinalReport
	var inAcceptance bool
	var hasRed bool
	var hasGreen bool

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if match := commitLinePattern.FindStringSubmatch(trimmed); match != nil {
			report.Commit = match[1]
		}

		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "red") && strings.Contains(lower, "exit 1") {
			hasRed = true
		}
		if strings.Contains(lower, "green") && strings.Contains(lower, "exit 0") {
			hasGreen = true
		}

		if strings.EqualFold(trimmed, "Acceptance:") {
			inAcceptance = true
			continue
		}
		if !inAcceptance {
			continue
		}

		if strings.HasPrefix(trimmed, "-") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if item != "" {
				report.Acceptance = append(report.Acceptance, item)
			}
			continue
		}
		if trimmed != "" {
			inAcceptance = false
		}
	}

	if report.Commit == "" {
		return FinalReport{}, fmt.Errorf("final report missing commit")
	}
	if len(report.Acceptance) == 0 {
		return FinalReport{}, fmt.Errorf("final report missing acceptance")
	}
	if !hasRed {
		return FinalReport{}, fmt.Errorf("final report missing RED evidence with exit 1")
	}
	if !hasGreen {
		return FinalReport{}, fmt.Errorf("final report missing GREEN evidence with exit 0")
	}

	return report, nil
}
