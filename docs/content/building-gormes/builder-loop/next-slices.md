---
title: "Next Slices"
weight: 30
aliases:
  - /building-gormes/next-slices/
---

# Next Slices

This page is generated from the canonical progress file and lists the highest
leverage contract-bearing roadmap rows to execute next.

The ordering is:

1. unblocked `P0` handoffs;
2. active `in_progress` rows;
3. `fixture_ready` rows;
4. unblocked rows that unblock other slices;
5. remaining `draft` contract rows.

Use this page when choosing implementation work. If a row is too broad, split
the row in `progress.json` before assigning it.

<!-- PROGRESS:START kind=next-slices -->
| Phase | Slice | Contract | Trust class | Fixture | Why now |
|---|---|---|---|---|---|
| 4 / 4.H | Provider rate guard — degraded-state + last-known-good evidence | TDD packet for a missing pure state-transition helper. The prerequisite (RateLimitClass + Classify429 in internal/hermes/provider_rate_guard.go) is shipped on main as of commit a1d7d928; do NOT stub or duplicate those constants. STEP 1: write internal/hermes/provider_rate_guard_degraded_test.go in package hermes (importing only `testing` and `time`). Define t0 and t1 once at the top of TestApplyClassification: `t0 := time.Date(2026,4,26,12,0,0,0,time.UTC); t1 := t0.Add(5*time.Minute)`. Add six t.Run subtests in this order — preserves_last_known_when_insufficient, treats_empty_class_as_insufficient, fresh_genuine_quota_clears_unavailable, fresh_upstream_capacity_clears_unavailable, input_immutability_via_struct_value, transitions_back_to_available_on_fresh_evidence — each calling ApplyClassification with explicit GuardState literals and asserting the returned struct field-by-field with t.Fatalf on mismatch. STEP 2: write internal/hermes/provider_rate_guard_degraded.go in package hermes (importing only `time`). Define `type GuardState struct { LastKnownClass RateLimitClass; LastKnownAt time.Time; Unavailable bool }` and `func ApplyClassification(state GuardState, now time.Time, class RateLimitClass) GuardState`. Algorithm: if class == RateLimitInsufficientEvidence OR class == RateLimitClass(""), return GuardState{LastKnownClass: state.LastKnownClass, LastKnownAt: state.LastKnownAt, Unavailable: true}. Otherwise return GuardState{LastKnownClass: class, LastKnownAt: now, Unavailable: false}. The helper takes its argument by value, never mutates it, never reads time.Now, never spawns a goroutine, and never touches retry/breaker code. | system | `internal/hermes/provider_rate_guard_degraded_test.go (new file)::TestApplyClassification/preserves_last_known_when_insufficient` | Unblocks Provider rate guard + budget telemetry. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
| 5 / 5.O | CLI profile name validator | internal/cli adds a pure function `ValidateProfileName(name string) error` and an exported sentinel error set: ErrProfileNameEmpty, ErrProfileNameTooLong, ErrProfileNameInvalidChars, ErrProfileNameReserved; the function accepts names matching `^[a-z0-9][a-z0-9_-]{0,63}$`, treats 'default' as valid (special alias), and rejects the reserved subcommand names {'create','delete','list','use','export','import','show'} | operator, system | `internal/cli/profile_name_test.go` | Unblocks CLI active-profile store, CLI profile root resolver. |
| 5 / 5.O | doctorCustomEndpointReadiness check function | cmd/gormes adds a pure function `doctorCustomEndpointReadiness(cfg config.Config) doctor.CheckResult` that returns Name='Custom endpoint', Status=Pass when Hermes.Endpoint and Hermes.APIKey and Hermes.Model are all non-empty, Status=Warn when any one is missing (with itemized evidence), and Status=Fail when Endpoint is set but Model is empty; doctorCmd RunE invokes this function after the existing Goncho/Slack checks; --offline still skips network probes elsewhere | operator, system | `cmd/gormes/doctor_custom_provider_test.go` | Unblocks CLI status summary over native stores. |
| 5 / 5.O | Custom provider model-switch credential preservation | internal/cli adds a pure function `ResolveCustomProviderSecret(ref CustomProviderRef, env map[string]string) (CustomProviderResolution, error)` where CustomProviderRef has fields {Name string, BaseURL string, APIKey string, KeyEnv string} and CustomProviderResolution has fields {EffectiveSecret string, PersistAsRef string, Evidence string}; the function reads env-template `${VAR}` from APIKey via env, prefers KeyEnv when APIKey is empty, and never returns plaintext in PersistAsRef when the input was a reference | operator, system | `internal/cli/custom_provider_secret_test.go` | Unblocks CLI command registry parity + active-turn busy policy. |
| 5 / 5.F | [IMPORTANT:] prompt prefix for cron and skill commands | internal/cron.CronHeartbeatPrefix and internal/skills.BuildSkillSlashCommandMessage emit `[IMPORTANT:` instead of `[SYSTEM:` so Azure OpenAI Default/DefaultV2 content filters do not reject Gormes prompts as prompt-injection (HTTP 400) — same semantic meta-instruction, different bracketed marker; tests update in lockstep so the byte-match assertions still cover drift | operator, system | `internal/cron/heartbeat_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.Q | TUI TerminalNativeSelectionHelp constant + help-string fixture | internal/tui declares an exported string constant TerminalNativeSelectionHelp = 'Selection: use your terminal's native selection (Shift-drag in most terminals; iTerm Cmd-drag, tmux copy-mode). Gormes does not advertise an in-app copy hotkey.' and a pure helper SelectionHelpLine() that returns it; one fixture asserts the constant exists, mentions 'terminal' but not 'Cmd+C'/'Ctrl+C'/'Ctrl-Shift-C'/'OSC 52'/'clipboard hotkey'/'Ink', and another asserts no advertised copy shortcut leaks anywhere else in the package | operator | `internal/tui/selection_help_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
<!-- PROGRESS:END -->
