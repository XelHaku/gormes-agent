package session

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// fakeTurnCopier is a TurnCopier that records the call and returns a
// configured count or error. The session.Fork orchestration test exercises
// the contract that the metadata helper queries the copier for the turn
// count and writes Metadata{ParentSessionID, LineageKindFork} for the child.
// The transcript-side SQL copy is exercised separately in
// internal/transcript/fork_test.go.
type fakeTurnCopier struct {
	gotParent string
	gotChild  string
	calls     int
	count     int
	err       error
}

func (f *fakeTurnCopier) CopyTurns(_ context.Context, parent, child string) (int, error) {
	f.calls++
	f.gotParent = parent
	f.gotChild = child
	if f.err != nil {
		return 0, f.err
	}
	return f.count, nil
}

func TestSessionFork_CopiesTranscriptAndWritesForkMetadata(t *testing.T) {
	ctx := context.Background()
	m := NewMemMap()
	if err := m.PutMetadata(ctx, Metadata{
		SessionID: "sess-parent",
		Source:    "tui",
		ChatID:    "default",
		UpdatedAt: 100,
	}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	copier := &fakeTurnCopier{count: 4}

	res, err := Fork(ctx, m, copier, ForkRequest{
		ParentSessionID: "sess-parent",
		ChildSessionID:  "sess-child",
		Title:           "refactor path",
	})
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}

	if res.SessionID != "sess-child" {
		t.Fatalf("ForkResult.SessionID = %q, want sess-child", res.SessionID)
	}
	if res.ParentSessionID != "sess-parent" {
		t.Fatalf("ForkResult.ParentSessionID = %q, want sess-parent", res.ParentSessionID)
	}
	if res.Title != "refactor path" {
		t.Fatalf("ForkResult.Title = %q, want refactor path", res.Title)
	}
	if res.TranscriptCopied != 4 {
		t.Fatalf("ForkResult.TranscriptCopied = %d, want 4 (count returned by copier)", res.TranscriptCopied)
	}
	if copier.calls != 1 {
		t.Fatalf("copier.calls = %d, want 1", copier.calls)
	}
	if copier.gotParent != "sess-parent" || copier.gotChild != "sess-child" {
		t.Fatalf("copier saw parent=%q child=%q, want sess-parent/sess-child", copier.gotParent, copier.gotChild)
	}

	child, ok, err := m.GetMetadata(ctx, "sess-child")
	if err != nil {
		t.Fatalf("GetMetadata child: %v", err)
	}
	if !ok {
		t.Fatal("child metadata not persisted")
	}
	if child.ParentSessionID != "sess-parent" {
		t.Fatalf("child ParentSessionID = %q, want sess-parent", child.ParentSessionID)
	}
	if child.LineageKind != LineageKindFork {
		t.Fatalf("child LineageKind = %q, want %q", child.LineageKind, LineageKindFork)
	}
}

func TestSessionFork_CopierFailureLeavesNoMetadata(t *testing.T) {
	ctx := context.Background()
	m := NewMemMap()
	if err := m.PutMetadata(ctx, Metadata{SessionID: "sess-parent", UpdatedAt: 1}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	wantErr := errors.New("copy boom")
	copier := &fakeTurnCopier{err: wantErr}

	if _, err := Fork(ctx, m, copier, ForkRequest{
		ParentSessionID: "sess-parent",
		ChildSessionID:  "sess-child",
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Fork err = %v, want errors.Is(err, %v)", err, wantErr)
	}

	if _, ok, err := m.GetMetadata(ctx, "sess-child"); err != nil {
		t.Fatalf("GetMetadata after copy error: %v", err)
	} else if ok {
		t.Fatal("child metadata persisted despite copy error; want no partial state")
	}
}

func TestSessionFork_RejectsEmptyOrSelfParent(t *testing.T) {
	ctx := context.Background()
	m := NewMemMap()
	cases := []struct {
		name string
		req  ForkRequest
	}{
		{name: "empty parent", req: ForkRequest{ParentSessionID: "", ChildSessionID: "c"}},
		{name: "empty child", req: ForkRequest{ParentSessionID: "p", ChildSessionID: ""}},
		{name: "self parent", req: ForkRequest{ParentSessionID: "x", ChildSessionID: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Fork(ctx, m, &fakeTurnCopier{}, tc.req); err == nil {
				t.Fatalf("Fork(%+v) err = nil, want non-nil", tc.req)
			}
		})
	}
}

func TestSessionFork_ResolveLineageTipIgnoresFork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(path)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	ctx := context.Background()
	if err := m.PutMetadata(ctx, Metadata{SessionID: "sess-parent", UpdatedAt: 10}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}

	res, err := Fork(ctx, m, &fakeTurnCopier{count: 2}, ForkRequest{
		ParentSessionID: "sess-parent",
		ChildSessionID:  "sess-fork",
		Title:           "exploration",
	})
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if res.SessionID != "sess-fork" {
		t.Fatalf("Fork result SessionID = %q, want sess-fork", res.SessionID)
	}

	tip, err := m.ResolveLineageTip(ctx, "sess-parent")
	if err != nil {
		t.Fatalf("ResolveLineageTip: %v", err)
	}
	if tip.LiveSessionID != "sess-parent" {
		t.Fatalf("ResolveLineageTip.LiveSessionID = %q, want sess-parent (fork must not become the live tip)", tip.LiveSessionID)
	}
	if len(tip.Path) != 1 || tip.Path[0] != "sess-parent" {
		t.Fatalf("ResolveLineageTip.Path = %v, want [sess-parent]", tip.Path)
	}
}
