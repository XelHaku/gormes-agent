package singularity

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestConfigResolvedScratchDirPrecedence(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "explicit scratch dir wins",
			cfg: Config{
				ScratchDir: "/tmp/custom-scratch",
				SandboxDir: "/tmp/sandboxes",
				Username:   "alice",
				HomeDir:    "/home/alice",
			},
			want: "/tmp/custom-scratch",
		},
		{
			name: "sandbox dir falls back to singularity child",
			cfg: Config{
				SandboxDir: "/tmp/sandboxes",
				Username:   "alice",
				HomeDir:    "/home/alice",
			},
			want: "/tmp/sandboxes/singularity",
		},
		{
			name: "hpc scratch uses username",
			cfg: Config{
				Username: "alice",
				HomeDir:  "/home/alice",
			},
			want: "/scratch/alice/hermes-agent",
		},
		{
			name: "home dir is the final fallback",
			cfg: Config{
				HomeDir: "/home/alice",
			},
			want: "/home/alice/.hermes/sandboxes/singularity",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "home dir is the final fallback" {
				t.Setenv("USER", "")
			}
			if got := tt.cfg.Normalized().ResolvedScratchDir(); got != tt.want {
				t.Fatalf("ResolvedScratchDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBackendExecutePersistentOverlayCreatesOverlayOnceAndReusesExecState(t *testing.T) {
	scratchDir := t.TempDir()
	runner := &fakeRunner{
		results: []CommandResult{
			{},
			{Output: "hi\n", ExitCode: 0},
			{Output: "/root\n", ExitCode: 0},
		},
	}
	backend := New(runner, Config{
		Image:                "/images/python.sif",
		TaskID:               "task-123",
		Timeout:              90 * time.Second,
		DiskMB:               2048,
		PersistentFilesystem: true,
		ScratchDir:           scratchDir,
	})

	first, err := backend.Execute(context.Background(), ExecRequest{
		Command: "echo hi",
		Login:   true,
		Env: map[string]string{
			"NAME":    "gormes",
			"API_KEY": "secret",
		},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Output != "hi\n" || first.ExitCode != 0 {
		t.Fatalf("Execute(first) = %+v, want output %q exit %d", first, "hi\n", 0)
	}

	second, err := backend.Execute(context.Background(), ExecRequest{
		Command: "pwd",
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Output != "/root\n" || second.ExitCode != 0 {
		t.Fatalf("Execute(second) = %+v, want output %q exit %d", second, "/root\n", 0)
	}

	if got := backend.WorkingDir(); got != "/root" {
		t.Fatalf("WorkingDir() = %q, want %q", got, "/root")
	}

	overlayPath := filepath.Join(scratchDir, "overlays", "task-123.img")
	sessionDir := filepath.Join(scratchDir, "task-123")
	if _, err := os.Stat(filepath.Dir(overlayPath)); err != nil {
		t.Fatalf("overlay parent missing: %v", err)
	}
	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("session dir missing: %v", err)
	}

	wantCalls := []runCall{
		{
			Args: []string{
				"apptainer",
				"overlay", "create",
				"--size", "2048",
				overlayPath,
			},
			Timeout: 90 * time.Second,
		},
		{
			Args: []string{
				"apptainer",
				"exec",
				"--containall",
				"--no-home",
				"--workdir", sessionDir,
				"--overlay", overlayPath,
				"--pwd", "/root",
				"--env", "API_KEY=secret,NAME=gormes",
				"/images/python.sif",
				"bash", "-l", "-c", "echo hi",
			},
			Timeout: 5 * time.Second,
		},
		{
			Args: []string{
				"apptainer",
				"exec",
				"--containall",
				"--no-home",
				"--workdir", sessionDir,
				"--overlay", overlayPath,
				"--pwd", "/root",
				"/images/python.sif",
				"bash", "-c", "pwd",
			},
			Timeout: 90 * time.Second,
		},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
}

func TestBackendExecuteEphemeralUsesWritableTmpfsAndCleanupRemovesScratch(t *testing.T) {
	scratchDir := t.TempDir()
	runner := &fakeRunner{
		results: []CommandResult{
			{Output: "/workspace\n", ExitCode: 0},
		},
	}
	backend := New(runner, Config{
		Runtime:              "singularity",
		Image:                "docker://python:3.11-slim",
		CWD:                  "/workspace",
		Timeout:              2 * time.Minute,
		PersistentFilesystem: false,
		ScratchDir:           scratchDir,
	})

	got, err := backend.Execute(context.Background(), ExecRequest{
		Command: "pwd",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != "/workspace\n" || got.ExitCode != 0 {
		t.Fatalf("Execute() = %+v, want output %q exit %d", got, "/workspace\n", 0)
	}

	sessionDir := filepath.Join(scratchDir, "default")
	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("session dir missing before cleanup: %v", err)
	}

	if err := backend.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if err := backend.Cleanup(context.Background()); err != nil {
		t.Fatalf("second Cleanup() error = %v", err)
	}
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatalf("session dir still exists after cleanup, err = %v", err)
	}

	wantCalls := []runCall{
		{
			Args: []string{
				"singularity",
				"exec",
				"--containall",
				"--no-home",
				"--workdir", sessionDir,
				"--writable-tmpfs",
				"--pwd", "/workspace",
				"docker://python:3.11-slim",
				"bash", "-c", "pwd",
			},
			Timeout: 2 * time.Minute,
		},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
}

type fakeRunner struct {
	results []CommandResult
	calls   []runCall
}

type runCall struct {
	Args    []string
	Timeout time.Duration
}

func (r *fakeRunner) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	r.calls = append(r.calls, runCall{
		Args:    append([]string(nil), req.Args...),
		Timeout: req.Timeout,
	})
	if len(r.results) == 0 {
		return CommandResult{}, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}
