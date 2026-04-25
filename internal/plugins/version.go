package plugins

import (
	"fmt"
	"strconv"
	"strings"
)

type semanticVersion struct {
	major int
	minor int
	patch int
}

func versionSatisfies(current, constraint string) (bool, error) {
	currentVersion, err := parseSemanticVersion(current)
	if err != nil {
		return false, err
	}
	parts := strings.Fields(strings.ReplaceAll(constraint, ",", " "))
	if len(parts) == 0 {
		return true, nil
	}
	for _, part := range parts {
		ok, err := checkVersionComparator(currentVersion, part)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}

func checkVersionComparator(current semanticVersion, raw string) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true, nil
	}
	op := "="
	value := raw
	for _, candidate := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(raw, candidate) {
			op = candidate
			value = strings.TrimSpace(strings.TrimPrefix(raw, candidate))
			break
		}
	}
	want, err := parseSemanticVersion(value)
	if err != nil {
		return false, err
	}
	cmp := compareSemanticVersion(current, want)
	switch op {
	case ">=":
		return cmp >= 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case "<":
		return cmp < 0, nil
	case "=":
		return cmp == 0, nil
	default:
		return false, fmt.Errorf("unsupported version comparator %q", op)
	}
}

func parseSemanticVersion(raw string) (semanticVersion, error) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "v")
	fields := strings.Split(raw, ".")
	if len(fields) != 3 {
		return semanticVersion{}, fmt.Errorf("version %q must use major.minor.patch", raw)
	}
	major, err := strconv.Atoi(fields[0])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid major version %q", fields[0])
	}
	minor, err := strconv.Atoi(fields[1])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid minor version %q", fields[1])
	}
	patch, err := strconv.Atoi(fields[2])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid patch version %q", fields[2])
	}
	return semanticVersion{major: major, minor: minor, patch: patch}, nil
}

func compareSemanticVersion(a, b semanticVersion) int {
	if a.major != b.major {
		return compareInt(a.major, b.major)
	}
	if a.minor != b.minor {
		return compareInt(a.minor, b.minor)
	}
	return compareInt(a.patch, b.patch)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
