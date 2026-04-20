package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

// TestSmoke_ConcurrentWritersNoLockErrors stresses the single-owner-worker
// design by firing 10 goroutines each calling Exec 500 times. Total 5000
// commands. If the WASM SQLite engine under ncruces ever throws
// "database is locked" under concurrent Exec pressure, this catches it —
// Exec itself never touches *sql.DB directly, so any lock should be
// structurally unreachable. Proves the drop-on-full + single-writer
// architecture holds under load.
func TestSmoke_ConcurrentWritersNoLockErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 8192, nil) // generous buffer so drops are rare

	const goroutines = 10
	const perG = 500
	const total = goroutines * perG

	var wg sync.WaitGroup
	start := time.Now()
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				payload, _ := json.Marshal(map[string]any{
					"session_id": fmt.Sprintf("sess-g%d", gid),
					"content":    fmt.Sprintf("msg %d from goroutine %d", i, gid),
					"ts_unix":    time.Now().UnixNano(),
				})
				_, err := s.Exec(context.Background(), store.Command{
					Kind:    store.AppendUserTurn,
					Payload: payload,
				})
				if err != nil {
					t.Errorf("Exec returned err under concurrent load: %v", err)
				}
			}
		}(g)
	}
	wg.Wait()
	enqueueElapsed := time.Since(start)

	// All enqueues done; now drain via graceful Close.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	totalElapsed := time.Since(start)

	t.Logf("enqueued %d cmds in %v (%.0f cmd/s); full drain+close in %v",
		total, enqueueElapsed, float64(total)/enqueueElapsed.Seconds(), totalElapsed)

	// Reopen and confirm rows landed.
	s2, _ := OpenSqlite(path, 0, nil)
	defer s2.Close(context.Background())

	var n int
	if err := s2.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&n); err != nil {
		t.Fatal(err)
	}
	accepted := int(s.Stats().Accepted)
	drops := int(s.Stats().Drops)
	if accepted+drops != total {
		t.Errorf("Accepted (%d) + Drops (%d) = %d, want %d",
			accepted, drops, accepted+drops, total)
	}
	if n != accepted {
		t.Errorf("rows in turns = %d, but Accepted counter = %d", n, accepted)
	}
	t.Logf("accepted=%d drops=%d rows_on_disk=%d", accepted, drops, n)

	// FTS5 should have indexed every row too.
	var ftsN int
	_ = s2.db.QueryRow("SELECT COUNT(*) FROM turns_fts").Scan(&ftsN)
	if ftsN != n {
		t.Errorf("turns_fts has %d rows, turns has %d — trigger sync broken",
			ftsN, n)
	}
}

// TestSmoke_ExecLatencyPercentiles proves the 250ms rule under load:
// Exec's enqueue operation must remain sub-millisecond even while the
// worker is catching up on backed-up writes. Measures p50 / p99 / max
// latency across 2000 Exec calls fired serially.
func TestSmoke_ExecLatencyPercentiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 2048, nil)
	defer s.Close(context.Background())

	const N = 2000
	latencies := make([]time.Duration, 0, N)
	payload, _ := json.Marshal(map[string]any{
		"session_id": "sess-latency",
		"content":    "a test message whose length is roughly similar to a real turn",
		"ts_unix":    1,
	})

	for i := 0; i < N; i++ {
		start := time.Now()
		_, _ = s.Exec(context.Background(), store.Command{
			Kind:    store.AppendUserTurn,
			Payload: payload,
		})
		latencies = append(latencies, time.Since(start))
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[N/2]
	p99 := latencies[(N*99)/100]
	max := latencies[N-1]

	t.Logf("Exec latency over %d calls: p50=%v p99=%v max=%v", N, p50, p99, max)

	// Hard budget: any Exec must finish far under the 250 ms kernel deadline.
	// Realistic: p50 is microseconds; p99 is sub-millisecond; max may hit
	// a scheduling hiccup but should stay under 50 ms even on CI.
	if p99 > 10*time.Millisecond {
		t.Errorf("p99 Exec latency = %v, want < 10 ms (would jeopardize 250 ms rule under back-pressure)", p99)
	}
	if max > 100*time.Millisecond {
		t.Errorf("max Exec latency = %v, want < 100 ms (still well under 250 ms but this is suspicious)", max)
	}
}

// TestSmoke_WALCheckpointsOnClose proves the durability invariant: after
// Close(), the -wal sidecar file should have been checkpointed back into
// the main memory.db. If the bot crashes mid-turn, the remaining -wal
// would be replayed on next open; we still want clean Close to drain it.
func TestSmoke_WALCheckpointsOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.db")
	s, _ := OpenSqlite(path, 1024, nil)

	// Write enough rows to force the -wal file to exist.
	for i := 0; i < 50; i++ {
		payload, _ := json.Marshal(map[string]any{
			"session_id": "sess-wal",
			"content":    fmt.Sprintf("row %d", i),
			"ts_unix":    int64(i),
		})
		_, _ = s.Exec(context.Background(), store.Command{
			Kind: store.AppendUserTurn, Payload: payload,
		})
	}

	// Wait for worker to drain to disk.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if int(s.Stats().Accepted) >= 50 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// BEFORE Close: -wal file may or may not exist (WAL is lazy). Checkpoint
	// on close is the key invariant.
	walBefore := fileSize(path + "-wal")
	t.Logf("before Close: memory.db=%d bytes  memory.db-wal=%d bytes",
		fileSize(path), walBefore)

	// Graceful close should flush + checkpoint.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	walAfter := fileSize(path + "-wal")
	shmAfter := fileSize(path + "-shm")
	mainAfter := fileSize(path)
	t.Logf("after Close:  memory.db=%d bytes  memory.db-wal=%d bytes  memory.db-shm=%d bytes",
		mainAfter, walAfter, shmAfter)

	// Primary contract: main DB has all the data. A leftover -wal is
	// acceptable (SQLite may retain it zero-length for perf), but if it's
	// larger than the main DB something's wrong.
	if mainAfter == 0 {
		t.Errorf("main DB is 0 bytes after Close — data lost")
	}
	if walAfter > mainAfter && walAfter > 64*1024 {
		t.Errorf("memory.db-wal (%d) > memory.db (%d) after Close — checkpoint may not have run",
			walAfter, mainAfter)
	}

	// Secondary: reopen must see every row.
	s2, _ := OpenSqlite(path, 0, nil)
	defer s2.Close(context.Background())
	var n int
	_ = s2.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&n)
	if n != 50 {
		t.Errorf("after reopen, rows = %d, want 50", n)
	}
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
