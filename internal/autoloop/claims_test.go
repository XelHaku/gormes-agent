package autoloop

import (
	"os"
	"path/filepath"
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
