package autoloop

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestCleanupStaleLocksRemovesExpiredClaim(t *testing.T) {
	lockRoot := t.TempDir()
	lockDir := filepath.Join(lockRoot, "task.lock")
	claimPath := lockDir + ".claim.json"

	if err := os.Mkdir(lockDir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	claim := `{"pid":1,"claimed_at_epoch":100}`
	if err := os.WriteFile(claimPath, []byte(claim), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	now := func() time.Time { return time.Unix(200, 0) }
	if err := CleanupStaleLocks(lockRoot, time.Minute, now); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Fatalf("lock dir exists after cleanup, stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Fatalf("claim file exists after cleanup, stat error = %v", err)
	}
}

func TestCleanupStaleLocksKeepsLiveUnexpiredClaim(t *testing.T) {
	lockRoot := t.TempDir()
	lockDir := filepath.Join(lockRoot, "task.lock")
	claimPath := lockDir + ".claim.json"
	now := time.Unix(200, 0)

	if err := os.Mkdir(lockDir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	claim := `{"pid":` + strconv.Itoa(os.Getpid()) + `,"claimed_at_epoch":190}`
	if err := os.WriteFile(claimPath, []byte(claim), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return now }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockDir); err != nil {
		t.Fatalf("lock dir stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); err != nil {
		t.Fatalf("claim file stat error = %v", err)
	}
}

func TestCleanupStaleLocksRemovesExpiredRegularFileLock(t *testing.T) {
	lockRoot := t.TempDir()
	lockPath := filepath.Join(lockRoot, "task.lock")
	claimPath := lockPath + ".claim.json"

	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() lock error = %v", err)
	}
	claim := `{"pid":1,"claimed_at_epoch":100}`
	if err := os.WriteFile(claimPath, []byte(claim), 0o644); err != nil {
		t.Fatalf("WriteFile() claim error = %v", err)
	}

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return time.Unix(200, 0) }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file exists after cleanup, stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Fatalf("claim file exists after cleanup, stat error = %v", err)
	}
}

func TestCleanupStaleLocksKeepsLiveUnexpiredRegularFileLock(t *testing.T) {
	lockRoot := t.TempDir()
	lockPath := filepath.Join(lockRoot, "task.lock")
	claimPath := lockPath + ".claim.json"
	now := time.Unix(200, 0)

	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() lock error = %v", err)
	}
	claim := `{"pid":` + strconv.Itoa(os.Getpid()) + `,"claimed_at_epoch":190}`
	if err := os.WriteFile(claimPath, []byte(claim), 0o644); err != nil {
		t.Fatalf("WriteFile() claim error = %v", err)
	}

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return now }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); err != nil {
		t.Fatalf("claim file stat error = %v", err)
	}
}

func TestCleanupStaleLocksRemovesMissingClaim(t *testing.T) {
	lockRoot := t.TempDir()
	lockPath := filepath.Join(lockRoot, "task.lock")
	claimPath := lockPath + ".claim.json"

	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() lock error = %v", err)
	}

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return time.Unix(200, 0) }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file exists after cleanup, stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Fatalf("claim file exists after cleanup, stat error = %v", err)
	}
}

func TestCleanupStaleLocksRemovesMalformedClaim(t *testing.T) {
	lockRoot := t.TempDir()
	lockPath := filepath.Join(lockRoot, "task.lock")
	claimPath := lockPath + ".claim.json"

	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() lock error = %v", err)
	}
	if err := os.WriteFile(claimPath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile() claim error = %v", err)
	}

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return time.Unix(200, 0) }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file exists after cleanup, stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Fatalf("claim file exists after cleanup, stat error = %v", err)
	}
}

func TestCleanupStaleLocksRemovesNonpositiveOrMissingPIDClaims(t *testing.T) {
	cases := map[string]string{
		"nonpositive pid": `{"pid":0,"claimed_at_epoch":190}`,
		"missing pid":     `{"claimed_at_epoch":190}`,
	}

	for name, claim := range cases {
		t.Run(name, func(t *testing.T) {
			lockRoot := t.TempDir()
			lockPath := filepath.Join(lockRoot, "task.lock")
			claimPath := lockPath + ".claim.json"

			if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
				t.Fatalf("WriteFile() lock error = %v", err)
			}
			if err := os.WriteFile(claimPath, []byte(claim), 0o644); err != nil {
				t.Fatalf("WriteFile() claim error = %v", err)
			}

			if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return time.Unix(200, 0) }); err != nil {
				t.Fatalf("CleanupStaleLocks() error = %v", err)
			}

			if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
				t.Fatalf("lock file exists after cleanup, stat error = %v", err)
			}
			if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
				t.Fatalf("claim file exists after cleanup, stat error = %v", err)
			}
		})
	}
}

func TestCleanupStaleLocksRemovesDeadPIDBeforeTTLExpires(t *testing.T) {
	lockRoot := t.TempDir()
	lockPath := filepath.Join(lockRoot, "task.lock")
	claimPath := lockPath + ".claim.json"

	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() lock error = %v", err)
	}
	claim := `{"pid":` + strconv.Itoa(deadPID()) + `,"claimed_at_epoch":190}`
	if err := os.WriteFile(claimPath, []byte(claim), 0o644); err != nil {
		t.Fatalf("WriteFile() claim error = %v", err)
	}

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return time.Unix(200, 0) }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file exists after cleanup, stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Fatalf("claim file exists after cleanup, stat error = %v", err)
	}
}

func TestCleanupStaleLocksKeepsActivelyFlockedRegularFileWithMissingClaim(t *testing.T) {
	lockRoot := t.TempDir()
	lockPath := filepath.Join(lockRoot, "task.lock")
	claimPath := lockPath + ".claim.json"
	file := holdExclusiveFlock(t, lockPath)
	defer file.Close()

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return time.Unix(200, 0) }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Fatalf("claim file exists after cleanup, stat error = %v", err)
	}
}

func TestCleanupStaleLocksKeepsActivelyFlockedRegularFileWithMalformedClaim(t *testing.T) {
	lockRoot := t.TempDir()
	lockPath := filepath.Join(lockRoot, "task.lock")
	claimPath := lockPath + ".claim.json"
	file := holdExclusiveFlock(t, lockPath)
	defer file.Close()

	if err := os.WriteFile(claimPath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile() claim error = %v", err)
	}

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return time.Unix(200, 0) }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); err != nil {
		t.Fatalf("claim file stat error = %v", err)
	}
}

func TestCleanupStaleLocksKeepsActivelyFlockedRegularFileWithExpiredClaim(t *testing.T) {
	lockRoot := t.TempDir()
	lockPath := filepath.Join(lockRoot, "task.lock")
	claimPath := lockPath + ".claim.json"
	file := holdExclusiveFlock(t, lockPath)
	defer file.Close()

	claim := `{"pid":1,"claimed_at_epoch":100}`
	if err := os.WriteFile(claimPath, []byte(claim), 0o644); err != nil {
		t.Fatalf("WriteFile() claim error = %v", err)
	}

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return time.Unix(200, 0) }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); err != nil {
		t.Fatalf("claim file stat error = %v", err)
	}
}

func TestCleanupStaleLocksKeepsActivelyFlockedRegularFileWithDeadPIDClaim(t *testing.T) {
	lockRoot := t.TempDir()
	lockPath := filepath.Join(lockRoot, "task.lock")
	claimPath := lockPath + ".claim.json"
	file := holdExclusiveFlock(t, lockPath)
	defer file.Close()

	claim := `{"pid":` + strconv.Itoa(deadPID()) + `,"claimed_at_epoch":190}`
	if err := os.WriteFile(claimPath, []byte(claim), 0o644); err != nil {
		t.Fatalf("WriteFile() claim error = %v", err)
	}

	if err := CleanupStaleLocks(lockRoot, time.Minute, func() time.Time { return time.Unix(200, 0) }); err != nil {
		t.Fatalf("CleanupStaleLocks() error = %v", err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); err != nil {
		t.Fatalf("claim file stat error = %v", err)
	}
}

func holdExclusiveFlock(t *testing.T, path string) *os.File {
	t.Helper()

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		t.Fatalf("Flock() error = %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	})
	return file
}

func deadPID() int {
	for pid := os.Getpid() + 100000; pid < os.Getpid()+101000; pid++ {
		if !processLive(pid) {
			return pid
		}
	}
	return os.Getpid() + 1000000
}
