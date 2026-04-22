package gateway

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadHookScripts_LoadsExactAndWildcardHooks(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}

	root := t.TempDir()
	outDir := filepath.Join(root, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(out): %v", err)
	}

	t.Setenv("GO_WANT_HOOK_HELPER_PROCESS", "1")
	t.Setenv("GORMES_HOOK_TEST_OUT_DIR", outDir)

	writeHookManifest(t, filepath.Join(root, "exact"), `name: exact
events:
  - before_send
command:
  - `+exe+`
  - -test.run=TestHookHelperProcess
  - --
  - exact
`)
	writeHookManifest(t, filepath.Join(root, "prefix"), `name: prefix
events:
  - before_*
command:
  - `+exe+`
  - -test.run=TestHookHelperProcess
  - --
  - prefix
`)
	writeHookManifest(t, filepath.Join(root, "global"), `name: global
events:
  - "*"
command:
  - `+exe+`
  - -test.run=TestHookHelperProcess
  - --
  - global
`)

	hooks, loaded, err := LoadHookScripts(root, nil)
	if err != nil {
		t.Fatalf("LoadHookScripts: %v", err)
	}
	if got := len(loaded); got != 3 {
		t.Fatalf("loaded hooks = %d, want 3", got)
	}

	hooks.Fire(context.Background(), HookEvent{
		Point:    HookBeforeSend,
		Platform: "telegram",
		ChatID:   "42",
		Text:     "hello",
	})
	hooks.Fire(context.Background(), HookEvent{
		Point:    HookAfterSend,
		Platform: "telegram",
		ChatID:   "42",
		MsgID:    "m1",
		Text:     "done",
	})

	assertHookLines(t, filepath.Join(outDir, "exact.jsonl"), 1, `"point":"before_send"`)
	assertHookLines(t, filepath.Join(outDir, "prefix.jsonl"), 1, `"point":"before_send"`)
	assertHookLines(t, filepath.Join(outDir, "global.jsonl"), 2, `"point":"after_send"`)
}

func TestLoadHookScripts_SkipsInvalidHooks(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}

	root := t.TempDir()
	outDir := filepath.Join(root, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(out): %v", err)
	}

	t.Setenv("GO_WANT_HOOK_HELPER_PROCESS", "1")
	t.Setenv("GORMES_HOOK_TEST_OUT_DIR", outDir)

	writeHookManifest(t, filepath.Join(root, "valid"), `name: valid
events:
  - on_error
command:
  - `+exe+`
  - -test.run=TestHookHelperProcess
  - --
  - valid
`)
	writeHookManifest(t, filepath.Join(root, "missing-command"), `name: invalid
events:
  - before_send
`)
	writeHookManifest(t, filepath.Join(root, "bad-event"), `name: bad-event
events:
  - never_send
command:
  - `+exe+`
  - -test.run=TestHookHelperProcess
  - --
  - bad
`)

	hooks, loaded, err := LoadHookScripts(root, nil)
	if err != nil {
		t.Fatalf("LoadHookScripts: %v", err)
	}
	if got := len(loaded); got != 1 {
		t.Fatalf("loaded hooks = %d, want 1", got)
	}
	if loaded[0].Name != "valid" {
		t.Fatalf("loaded[0].Name = %q, want %q", loaded[0].Name, "valid")
	}

	hooks.Fire(context.Background(), HookEvent{
		Point:    HookOnError,
		Platform: "telegram",
		ChatID:   "42",
		Text:     "boom",
	})

	assertHookLines(t, filepath.Join(outDir, "valid.jsonl"), 1, `"point":"on_error"`)
	if _, err := os.Stat(filepath.Join(outDir, "bad.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("bad hook output should not exist, stat err = %v", err)
	}
}

func TestHookHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HOOK_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	marker := -1
	for i, arg := range args {
		if arg == "--" {
			marker = i
			break
		}
	}
	if marker == -1 || marker+1 >= len(args) {
		t.Fatalf("helper args missing label: %v", args)
	}
	label := args[marker+1]

	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatalf("ReadAll(stdin): %v", err)
	}

	outPath := filepath.Join(os.Getenv("GORMES_HOOK_TEST_OUT_DIR"), label+".jsonl")
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(%s): %v", outPath, err)
	}
	defer f.Close()

	if _, err := f.Write(append(payload, '\n')); err != nil {
		t.Fatalf("Write(%s): %v", outPath, err)
	}
}

func writeHookManifest(t *testing.T, dir, manifest string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "HOOK.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile(HOOK.yaml): %v", err)
	}
}

func assertHookLines(t *testing.T, path string, wantLines int, wantSubstring string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if got := len(lines); got != wantLines {
		t.Fatalf("%s lines = %d, want %d; data=%q", path, got, wantLines, string(data))
	}
	if !strings.Contains(string(data), wantSubstring) {
		t.Fatalf("%s missing %q; data=%q", path, wantSubstring, string(data))
	}
}
