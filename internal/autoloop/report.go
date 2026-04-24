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
var branchLinePattern = regexp.MustCompile("^Branch:\\s*`?(.+)`?\\s*$")
var exitLinePattern = regexp.MustCompile("^Exit:\\s*`?(-?\\d+)`?\\s*$")
var sectionLinePattern = regexp.MustCompile(`^([1-9])[).]\s*(.+)$`)

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
	hasBranch    bool
	sections     [9]bool
	current      int
	sectionExits map[int][]int
	criteria     []string
}

func (legacy *legacyReportEvidence) collect(line string) {
	legacy.collectSection(line)
	if branchLinePattern.MatchString(line) {
		legacy.hasBranch = true
		return
	}
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
		if legacy.current > 0 {
			if legacy.sectionExits == nil {
				legacy.sectionExits = make(map[int][]int)
			}
			legacy.sectionExits[legacy.current] = append(legacy.sectionExits[legacy.current], exitCode)
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

func (legacy *legacyReportEvidence) collectSection(line string) {
	normalized := normalizeSectionLine(line)
	match := sectionLinePattern.FindStringSubmatch(normalized)
	if match == nil {
		return
	}

	number, err := strconv.Atoi(match[1])
	if err != nil || number < 1 || number > len(legacySectionTitles) {
		return
	}
	title := strings.Trim(strings.TrimSpace(match[2]), "*")
	title = strings.TrimSpace(title)
	if title == legacySectionTitles[number-1] {
		legacy.sections[number-1] = true
		legacy.current = number
	}
}

func normalizeSectionLine(line string) string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "#") {
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
	}
	for strings.HasPrefix(line, "**") && strings.HasSuffix(line, "**") && len(line) >= 4 {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "**"), "**"))
	}
	return line
}

func (legacy legacyReportEvidence) validate() error {
	for i, found := range legacy.sections {
		if !found {
			return fmt.Errorf("final report missing section %d) %s", i+1, legacySectionTitles[i])
		}
	}
	if legacy.commandCount < 4 {
		return fmt.Errorf("final report missing command evidence")
	}
	if legacy.nonZeroExits < 1 {
		return fmt.Errorf("final report missing non-zero RED exit evidence")
	}
	if legacy.zeroExits < 3 {
		return fmt.Errorf("final report missing GREEN exit evidence")
	}
	if !sectionHasNonZeroExit(legacy.sectionExits[3]) {
		return fmt.Errorf("final report RED proof missing non-zero exit")
	}
	if !sectionHasZeroExit(legacy.sectionExits[4]) {
		return fmt.Errorf("final report GREEN proof missing zero exit")
	}
	if !sectionHasZeroExit(legacy.sectionExits[5]) {
		return fmt.Errorf("final report REFACTOR proof missing zero exit")
	}
	if !sectionHasZeroExit(legacy.sectionExits[6]) {
		return fmt.Errorf("final report Regression proof missing zero exit")
	}
	if len(legacy.criteria) < 3 {
		return fmt.Errorf("final report missing acceptance")
	}
	if !legacy.hasBranch {
		return fmt.Errorf("final report missing Branch field")
	}
	for _, criterion := range legacy.criteria {
		if strings.Contains(criterion, "FAIL") {
			return fmt.Errorf("final report acceptance failed")
		}
	}
	return nil
}

func sectionHasNonZeroExit(exits []int) bool {
	for _, exit := range exits {
		if exit != 0 {
			return true
		}
	}
	return false
}

func sectionHasZeroExit(exits []int) bool {
	for _, exit := range exits {
		if exit == 0 {
			return true
		}
	}
	return false
}

var legacySectionTitles = []string{
	"Selected task",
	"Pre-doc baseline",
	"RED proof",
	"GREEN proof",
	"REFACTOR proof",
	"Regression proof",
	"Post-doc closeout",
	"Commit",
	"Acceptance check",
}
