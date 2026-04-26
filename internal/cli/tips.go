package cli

// Tips is the fixed corpus of operator-facing tip strings rendered by Gormes
// CLI surfaces (banner footers, doctor hints, idle screens). Entries are owned
// by Gormes — they reference Gormes commands and concepts and intentionally do
// not mirror any upstream tip text. Each entry is unique, non-empty, and free
// of newline characters so callers can render a tip on a single line without
// further sanitisation.
var Tips = []string{
	"Run `gormes doctor` to verify your local setup before opening a support ticket.",
	"Use `gormes session export <id>` to share a session transcript without leaking secrets.",
	"`gormes gateway status` reports live gateway health, including queue depth and credential locks.",
	"Set GORMES_LOG_LEVEL=debug to see the same diagnostics the support summary captures.",
	"`gormes memory status` summarises AgentDB usage, including HNSW index freshness.",
	"`gormes version` prints the build version, release date, and upstream/local git state when available.",
	"Run `gormes goncho doctor` to validate Goncho coordinator connectivity end-to-end.",
	"Pair the CLI with `--toolset` to scope a session to a specific tool surface (e.g. read-only review).",
	"Use `gormes telegram` to drive Telegram-bound flows from the same binary that powers the gateway.",
	"`gormes gateway` accepts a config file path so multiple profiles can run side by side.",
	"Toolset names ending in `_tools` are normalised for display; the underlying config remains unchanged.",
	"Add `--help` to any subcommand to see its flags before reaching for documentation.",
	"Banner version labels include `+N carried commits` when your local branch leads its upstream.",
	"Long context windows render as `128K` or `1M`; the raw token count is still in the configuration.",
	"Set GORMES_CONFIG to point at an alternate config root when testing changes against a real session store.",
	"`gormes session` lists known sessions, including their lineage_kind, so you can spot orphaned children.",
	"Lineage tips resolve append-only compression descendants without rewriting ancestor metadata.",
	"Gateway credential locks are scoped per platform plus credential hash, so unrelated tokens stay independent.",
	"Inject `time.Now().UnixNano()` into TipFor when you actually want a random tip; tests should pin the seed.",
	"The CLI banner is pure text — wire it through FormatBannerVersionLabel rather than reformatting at call sites.",
	"`go test ./internal/cli -count=1` is the canonical local check before committing CLI helper changes.",
	"`go vet ./internal/cli` catches the printf and shadowing mistakes that slip past compilation.",
	"`go run ./cmd/builder-loop progress validate` keeps the architecture progress file honest.",
	"Use FormatInfo, FormatSuccess, FormatWarning, and FormatError so operators see consistent line prefixes.",
	"Reach for FormatYesNoPrompt and ResolveYesNoAnswer instead of hand-rolling boolean prompt parsing.",
	"FormatPrompt renders an optional default in brackets; ResolvePromptInput trims and falls back for you.",
	"DisplayToolsetName turns blanks into `unknown`, so empty configs still produce a readable banner line.",
	"Service restart polling lives in service_restart.go; reuse it rather than spawning ad-hoc shell loops.",
	"PTY bridging is gated per platform — non-Linux builds compile against pty_bridge_unsupported.go on purpose.",
	"Effective toolsets are derived once and cached; treat them as read-only outside of toolset_config.go.",
	"Banner formatting helpers are file, subprocess, and network inert — keep new helpers that way.",
	"When a slice grows past one helper file plus its test fixture, consider splitting it into sibling rows.",
	"Internal/cli helpers must compile without TTY detection, clock access, config files, or network calls.",
}

// TipFor returns the tip selected by the provided seed. The function is pure:
// the same seed always yields the same tip, so callers can drive deterministic
// rendering in tests by passing a fixed seed and a non-deterministic display
// by passing time.Now().UnixNano(). Negative seeds are normalised before
// indexing so the function never panics on the operator's clock skew or on a
// freshly zero-valued counter.
func TipFor(seed int64) string {
	n := int64(len(Tips))
	idx := seed % n
	if idx < 0 {
		idx += n
	}
	return Tips[idx]
}
