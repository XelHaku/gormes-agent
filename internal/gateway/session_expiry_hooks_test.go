package gateway

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

func TestSessionExpiryHooks_FinalizeAndCachedAgentCleanupRunOnceAndPersistAcrossReload(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 25, 18, 5, 0, 0, time.UTC)
	sessionPath := filepath.Join(t.TempDir(), "sessions.db")
	smap := openSessionExpiryBolt(t, sessionPath)

	if err := smap.Put(ctx, "telegram:42", "sess-expired"); err != nil {
		t.Fatalf("Put session mapping: %v", err)
	}
	if err := smap.PutMetadata(ctx, session.Metadata{
		SessionID: "sess-expired",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "u-42",
		UpdatedAt: now.Add(-2 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("PutMetadata expired session: %v", err)
	}

	status := NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))
	status.now = func() time.Time { return now }
	scanner := &sessionExpiryScannerFixture{
		store: smap,
		keys:  map[string]string{"telegram:42": "sess-expired"},
	}
	hooks := &sessionExpiryHookFixture{}
	m := NewManagerWithSubmitter(ManagerConfig{
		SessionMap:    smap,
		RuntimeStatus: status,
		SessionExpiry: SessionExpiryConfig{
			Scanner:   scanner,
			Finalizer: hooks,
		},
		Now: func() time.Time { return now },
	}, &fakeKernel{}, slog.Default())

	if err := m.FinalizeExpiredSessions(ctx); err != nil {
		t.Fatalf("FinalizeExpiredSessions first sweep: %v", err)
	}
	if got := hooks.finalizeSnapshot(); len(got) != 1 {
		t.Fatalf("finalize calls = %+v, want exactly one call", got)
	} else if got[0].SessionKey != "telegram:42" || got[0].SessionID != "sess-expired" || got[0].Platform != "telegram" {
		t.Fatalf("finalize event = %+v, want telegram sess-expired", got[0])
	}
	if got := hooks.cleanupSnapshot(); len(got) != 1 {
		t.Fatalf("cleanup calls = %+v, want exactly one call", got)
	}

	meta, ok, err := smap.GetMetadata(ctx, "sess-expired")
	if err != nil {
		t.Fatalf("GetMetadata finalized: %v", err)
	}
	if !ok {
		t.Fatal("finalized metadata missing")
	}
	if !meta.ExpiryFinalized {
		t.Fatalf("ExpiryFinalized = false, want true after successful finalization: %+v", meta)
	}
	if meta.ExpiryFinalizeStatus != session.ExpiryFinalizeStatusFinalized || meta.ExpiryFinalizeAttempts != 1 {
		t.Fatalf("finalize metadata = status %q attempts %d, want finalized/1: %+v", meta.ExpiryFinalizeStatus, meta.ExpiryFinalizeAttempts, meta)
	}

	smap.Close()
	smap = openSessionExpiryBolt(t, sessionPath)
	defer smap.Close()
	scanner.store = smap
	m.cfg.SessionMap = smap

	if err := m.FinalizeExpiredSessions(ctx); err != nil {
		t.Fatalf("FinalizeExpiredSessions reload sweep: %v", err)
	}
	if got := hooks.finalizeSnapshot(); len(got) != 1 {
		t.Fatalf("finalize calls after reload = %+v, want still one call", got)
	}
	if got := hooks.cleanupSnapshot(); len(got) != 1 {
		t.Fatalf("cleanup calls after reload = %+v, want still one call", got)
	}

	runtime, err := status.ReadRuntimeStatus(ctx)
	if err != nil {
		t.Fatalf("ReadRuntimeStatus: %v", err)
	}
	if len(runtime.ExpiryFinalized) != 1 || runtime.ExpiryFinalized[0].SessionID != "sess-expired" {
		t.Fatalf("ExpiryFinalized evidence = %+v, want one sess-expired row", runtime.ExpiryFinalized)
	}
	finalizeEvidence := latestExpiryFinalizeEvidence(runtime.ExpiryFinalize)
	if finalizeEvidence.Status != string(session.ExpiryFinalizeStatusFinalized) || finalizeEvidence.Attempts != 1 {
		t.Fatalf("latest expiry finalize evidence = %+v, want finalized attempts=1", finalizeEvidence)
	}
}

func TestSessionExpiryHooks_FailedFinalizationRetriesThreeTimesThenGivesUp(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 25, 18, 10, 0, 0, time.UTC)
	smap := session.NewMemMap()
	if err := smap.PutMetadata(ctx, session.Metadata{
		SessionID: "sess-retry",
		Source:    "telegram",
		ChatID:    "42",
		UserID:    "u-42",
		UpdatedAt: now.Add(-2 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("PutMetadata retry session: %v", err)
	}

	status := NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))
	status.now = func() time.Time { return now }
	scanner := &sessionExpiryScannerFixture{
		store: smap,
		keys:  map[string]string{"telegram:42": "sess-retry"},
	}
	hooks := &sessionExpiryHookFixture{finalizeErr: errors.New("plugin unavailable")}
	kernel := &fakeKernel{}
	m := NewManagerWithSubmitter(ManagerConfig{
		SessionMap:    smap,
		RuntimeStatus: status,
		SessionExpiry: SessionExpiryConfig{
			Scanner:             scanner,
			Finalizer:           hooks,
			MaxFinalizeAttempts: 3,
		},
		Now: func() time.Time { return now },
	}, kernel, slog.Default())

	for attempt := 1; attempt <= 3; attempt++ {
		if err := m.FinalizeExpiredSessions(ctx); err != nil {
			t.Fatalf("FinalizeExpiredSessions attempt %d: %v", attempt, err)
		}
		now = now.Add(time.Minute)
	}
	if err := m.FinalizeExpiredSessions(ctx); err != nil {
		t.Fatalf("FinalizeExpiredSessions post give-up sweep: %v", err)
	}

	if got := hooks.finalizeSnapshot(); len(got) != 3 {
		t.Fatalf("finalize calls = %+v, want three bounded attempts", got)
	}
	if got := hooks.cleanupSnapshot(); len(got) != 0 {
		t.Fatalf("cleanup calls = %+v, want none when finalize hook fails", got)
	}
	if got := kernel.submitsSnapshot(); len(got) != 0 || kernel.resets != 0 {
		t.Fatalf("kernel work = submits %+v resets %d, want no hidden memory-flush or reset tasks", got, kernel.resets)
	}

	meta, ok, err := smap.GetMetadata(ctx, "sess-retry")
	if err != nil {
		t.Fatalf("GetMetadata gave-up: %v", err)
	}
	if !ok {
		t.Fatal("gave-up metadata missing")
	}
	if !meta.ExpiryFinalized {
		t.Fatalf("ExpiryFinalized = false, want true after give-up to prevent spin: %+v", meta)
	}
	if meta.ExpiryFinalizeStatus != session.ExpiryFinalizeStatusGaveUp || meta.ExpiryFinalizeAttempts != 3 {
		t.Fatalf("finalize metadata = status %q attempts %d, want gave-up/3: %+v", meta.ExpiryFinalizeStatus, meta.ExpiryFinalizeAttempts, meta)
	}

	runtime, err := status.ReadRuntimeStatus(ctx)
	if err != nil {
		t.Fatalf("ReadRuntimeStatus: %v", err)
	}
	statuses := expiryFinalizeStatuses(runtime.ExpiryFinalize)
	for _, want := range []session.ExpiryFinalizeStatus{
		session.ExpiryFinalizeStatusPending,
		session.ExpiryFinalizeStatusFailed,
		session.ExpiryFinalizeStatusGaveUp,
	} {
		if !statuses[want] {
			t.Fatalf("ExpiryFinalize statuses = %+v, missing %s in %+v", statuses, want, runtime.ExpiryFinalize)
		}
	}
	latest := latestExpiryFinalizeEvidence(runtime.ExpiryFinalize)
	if latest.Status != string(session.ExpiryFinalizeStatusGaveUp) || latest.Attempts != 3 {
		t.Fatalf("latest expiry finalize evidence = %+v, want gave-up attempts=3", latest)
	}
}

func TestSessionExpiryHooks_StatusSummaryRendersRetryEvidenceWithAttemptCounts(t *testing.T) {
	got := RenderStatusSummary(StatusSummary{
		Runtime: RuntimeStatus{
			Kind:         "gormes-gateway",
			GatewayState: GatewayStateRunning,
			ExpiryFinalize: []RuntimeExpiryFinalizeEvidence{
				{SessionID: "sess-pending", Source: "telegram", ChatID: "42", Status: string(session.ExpiryFinalizeStatusPending), Attempts: 0},
				{SessionID: "sess-failed", Source: "telegram", ChatID: "43", Status: string(session.ExpiryFinalizeStatusFailed), Attempts: 1, Error: "temporary hook failure"},
				{SessionID: "sess-gave-up", Source: "telegram", ChatID: "44", Status: string(session.ExpiryFinalizeStatusGaveUp), Attempts: 3, Error: "still failing"},
			},
		},
	})

	for _, want := range []string{
		"- expiry_finalize_pending session=sess-pending source=telegram chat=42 attempts=0",
		"- expiry_finalize_failed session=sess-failed source=telegram chat=43 attempts=1 error=\"temporary hook failure\"",
		"- expiry_finalize_gave_up session=sess-gave-up source=telegram chat=44 attempts=3 error=\"still failing\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderStatusSummary missing %q\n%s", want, got)
		}
	}
}

func TestSessionExpiryHooks_ResumeSubmitDoesNotLaunchMemoryFlushOrExtractorWork(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 25, 18, 20, 0, 0, time.UTC)
	smap := session.NewMemMap()
	if err := smap.Put(ctx, "telegram:42", "sess-finalized"); err != nil {
		t.Fatalf("Put session mapping: %v", err)
	}
	if err := smap.PutMetadata(ctx, session.Metadata{
		SessionID:                    "sess-finalized",
		Source:                       "telegram",
		ChatID:                       "42",
		UserID:                       "u-42",
		ExpiryFinalized:              true,
		ExpiryFinalizeStatus:         session.ExpiryFinalizeStatusFinalized,
		ExpiryFinalizeAttempts:       1,
		ExpiryFinalizeLastEvidenceAt: now.Add(-time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("PutMetadata finalized session: %v", err)
	}

	tg := newFakeChannel("telegram")
	kernel := &fakeKernel{}
	hooks := &sessionExpiryHookFixture{}
	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		SessionMap:   smap,
		SessionExpiry: SessionExpiryConfig{
			Scanner:   &sessionExpiryScannerFixture{store: smap, keys: map[string]string{"telegram:42": "sess-finalized"}},
			Finalizer: hooks,
		},
		Now: func() time.Time { return now },
	}, kernel, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	runManagerForResumeTest(t, m)

	tg.pushInbound(InboundEvent{
		Platform: "telegram",
		ChatID:   "42",
		UserID:   "u-42",
		MsgID:    "m1",
		Kind:     EventSubmit,
		Text:     "resume without side work",
	})
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(kernel.submitsSnapshot()) == 1
	})

	got := kernel.submitsSnapshot()[0]
	if got.SessionID != "sess-finalized" {
		t.Fatalf("submitted SessionID = %q, want sess-finalized", got.SessionID)
	}
	combined := strings.ToLower(got.Text + "\n" + got.SessionContext)
	for _, forbidden := range []string{"memory flush", "flush_memories", "goncho", "honcho"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("submit path launched or referenced forbidden side work %q:\ntext=%s\ncontext=%s", forbidden, got.Text, got.SessionContext)
		}
	}
	if got := hooks.finalizeSnapshot(); len(got) != 0 {
		t.Fatalf("finalize hooks during resume submit = %+v, want none", got)
	}
	if got := hooks.cleanupSnapshot(); len(got) != 0 {
		t.Fatalf("cleanup hooks during resume submit = %+v, want none", got)
	}
}

type sessionExpiryScannerFixture struct {
	store interface {
		GetMetadata(context.Context, string) (session.Metadata, bool, error)
	}
	keys map[string]string
}

func (s *sessionExpiryScannerFixture) ExpiredSessions(ctx context.Context, _ time.Time) ([]SessionExpiryCandidate, error) {
	out := make([]SessionExpiryCandidate, 0, len(s.keys))
	for key, sessionID := range s.keys {
		meta, ok, err := s.store.GetMetadata(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, SessionExpiryCandidate{SessionKey: key, Metadata: meta})
	}
	return out, nil
}

type sessionExpiryHookFixture struct {
	mu          sync.Mutex
	finalizeErr error
	cleanupErr  error
	finalize    []SessionExpiryEvent
	cleanup     []SessionExpiryEvent
}

func (h *sessionExpiryHookFixture) FinalizeExpiredSession(_ context.Context, ev SessionExpiryEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finalize = append(h.finalize, ev)
	return h.finalizeErr
}

func (h *sessionExpiryHookFixture) CleanupCachedAgent(_ context.Context, ev SessionExpiryEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanup = append(h.cleanup, ev)
	return h.cleanupErr
}

func (h *sessionExpiryHookFixture) finalizeSnapshot() []SessionExpiryEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]SessionExpiryEvent, len(h.finalize))
	copy(out, h.finalize)
	return out
}

func (h *sessionExpiryHookFixture) cleanupSnapshot() []SessionExpiryEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]SessionExpiryEvent, len(h.cleanup))
	copy(out, h.cleanup)
	return out
}

func openSessionExpiryBolt(t *testing.T, path string) *session.BoltMap {
	t.Helper()
	m, err := session.OpenBolt(path)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	return m
}

func latestExpiryFinalizeEvidence(records []RuntimeExpiryFinalizeEvidence) RuntimeExpiryFinalizeEvidence {
	if len(records) == 0 {
		return RuntimeExpiryFinalizeEvidence{}
	}
	return records[len(records)-1]
}

func expiryFinalizeStatuses(records []RuntimeExpiryFinalizeEvidence) map[session.ExpiryFinalizeStatus]bool {
	out := make(map[session.ExpiryFinalizeStatus]bool, len(records))
	for _, record := range records {
		out[session.ExpiryFinalizeStatus(record.Status)] = true
	}
	return out
}
