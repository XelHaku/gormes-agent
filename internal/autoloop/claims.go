package autoloop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type lockClaim struct {
	PID            int   `json:"pid"`
	ClaimedAtEpoch int64 `json:"claimed_at_epoch"`
}

func CleanupStaleLocks(lockRoot string, ttl time.Duration, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}

	matches, err := filepath.Glob(filepath.Join(lockRoot, "*.lock"))
	if err != nil {
		return err
	}

	for _, lockDir := range matches {
		info, err := os.Stat(lockDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if !info.IsDir() {
			continue
		}

		claim, keep := readValidLockClaim(lockDir, ttl, now())
		if keep && processLive(claim.PID) {
			continue
		}

		if err := os.RemoveAll(lockDir); err != nil {
			return err
		}
		if err := os.Remove(lockDir + ".claim.json"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	return nil
}

func readValidLockClaim(lockDir string, ttl time.Duration, current time.Time) (lockClaim, bool) {
	raw, err := os.ReadFile(lockDir + ".claim.json")
	if err != nil {
		return lockClaim{}, false
	}

	var claim lockClaim
	if err := json.Unmarshal(raw, &claim); err != nil {
		return lockClaim{}, false
	}
	if claim.PID <= 0 {
		return lockClaim{}, false
	}
	if current.Sub(time.Unix(claim.ClaimedAtEpoch, 0)) > ttl {
		return lockClaim{}, false
	}

	return claim, true
}

func processLive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
