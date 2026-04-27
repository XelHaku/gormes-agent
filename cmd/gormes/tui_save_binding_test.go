package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tui"
)

func TestTUISaveBinding_LocalModelReceivesSessionExport(t *testing.T) {
	setupNativeTUITestEnv(t)
	seedTUISaveTranscriptDB(t, "sess-binding", "discord:binding")

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}

	cmd := newRootCommand()
	if err := cmd.Flags().Set("offline", "true"); err != nil {
		t.Fatalf("set offline flag: %v", err)
	}

	var captured tea.Model
	err = runResolvedTUIWithRuntime(cmd, tuiInvocation{Config: cfg}, rootRuntime{
		tuiProgramFactory: func(model tea.Model, _ ...tea.ProgramOption) tuiProgram {
			captured = model
			return fakeTUIProgram{}
		},
	})
	if err != nil {
		t.Fatalf("runResolvedTUIWithRuntime: %v", err)
	}

	exportFn := capturedTUISessionExport(t, captured)
	if exportFn == nil {
		t.Fatal("local TUI SessionExport = nil, want XDG-backed export helper")
	}

	path, err := exportFn(context.Background(), "sess-binding")
	if err != nil {
		t.Fatalf("SessionExportFunc: %v", err)
	}
	wantDir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "gormes", "sessions", "exports")
	if filepath.Dir(path) != wantDir {
		t.Fatalf("export dir = %q, want %q", filepath.Dir(path), wantDir)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(export path): %v", err)
	}
}

func TestTUISaveBinding_RemoteTUIUnchanged(t *testing.T) {
	setupNativeTUITestEnv(t)

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		frame := kernel.RenderFrame{Phase: kernel.PhaseIdle, Seq: 1, Model: "remote-binding"}
		data, _ := json.Marshal(frame)
		fmt.Fprintf(w, "event: frame\ndata: %s\n\n", data)
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer gateway.Close()

	var captured tea.Model
	err = runResolvedTUIWithRuntime(newRootCommand(), tuiInvocation{
		Config:    cfg,
		RemoteURL: gateway.URL,
	}, rootRuntime{
		tuiProgramFactory: func(model tea.Model, _ ...tea.ProgramOption) tuiProgram {
			captured = model
			return fakeTUIProgram{}
		},
	})
	if err != nil {
		t.Fatalf("runResolvedTUIWithRuntime(remote): %v", err)
	}

	if exportFn := capturedTUISessionExport(t, captured); exportFn != nil {
		t.Fatal("remote TUI SessionExport is non-nil; remote startup must not receive local /save binding")
	}
}

func capturedTUISessionExport(t *testing.T, model tea.Model) tui.SessionExportFunc {
	t.Helper()

	m, ok := model.(tui.Model)
	if !ok {
		t.Fatalf("captured model type = %T, want tui.Model", model)
	}

	field := reflect.ValueOf(&m).Elem().FieldByName("sessionExport")
	if !field.IsValid() {
		t.Fatal("tui.Model missing sessionExport field")
	}
	if field.IsNil() {
		return nil
	}

	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(tui.SessionExportFunc)
}
