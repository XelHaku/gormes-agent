//go:build !linux

package cli

import (
	"context"
	"runtime"
)

func spawnPtySession(context.Context, PtySpawnRequest) (PtySession, error) {
	return nil, &PtyUnavailableError{
		GOOS:   runtime.GOOS,
		Reason: ptyUnavailableReason(runtime.GOOS),
	}
}
