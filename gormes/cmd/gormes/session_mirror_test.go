package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
)

func TestStartSessionIndexMirror_WritesToXDGPath(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dataHome, "config"))

	smap, err := session.OpenBolt(config.SessionDBPath())
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer smap.Close()

	if err := smap.Put(context.Background(), "telegram:42", "sess-telegram"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	refresh := startSessionIndexMirror(smap, nil)
	if refresh == nil {
		t.Fatal("startSessionIndexMirror() = nil, want running refresher")
	}
	defer refresh.Stop()

	mirrorPath := config.SessionIndexMirrorPath()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(mirrorPath)
		if err == nil && strings.Contains(string(raw), "telegram:42: sess-telegram") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	raw, _ := os.ReadFile(mirrorPath)
	t.Fatalf("session mirror %q never received expected content; last content:\n%s", mirrorPath, raw)
}
