package builderloop

import (
	"strings"
	"time"
)

type RowKind string

const (
	RowKindSelfImprovement RowKind = "self_improvement"
	RowKindUserFeature     RowKind = "user_feature"
	RowKindUnclassified    RowKind = "unclassified"
)

type ShippedRowEvent struct {
	SubphaseID string
	ShippedAt  time.Time
}

type ShipRatio struct {
	SelfImprovement int
	UserFeature     int
	Unclassified    int
	Total           int
}

var phaseSegments = map[string]bool{
	"1": true,
	"4": true,
	"5": true,
	"6": true,
	"7": true,
}

func ClassifySubphase(subphaseID string) RowKind {
	trimmed := strings.TrimSpace(subphaseID)
	if trimmed == "" {
		return RowKindUnclassified
	}

	if strings.HasPrefix(trimmed, "control-plane/") {
		return RowKindSelfImprovement
	}

	segment := trimmed
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		first := trimmed[:idx]
		if phaseSegments[first] {
			rest := trimmed[idx+1:]
			if next := strings.Index(rest, "/"); next >= 0 {
				segment = rest[:next]
			} else {
				segment = rest
			}
		}
	}

	return classifyToken(segment)
}

func classifyToken(token string) RowKind {
	token = strings.TrimSpace(token)
	if token == "" {
		return RowKindUnclassified
	}

	dot := strings.Index(token, ".")
	if dot <= 0 {
		return RowKindUnclassified
	}
	phase := token[:dot]
	letter := token[dot+1:]
	if letter == "" {
		return RowKindUnclassified
	}

	switch phase {
	case "1":
		if letter == "C" {
			return RowKindSelfImprovement
		}
	case "5":
		if letter == "O" {
			return RowKindSelfImprovement
		}
	case "4":
		if letter == "A" || letter == "H" {
			return RowKindUserFeature
		}
	case "6", "7":
		return RowKindUserFeature
	}
	return RowKindUnclassified
}

func ComputeShipRatio(events []ShippedRowEvent, window time.Duration, now time.Time) ShipRatio {
	var ratio ShipRatio
	if len(events) == 0 {
		return ratio
	}
	cutoff := now.Add(-window)
	for _, ev := range events {
		if ev.ShippedAt.Before(cutoff) {
			continue
		}
		if ev.ShippedAt.After(now) {
			continue
		}
		switch ClassifySubphase(ev.SubphaseID) {
		case RowKindSelfImprovement:
			ratio.SelfImprovement++
		case RowKindUserFeature:
			ratio.UserFeature++
		default:
			ratio.Unclassified++
		}
	}
	ratio.Total = ratio.SelfImprovement + ratio.UserFeature + ratio.Unclassified
	return ratio
}
