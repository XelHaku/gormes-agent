package cli

const (
	// OpenClawResidueCleanupFlag is the stable onboarding.seen key for the
	// one-time OpenClaw residue cleanup banner.
	OpenClawResidueCleanupFlag = "openclaw_residue_cleanup"
	OpenClawResidueFlag        = OpenClawResidueCleanupFlag
)

// OnboardingSeen reports whether config has onboarding.seen.<flag> set to
// true. With no flag argument, it checks the OpenClaw residue cleanup flag.
// Malformed or missing onboarding maps are treated as unseen.
func OnboardingSeen(config map[string]any, flags ...string) bool {
	seen, ok := onboardingSeenMap(config)
	if !ok {
		return false
	}
	value, ok := seen[onboardingFlag(flags)].(bool)
	return ok && value
}

// MarkOnboardingSeen sets onboarding.seen.<flag> to true in memory and returns
// the corrected config map. With no flag argument, it marks the OpenClaw
// residue cleanup flag.
func MarkOnboardingSeen(config map[string]any, flags ...string) map[string]any {
	if config == nil {
		config = map[string]any{}
	}

	onboarding, ok := config["onboarding"].(map[string]any)
	if !ok {
		onboarding = map[string]any{}
		config["onboarding"] = onboarding
	}

	seen, ok := onboarding["seen"].(map[string]any)
	if !ok {
		seen = map[string]any{}
		onboarding["seen"] = seen
	}

	seen[onboardingFlag(flags)] = true
	return config
}

func onboardingSeenMap(config map[string]any) (map[string]any, bool) {
	onboarding, ok := config["onboarding"].(map[string]any)
	if !ok {
		return nil, false
	}
	seen, ok := onboarding["seen"].(map[string]any)
	return seen, ok
}

func onboardingFlag(flags []string) string {
	if len(flags) == 0 || flags[0] == "" {
		return OpenClawResidueCleanupFlag
	}
	return flags[0]
}
