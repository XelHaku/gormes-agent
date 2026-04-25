package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPtyAdapterRejectsUnavailablePlatformBeforeSpawn(t *testing.T) {
	spawnCalled := false

	_, err := NewPtyAdapter(context.Background(), PtySpawnRequest{
		Argv: []string{"/bin/sh", "-c", "printf unsafe"},
	}, PtyAdapterConfig{
		RuntimeGOOS: "windows",
		Spawn: func(context.Context, PtySpawnRequest) (PtySession, error) {
			spawnCalled = true
			return nil, nil
		},
	})

	if !errors.Is(err, ErrPtyUnavailable) {
		t.Fatalf("err = %v, want ErrPtyUnavailable", err)
	}
	if spawnCalled {
		t.Fatal("spawn was called for an unavailable platform")
	}
}

func TestPtyAdapterReadBoundsTimeout(t *testing.T) {
	bridge := startTestPTY(t, PtySpawnRequest{
		Argv: []string{"/bin/sh", "-c", "sleep 0.2; printf late"},
	})

	start := time.Now()
	chunk, err := bridge.Read(25*time.Millisecond, 8)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Read returned err = %v, want nil timeout result", err)
	}
	if len(chunk) != 0 {
		t.Fatalf("Read returned %q before child wrote anything", chunk)
	}
	if elapsed > 150*time.Millisecond {
		t.Fatalf("Read took %v, want bounded near the requested timeout", elapsed)
	}
}

func TestPtyAdapterReadBoundsChunkSize(t *testing.T) {
	bridge := startTestPTY(t, PtySpawnRequest{
		Argv: []string{"/bin/sh", "-c", "printf abcdef"},
	})

	chunk := readFirstChunk(t, bridge, 3)

	if len(chunk) > 3 {
		t.Fatalf("len(chunk) = %d, want <= 3", len(chunk))
	}
}

func TestPtyAdapterWriteSendsBytesToChild(t *testing.T) {
	bridge := startTestPTY(t, PtySpawnRequest{
		Argv: []string{"/bin/cat"},
	})

	if err := bridge.HandleClientMessage([]byte("hello-pty\n")); err != nil {
		t.Fatalf("HandleClientMessage(write) err = %v", err)
	}

	out := readUntil(t, bridge, []byte("hello-pty"), 2*time.Second)
	if !bytes.Contains(out, []byte("hello-pty")) {
		t.Fatalf("output = %q, want echoed write", out)
	}
}

func TestPtyAdapterRejectsInvalidWriteBeforeSession(t *testing.T) {
	session := &recordingPtySession{}
	bridge := NewPtyAdapterForSession(session)

	if err := bridge.Write(nil); !errors.Is(err, ErrInvalidPtyMessage) {
		t.Fatalf("Write(nil) err = %v, want ErrInvalidPtyMessage", err)
	}
	if len(session.writes) != 0 {
		t.Fatalf("writes reached session: %q", session.writes)
	}

	tooLarge := bytes.Repeat([]byte("x"), MaxPtyWriteBytes+1)
	if err := bridge.Write(tooLarge); !errors.Is(err, ErrInvalidPtyMessage) {
		t.Fatalf("Write(tooLarge) err = %v, want ErrInvalidPtyMessage", err)
	}
	if len(session.writes) != 0 {
		t.Fatalf("oversized write reached session: %q", session.writes)
	}
}

func TestPtyAdapterResizeMessageUpdatesChildWinsize(t *testing.T) {
	bridge := startTestPTY(t, PtySpawnRequest{
		Argv: []string{"/bin/sh", "-c", "sleep 0.1; stty size"},
		Cols: 80,
		Rows: 24,
	})

	if err := bridge.HandleClientMessage([]byte("\x1b[RESIZE:123;45]")); err != nil {
		t.Fatalf("HandleClientMessage(resize) err = %v", err)
	}

	out := readUntil(t, bridge, []byte("45 123"), 2*time.Second)
	if !bytes.Contains(out, []byte("45 123")) {
		t.Fatalf("output = %q, want resized rows/cols", out)
	}
}

func TestPtyAdapterRejectsInvalidResizeBeforeSession(t *testing.T) {
	session := &recordingPtySession{}
	bridge := NewPtyAdapterForSession(session)

	if err := bridge.HandleClientMessage([]byte("\x1b[RESIZE:0;24]")); !errors.Is(err, ErrInvalidPtyMessage) {
		t.Fatalf("HandleClientMessage(invalid resize) err = %v, want ErrInvalidPtyMessage", err)
	}
	if len(session.resizes) != 0 {
		t.Fatalf("resize reached session: %+v", session.resizes)
	}
	if len(session.writes) != 0 {
		t.Fatalf("invalid resize was written to PTY: %q", session.writes)
	}
}

func TestPtyAdapterCloseTerminatesChild(t *testing.T) {
	bridge := startTestPTY(t, PtySpawnRequest{
		Argv: []string{"/bin/sh", "-c", "sleep 30"},
	})

	if bridge.PID() <= 0 {
		t.Fatalf("PID() = %d, want child pid", bridge.PID())
	}
	if err := bridge.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if err := bridge.Close(); err != nil {
		t.Fatalf("second Close err = %v", err)
	}
	if bridge.IsAlive() {
		t.Fatal("bridge reports child alive after Close")
	}
}

func startTestPTY(t *testing.T, req PtySpawnRequest) *PtyAdapter {
	t.Helper()

	if runtime.GOOS != "linux" {
		t.Skip("real PTY fixture is linux-only in this slice")
	}

	bridge, err := NewPtyAdapter(context.Background(), req, PtyAdapterConfig{})
	if errors.Is(err, ErrPtyUnavailable) {
		t.Skipf("PTY unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("NewPtyAdapter err = %v", err)
	}
	t.Cleanup(func() {
		if err := bridge.Close(); err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("cleanup Close err = %v", err)
		}
	})

	return bridge
}

func readFirstChunk(t *testing.T, bridge *PtyAdapter, maxBytes int) []byte {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		chunk, err := bridge.Read(100*time.Millisecond, maxBytes)
		if err == nil && len(chunk) > 0 {
			return chunk
		}
		if errors.Is(err, io.EOF) {
			t.Fatal("PTY reached EOF before emitting a chunk")
		}
		if err != nil {
			t.Fatalf("Read err = %v", err)
		}
	}

	t.Fatal("timed out waiting for PTY output")
	return nil
}

func readUntil(t *testing.T, bridge *PtyAdapter, needle []byte, timeout time.Duration) []byte {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var out []byte
	for time.Now().Before(deadline) {
		chunk, err := bridge.Read(100*time.Millisecond, DefaultPtyReadChunkSize)
		if len(chunk) > 0 {
			out = append(out, chunk...)
			if bytes.Contains(out, needle) {
				return out
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Read err = %v", err)
		}
	}

	t.Fatalf("timed out waiting for %q in %q", needle, printablePTYOutput(out))
	return out
}

func printablePTYOutput(out []byte) string {
	return strings.ReplaceAll(string(out), "\r", "\\r")
}

type recordingPtySession struct {
	writes  [][]byte
	resizes []PtySize
	closed  bool
}

func (s *recordingPtySession) Read(time.Duration, int) ([]byte, error) {
	return []byte{}, nil
}

func (s *recordingPtySession) Write(data []byte) error {
	s.writes = append(s.writes, append([]byte(nil), data...))
	return nil
}

func (s *recordingPtySession) Resize(cols, rows int) error {
	s.resizes = append(s.resizes, PtySize{Cols: cols, Rows: rows})
	return nil
}

func (s *recordingPtySession) Close() error {
	s.closed = true
	return nil
}

func (s *recordingPtySession) IsAlive() bool {
	return !s.closed
}

func (s *recordingPtySession) PID() int {
	return 123
}
