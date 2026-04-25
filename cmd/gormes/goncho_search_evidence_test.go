package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

func TestGonchoDoctorSearchEvidenceTextIncludesLineageStatus(t *testing.T) {
	seedGonchoDoctorSearchEvidenceFixture(t, true)

	stdout, stderr, err := runGonchoDoctorCommand(t,
		"goncho", "doctor",
		"--peer", "user-juan",
		"--session", "discord:chan-9",
		"--scope", "user",
		"--sources", "telegram",
	)
	if err != nil {
		t.Fatalf("Execute: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, want := range []string{
		"scope_evidence: decision=allowed user_id=user-juan scope=user",
		"source_allowlist=telegram",
		"sessions_considered=3",
		"widened_sessions_considered=3",
		"source=turn origin_source=telegram session_key=sess-child lineage_status=ok",
		"source=turn origin_source=telegram session_key=sess-orphan lineage_status=orphan",
		"parent_session_id=sess-missing lineage_kind=fork",
		"source=turn origin_source=telegram session_key=sess-chat-only lineage_status=unavailable",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestGonchoDoctorSearchEvidenceJSONIncludesLineageStatus(t *testing.T) {
	seedGonchoDoctorSearchEvidenceFixture(t, true)

	stdout, stderr, err := runGonchoDoctorCommand(t,
		"goncho", "doctor",
		"--json",
		"--peer", "user-juan",
		"--session", "discord:chan-9",
		"--scope", "user",
		"--sources", "telegram",
	)
	if err != nil {
		t.Fatalf("Execute: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	got := decodeGonchoDoctorSearchEvidenceJSON(t, stdout)
	if got.ContextDryRun.ScopeEvidence.Decision != "allowed" ||
		got.ContextDryRun.ScopeEvidence.UserID != "user-juan" ||
		got.ContextDryRun.ScopeEvidence.SessionsConsidered != 3 ||
		got.ContextDryRun.ScopeEvidence.WidenedSessionsConsidered != 3 ||
		len(got.ContextDryRun.ScopeEvidence.SourceAllowlist) != 1 ||
		got.ContextDryRun.ScopeEvidence.SourceAllowlist[0] != "telegram" {
		t.Fatalf("scope_evidence = %+v, want allowed telegram evidence", got.ContextDryRun.ScopeEvidence)
	}

	hits := searchEvidenceBySessionKey(got.ContextDryRun.SearchResults)
	assertGonchoDoctorLineageEvidence(t, hits, "sess-child", "telegram", "ok", "sess-parent", "compression")
	assertGonchoDoctorLineageEvidence(t, hits, "sess-orphan", "telegram", "orphan", "sess-missing", "fork")
	assertGonchoDoctorLineageEvidence(t, hits, "sess-chat-only", "telegram", "unavailable", "", "")
}

func TestGonchoDoctorSearchEvidenceJSONReportsMissingSessionDirectoryFallback(t *testing.T) {
	seedGonchoDoctorSearchEvidenceFixture(t, false)

	stdout, stderr, err := runGonchoDoctorCommand(t,
		"goncho", "doctor",
		"--json",
		"--peer", "user-juan",
		"--session", "discord:chan-9",
		"--scope", "user",
		"--sources", "discord",
	)
	if err != nil {
		t.Fatalf("Execute: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	got := decodeGonchoDoctorSearchEvidenceJSON(t, stdout)
	if got.ContextDryRun.ScopeEvidence.Decision != "degraded" ||
		got.ContextDryRun.ScopeEvidence.FallbackScope != "same-chat" ||
		!strings.Contains(got.ContextDryRun.ScopeEvidence.Reason, "session directory unavailable") {
		t.Fatalf("scope_evidence = %+v, want degraded same-chat fallback", got.ContextDryRun.ScopeEvidence)
	}
	if len(got.ContextDryRun.SearchResults) != 1 {
		t.Fatalf("search_results len = %d, want same-chat fallback only: %+v",
			len(got.ContextDryRun.SearchResults), got.ContextDryRun.SearchResults)
	}
	hit := got.ContextDryRun.SearchResults[0]
	if hit.SessionKey != "sess-current" || hit.OriginSource != "discord" || strings.Contains(hit.Content, "remote") {
		t.Fatalf("search result = %+v, want current Discord fallback only", hit)
	}
	if hit.Lineage.Status != "unavailable" {
		t.Fatalf("fallback lineage status = %q, want unavailable", hit.Lineage.Status)
	}
}

type gonchoDoctorSearchEvidenceJSON struct {
	ContextDryRun struct {
		ScopeEvidence struct {
			Decision                  string   `json:"decision"`
			FallbackScope             string   `json:"fallback_scope"`
			Reason                    string   `json:"reason"`
			UserID                    string   `json:"user_id"`
			SourceAllowlist           []string `json:"source_allowlist"`
			SessionsConsidered        int      `json:"sessions_considered"`
			WidenedSessionsConsidered int      `json:"widened_sessions_considered"`
		} `json:"scope_evidence"`
		SearchResults []gonchoDoctorSearchHitJSON `json:"search_results"`
	} `json:"context_dry_run"`
}

type gonchoDoctorSearchHitJSON struct {
	Source       string                        `json:"source"`
	OriginSource string                        `json:"origin_source"`
	SessionKey   string                        `json:"session_key"`
	Content      string                        `json:"content"`
	Lineage      gonchoDoctorSearchLineageJSON `json:"lineage"`
}

type gonchoDoctorSearchLineageJSON struct {
	Status          string   `json:"status"`
	ParentSessionID string   `json:"parent_session_id"`
	LineageKind     string   `json:"lineage_kind"`
	ChildSessionIDs []string `json:"child_session_ids"`
}

func decodeGonchoDoctorSearchEvidenceJSON(t *testing.T, raw string) gonchoDoctorSearchEvidenceJSON {
	t.Helper()

	var got gonchoDoctorSearchEvidenceJSON
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal doctor JSON: %v\n%s", err, raw)
	}
	return got
}

func searchEvidenceBySessionKey(hits []gonchoDoctorSearchHitJSON) map[string]gonchoDoctorSearchHitJSON {
	out := make(map[string]gonchoDoctorSearchHitJSON, len(hits))
	for _, hit := range hits {
		out[hit.SessionKey] = hit
	}
	return out
}

func assertGonchoDoctorLineageEvidence(t *testing.T, hits map[string]gonchoDoctorSearchHitJSON, sessionKey, originSource, status, parentSessionID, lineageKind string) {
	t.Helper()

	hit, ok := hits[sessionKey]
	if !ok {
		t.Fatalf("missing search hit for %s in %+v", sessionKey, hits)
	}
	if hit.Source != "turn" || hit.OriginSource != originSource {
		t.Fatalf("hit %s = %+v, want turn from %s", sessionKey, hit, originSource)
	}
	if hit.Lineage.Status != status ||
		hit.Lineage.ParentSessionID != parentSessionID ||
		hit.Lineage.LineageKind != lineageKind {
		t.Fatalf("hit %s lineage = %+v, want status %q parent %q kind %q",
			sessionKey, hit.Lineage, status, parentSessionID, lineageKind)
	}
}

func seedGonchoDoctorSearchEvidenceFixture(t *testing.T, includeSessionDirectory bool) {
	t.Helper()
	setupGonchoDoctorEnv(t)

	store, err := memory.OpenSqlite(config.MemoryDBPath(), 8, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	ctx := context.Background()
	now := time.Now().Unix()
	for _, turn := range []struct {
		sessionID string
		chatID    string
		content   string
		ts        int64
	}{
		{"sess-current", "discord:chan-9", "doctor dry-run current Discord fallback evidence.", now - 40},
		{"sess-child", "telegram:42", "doctor dry-run child Telegram lineage evidence.", now - 30},
		{"sess-orphan", "telegram:42", "doctor dry-run orphan Telegram lineage evidence.", now - 20},
		{"sess-chat-only", "telegram:42", "doctor dry-run legacy chat-only lineage evidence.", now - 10},
	} {
		if _, err := store.DB().ExecContext(ctx,
			`INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES (?, 'user', ?, ?, ?)`,
			turn.sessionID, turn.content, turn.ts, turn.chatID,
		); err != nil {
			t.Fatalf("insert turn %s: %v", turn.sessionID, err)
		}
	}
	if err := store.Close(ctx); err != nil {
		t.Fatalf("Close memory store: %v", err)
	}

	if !includeSessionDirectory {
		return
	}

	smap, err := session.OpenBolt(config.SessionDBPath())
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	for _, meta := range []session.Metadata{
		{SessionID: "sess-current", Source: "discord", ChatID: "chan-9", UserID: "user-juan"},
		{SessionID: "sess-parent", Source: "telegram", ChatID: "42", UserID: "user-juan"},
		{
			SessionID:       "sess-child",
			Source:          "telegram",
			ChatID:          "42",
			UserID:          "user-juan",
			ParentSessionID: "sess-parent",
			LineageKind:     session.LineageKindCompression,
		},
		{
			SessionID:       "sess-orphan",
			Source:          "telegram",
			ChatID:          "42",
			UserID:          "user-juan",
			ParentSessionID: "sess-missing",
			LineageKind:     session.LineageKindFork,
		},
	} {
		if err := smap.PutMetadata(ctx, meta); err != nil {
			t.Fatalf("PutMetadata(%s): %v", meta.SessionID, err)
		}
	}
	if err := smap.Close(); err != nil {
		t.Fatalf("Close session map: %v", err)
	}
}
