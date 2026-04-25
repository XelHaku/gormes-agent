package gateway

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

func TestManager_SubmitReportsMigratedExpiryFinalizedWithoutMemoryFlushTask(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 25, 17, 45, 0, 0, time.UTC)
	statusPath := filepath.Join(t.TempDir(), "gateway_state.json")
	store := NewRuntimeStatusStore(statusPath)
	store.now = func() time.Time { return now }
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}
	smap := newExpiryMigrationSessionMap("telegram:42", "sess-legacy", session.Metadata{
		SessionID:             "sess-legacy",
		Source:                "telegram",
		ChatID:                "42",
		UserID:                "u-42",
		ExpiryFinalized:       true,
		MigratedMemoryFlushed: true,
	})

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats:  map[string]string{"telegram": "42"},
		SessionMap:    smap,
		RuntimeStatus: store,
		Now:           func() time.Time { return now },
	}, fk, slog.Default())
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
		Text:     "resume this",
	})
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})

	submits := fk.submitsSnapshot()
	if len(submits) != 1 {
		t.Fatalf("kernel submits len = %d, want exactly one normal submit: %#v", len(submits), submits)
	}
	got := submits[0]
	if got.SessionID != "sess-legacy" {
		t.Fatalf("submitted SessionID = %q, want sess-legacy", got.SessionID)
	}
	if got.Text != "resume this" {
		t.Fatalf("submitted text = %q, want unchanged user text", got.Text)
	}
	if strings.Contains(strings.ToLower(got.Text+"\n"+got.SessionContext), "memory") {
		t.Fatalf("submit path included memory-flush language:\ntext=%s\ncontext=%s", got.Text, got.SessionContext)
	}

	status, err := store.ReadRuntimeStatus(ctx)
	if err != nil {
		t.Fatalf("ReadRuntimeStatus: %v", err)
	}
	if len(status.ExpiryFinalized) != 1 {
		t.Fatalf("ExpiryFinalized status = %+v, want one migrated evidence row", status.ExpiryFinalized)
	}
	evidence := status.ExpiryFinalized[0]
	if evidence.SessionID != "sess-legacy" ||
		!evidence.ExpiryFinalized ||
		!evidence.MigratedMemoryFlushed {
		t.Fatalf("expiry finalized evidence = %+v, want migrated finalized sess-legacy", evidence)
	}

	rawStatus, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read raw runtime status: %v", err)
	}
	if bytes.Contains(rawStatus, []byte(`"memory_flushed"`)) {
		t.Fatalf("runtime status emitted legacy memory_flushed key:\n%s", rawStatus)
	}
}

func TestRenderStatusSummary_ReportsMigratedMemoryFlushedSeparatelyFromExpiryFinalized(t *testing.T) {
	got := RenderStatusSummary(StatusSummary{
		Runtime: RuntimeStatus{
			Kind:         "gormes-gateway",
			GatewayState: GatewayStateRunning,
			ExpiryFinalized: []RuntimeExpiryFinalizedEvidence{
				{
					SessionID:             "sess-legacy",
					Source:                "telegram",
					ChatID:                "42",
					UserID:                "u-42",
					ExpiryFinalized:       true,
					MigratedMemoryFlushed: true,
				},
			},
		},
	})

	for _, want := range []string{
		"session_expiry:",
		"- finalized session=sess-legacy source=telegram chat=42 user=u-42 expiry_finalized=true migrated_memory_flushed=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderStatusSummary missing %q\n%s", want, got)
		}
	}
}

type expiryMigrationSessionMap struct {
	mu        sync.Mutex
	key       string
	sessionID string
	meta      session.Metadata
}

func newExpiryMigrationSessionMap(key, sessionID string, meta session.Metadata) *expiryMigrationSessionMap {
	return &expiryMigrationSessionMap{
		key:       key,
		sessionID: sessionID,
		meta:      meta,
	}
}

func (m *expiryMigrationSessionMap) Get(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if key != m.key {
		return "", nil
	}
	return m.sessionID, nil
}

func (m *expiryMigrationSessionMap) Put(ctx context.Context, key, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if key == m.key {
		m.sessionID = sessionID
	}
	return nil
}

func (m *expiryMigrationSessionMap) Close() error { return nil }

func (m *expiryMigrationSessionMap) GetMetadata(ctx context.Context, sessionID string) (session.Metadata, bool, error) {
	if err := ctx.Err(); err != nil {
		return session.Metadata{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if sessionID != m.meta.SessionID {
		return session.Metadata{}, false, nil
	}
	return m.meta, true, nil
}
