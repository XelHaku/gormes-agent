package autoloop

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type FinalReport struct {
	Commit     string
	Acceptance []string
}

var commitLinePattern = regexp.MustCompile("^Commit:\\s*`?([0-9a-fA-F]{7,40})`?\\s*$")
var exitLinePattern = regexp.MustCompile(`^Exit:\s*(-?\d+)\s*$`)

func ParseFinalReport(text string) (FinalReport, error) {
	var report FinalReport
	var inAcceptance bool
	var legacy legacyReportEvidence

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if match := commitLinePattern.FindStringSubmatch(trimmed); match != nil {
			report.Commit = match[1]
		}
		legacy.collect(trimmed)

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
	if len(report.Acceptance) > 0 {
		hasRed, hasGreen := acceptanceEvidence(report.Acceptance)
		if !hasRed {
			return FinalReport{}, fmt.Errorf("final report missing RED evidence with exit 1")
		}
		if !hasGreen {
			return FinalReport{}, fmt.Errorf("final report missing GREEN evidence with exit 0")
		}
		return report, nil
	}

	if err := legacy.validate(); err != nil {
		return FinalReport{}, err
	}
	report.Acceptance = legacy.criteria
	return report, nil
}

func acceptanceEvidence(acceptance []string) (bool, bool) {
	var hasRed bool
	var hasGreen bool

	for _, item := range acceptance {
		lower := strings.ToLower(strings.TrimSpace(item))
		if strings.HasPrefix(lower, "red:") && strings.Contains(lower, "exit 1") {
			hasRed = true
		}
		if strings.HasPrefix(lower, "green:") && strings.Contains(lower, "exit 0") {
			hasGreen = true
		}
	}

	return hasRed, hasGreen
}

type legacyReportEvidence struct {
	commandCount int
	zeroExits    int
	nonZeroExits int
	criteria     []string
}

func (legacy *legacyReportEvidence) collect(line string) {
	if strings.HasPrefix(line, "Command:") {
		legacy.commandCount++
		return
	}
	if match := exitLinePattern.FindStringSubmatch(line); match != nil {
		exitCode, err := strconv.Atoi(match[1])
		if err != nil {
			return
		}
		if exitCode == 0 {
			legacy.zeroExits++
		} else {
			legacy.nonZeroExits++
		}
		return
	}
	if strings.HasPrefix(line, "Criterion:") {
		criterion := strings.TrimSpace(strings.TrimPrefix(line, "Criterion:"))
		if criterion != "" {
			legacy.criteria = append(legacy.criteria, criterion)
		}
	}
}

func (legacy legacyReportEvidence) validate() error {
	if legacy.commandCount < 4 {
		return fmt.Errorf("final report missing command evidence")
	}
	if legacy.nonZeroExits < 1 {
		return fmt.Errorf("final report missing non-zero RED exit evidence")
	}
	if legacy.zeroExits < 3 {
		return fmt.Errorf("final report missing GREEN exit evidence")
	}
	if len(legacy.criteria) < 3 {
		return fmt.Errorf("final report missing acceptance")
	}
	for _, criterion := range legacy.criteria {
		if strings.Contains(criterion, "FAIL") {
			return fmt.Errorf("final report acceptance failed")
		}
	}
	return nil
}
