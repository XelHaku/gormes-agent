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

	for _, lockPath := range matches {
		info, err := os.Stat(lockPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}

		claim, keep := readValidLockClaim(lockPath, ttl, now())
		if keep && processLive(claim.PID) {
			continue
		}
		var release func()
		if !info.IsDir() {
			var locked bool
			locked, release, err = tryExclusiveFileLock(lockPath)
			if err != nil {
				return err
			}
			if !locked {
				continue
			}
		}

		if err := removeStaleLock(lockPath, release); err != nil {
			return err
		}
	}

	return nil
}

func removeStaleLock(lockPath string, release func()) error {
	if release != nil {
		defer release()
	}
	if err := os.RemoveAll(lockPath); err != nil {
		return err
	}
	if err := os.Remove(lockPath + ".claim.json"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func tryExclusiveFileLock(path string) (bool, func(), error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, func() {}, nil
		}
		return false, nil, err
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func readValidLockClaim(lockPath string, ttl time.Duration, current time.Time) (lockClaim, bool) {
	raw, err := os.ReadFile(lockPath + ".claim.json")
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
