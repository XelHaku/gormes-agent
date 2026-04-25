package gateway

import (
	"fmt"
	"sort"
	"strings"
)

// StatusChannel is a configured channel row for the read-only gateway status
// view. Detail should be non-secret operator context such as an allowlist ID.
type StatusChannel struct {
	Name   string
	Detail string
}

// StatusSummary is the pure input model for RenderStatusSummary.
type StatusSummary struct {
	Channels []StatusChannel
	Pairing  PairingStatus
	Runtime  RuntimeStatus
}

// RenderStatusSummary renders the operator-facing gateway status text without
// touching transports, clients, stores, or process state.
func RenderStatusSummary(summary StatusSummary) string {
	var b strings.Builder
	b.WriteString("Gateway status\n")
	b.WriteString(renderRuntimeLine(summary.Runtime))
	b.WriteByte('\n')

	channels := sortedStatusChannels(summary.Channels)
	if len(channels) == 0 {
		b.WriteString("channels: none configured\n")
	} else {
		b.WriteString("channels:\n")
		pairingByPlatform := pairingPlatformMap(summary.Pairing.Platforms)
		for _, channel := range channels {
			b.WriteString(renderChannelLine(channel, summary.Runtime, pairingByPlatform[channel.Name]))
			b.WriteByte('\n')
		}
	}

	pending := sortedPendingPairingRecords(summary.Pairing.Pending)
	approved := sortedApprovedPairingRecords(summary.Pairing.Approved)
	if len(pending) > 0 || len(approved) > 0 {
		b.WriteString("pairing:\n")
		for _, record := range pending {
			b.WriteString(fmt.Sprintf("- pending %s user=%s code=%s age=%ds\n", record.Platform, record.UserID, record.Code, record.AgeSeconds))
		}
		for _, record := range approved {
			b.WriteString(fmt.Sprintf("- approved %s user=%s", record.Platform, record.UserID))
			if record.UserName != "" {
				b.WriteString(" name=")
				b.WriteString(record.UserName)
			}
			b.WriteByte('\n')
		}
	}

	degraded := sortedPairingDegradedEvidence(summary.Pairing.Degraded)
	if len(degraded) > 0 {
		b.WriteString("degraded:\n")
		for _, evidence := range degraded {
			b.WriteString("- pairing")
			if evidence.Platform != "" {
				b.WriteByte(' ')
				b.WriteString(evidence.Platform)
			}
			if evidence.Reason != "" {
				b.WriteByte(' ')
				b.WriteString(string(evidence.Reason))
			}
			if evidence.Message != "" {
				b.WriteString(": ")
				b.WriteString(evidence.Message)
			}
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func renderRuntimeLine(runtime RuntimeStatus) string {
	if runtimeStatusMissing(runtime) {
		return "runtime: missing"
	}

	state := string(runtime.GatewayState)
	if state == "" {
		state = "unknown"
	}
	parts := []string{}
	if runtime.PID > 0 {
		parts = append(parts, fmt.Sprintf("pid=%d", runtime.PID))
	}
	parts = append(parts, fmt.Sprintf("active_agents=%d", runtime.ActiveAgents))
	if runtime.ExitReason != "" {
		parts = append(parts, fmt.Sprintf("exit_reason=%q", runtime.ExitReason))
	}
	return fmt.Sprintf("runtime: %s (%s)", state, strings.Join(parts, " "))
}

func runtimeStatusMissing(runtime RuntimeStatus) bool {
	return runtime.Kind == "" &&
		runtime.PID == 0 &&
		runtime.GatewayState == "" &&
		runtime.ExitReason == "" &&
		runtime.ActiveAgents == 0 &&
		len(runtime.Platforms) == 0 &&
		len(runtime.TokenLocks) == 0 &&
		runtime.Proxy == (ProxyRuntimeStatus{}) &&
		runtime.UpdatedAt == ""
}

func renderChannelLine(channel StatusChannel, runtime RuntimeStatus, pairing PairingPlatformStatus) string {
	lifecycle := "unknown"
	if runtime.Platforms != nil {
		if platform, ok := runtime.Platforms[channel.Name]; ok && platform.State != "" {
			lifecycle = string(platform.State)
		}
	}

	pairingState := string(PairingPlatformStateUnpaired)
	pendingCount := 0
	approvedCount := 0
	if pairing.Platform != "" {
		pairingState = string(pairing.State)
		pendingCount = pairing.PendingCount
		approvedCount = pairing.ApprovedCount
	}

	target := channel.Detail
	if target == "" {
		target = "-"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "- %s: lifecycle=%s", channel.Name, lifecycle)
	if runtime.Platforms != nil {
		if platform, ok := runtime.Platforms[channel.Name]; ok && platform.ErrorMessage != "" {
			fmt.Fprintf(&b, " error=%q", platform.ErrorMessage)
		}
	}
	fmt.Fprintf(&b, "; pairing=%s pending=%d approved=%d; target=%s", pairingState, pendingCount, approvedCount, target)
	return b.String()
}

func sortedStatusChannels(channels []StatusChannel) []StatusChannel {
	out := make([]StatusChannel, 0, len(channels))
	for _, channel := range channels {
		channel.Name = strings.TrimSpace(channel.Name)
		channel.Detail = strings.TrimSpace(channel.Detail)
		if channel.Name == "" {
			continue
		}
		out = append(out, channel)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Detail < out[j].Detail
	})
	return out
}

func pairingPlatformMap(platforms []PairingPlatformStatus) map[string]PairingPlatformStatus {
	out := make(map[string]PairingPlatformStatus, len(platforms))
	for _, platform := range platforms {
		if platform.Platform == "" {
			continue
		}
		out[platform.Platform] = platform
	}
	return out
}

func sortedPendingPairingRecords(records []PairingPendingRecord) []PairingPendingRecord {
	out := append([]PairingPendingRecord(nil), records...)
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Platform != right.Platform {
			return left.Platform < right.Platform
		}
		if left.UserID != right.UserID {
			return left.UserID < right.UserID
		}
		if left.AgeSeconds != right.AgeSeconds {
			return left.AgeSeconds < right.AgeSeconds
		}
		return left.Code < right.Code
	})
	return out
}

func sortedApprovedPairingRecords(records []PairingApprovedRecord) []PairingApprovedRecord {
	out := append([]PairingApprovedRecord(nil), records...)
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Platform != right.Platform {
			return left.Platform < right.Platform
		}
		if left.UserID != right.UserID {
			return left.UserID < right.UserID
		}
		return left.UserName < right.UserName
	})
	return out
}

func sortedPairingDegradedEvidence(records []PairingDegradedEvidence) []PairingDegradedEvidence {
	out := append([]PairingDegradedEvidence(nil), records...)
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Platform != right.Platform {
			return left.Platform < right.Platform
		}
		if left.Reason != right.Reason {
			return left.Reason < right.Reason
		}
		if left.UserID != right.UserID {
			return left.UserID < right.UserID
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		return left.Message < right.Message
	})
	return out
}
