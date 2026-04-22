package main

import (
	"log/slog"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
)

const sessionIndexMirrorInterval = 30 * time.Second

func startSessionIndexMirror(smap *session.BoltMap, log *slog.Logger) *session.SessionIndexMirrorRefresher {
	if smap == nil {
		return nil
	}
	mirror := session.NewSessionIndexMirror(smap, config.SessionIndexMirrorPath())
	return mirror.StartRefresh(sessionIndexMirrorInterval, log)
}
