package docker

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestConfigRunArgsNormalizesSecurityResourcesAndPersistence(t *testing.T) {
	cfg := Config{
		Image:                "nikolaik/python-nodejs:python3.11-nodejs20",
		TaskID:               "task-123",
		CPU:                  2,
		MemoryMB:             5120,
		DiskMB:               10240,
		PersistentFilesystem: true,
		PersistenceRoot:      "/tmp/.gormes/sandboxes/docker",
		ForwardEnv: map[string]string{
			"GITHUB_TOKEN": "secret",
		},
		Volumes: []string{"/host/data:/data:ro"},
	}

	normalized := cfg.Normalized()
	if normalized.CWD != "/root" {
		t.Fatalf("Normalized().CWD = %q, want %q", normalized.CWD, "/root")
	}

	args, err := normalized.RunArgs()
	if err != nil {
		t.Fatalf("RunArgs() error = %v", err)
	}

	want := []string{
		"run", "-d",
		"--name", "gormes-task-123",
		"--hostname", "gormes-task-123",
		"--workdir", "/root",
		"--label", "gormes_task_id=task-123",
		"--label", "hermes_task_id=task-123",
		"--cap-drop", "ALL",
		"--cap-add", "DAC_OVERRIDE",
		"--cap-add", "CHOWN",
		"--cap-add", "FOWNER",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "256",
		"--tmpfs", "/tmp:rw,nosuid,size=512m",
		"--tmpfs", "/var/tmp:rw,noexec,nosuid,size=256m",
		"--tmpfs", "/run:rw,noexec,nosuid,size=64m",
		"--cpus", "2",
		"--memory", "5120m",
		"--storage-opt", "size=10240m",
		"-e", "GITHUB_TOKEN=secret",
		"-v", "/tmp/.gormes/sandboxes/docker/task-123/workspace:/workspace",
		"-v", "/tmp/.gormes/sandboxes/docker/task-123/root:/root",
		"-v", "/host/data:/data:ro",
		"nikolaik/python-nodejs:python3.11-nodejs20",
		"sleep", "2h",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("RunArgs() = %#v, want %#v", args, want)
	}
}

func TestBackendExecuteStartsContainerOnceAndUsesLoginShell(t *testing.T) {
	runner := &fakeRunner{
		results: []CommandResult{
			{Output: "container-123", ExitCode: 0},
			{Output: "hi\n", ExitCode: 0},
			{Output: "/workspace\n", ExitCode: 0},
		},
	}
	backend := New(runner, Config{
		TaskID:               "task-123",
		Timeout:              90 * time.Second,
		MountCWDToWorkspace:  true,
		HostCWD:              "/Users/alice/project",
		PersistentFilesystem: false,
	})

	first, err := backend.Execute(context.Background(), ExecRequest{
		Command: "echo hi",
		Login:   true,
		Env: map[string]string{
			"NAME": "gormes",
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
	if second.Output != "/workspace\n" || second.ExitCode != 0 {
		t.Fatalf("Execute(second) = %+v, want output %q exit %d", second, "/workspace\n", 0)
	}

	if got := backend.WorkingDir(); got != "/workspace" {
		t.Fatalf("WorkingDir() = %q, want %q", got, "/workspace")
	}

	if len(runner.calls) != 3 {
		t.Fatalf("len(calls) = %d, want %d", len(runner.calls), 3)
	}

	wantRunPrefix := []string{
		"run", "-d",
		"--name", "gormes-task-123",
		"--hostname", "gormes-task-123",
		"--workdir", "/workspace",
	}
	if !reflect.DeepEqual(runner.calls[0].Args[:len(wantRunPrefix)], wantRunPrefix) {
		t.Fatalf("run prefix = %#v, want %#v", runner.calls[0].Args[:len(wantRunPrefix)], wantRunPrefix)
	}
	if runner.calls[0].Timeout != 90*time.Second {
		t.Fatalf("run timeout = %v, want %v", runner.calls[0].Timeout, 90*time.Second)
	}

	wantFirstExec := []string{
		"exec",
		"-w", "/workspace",
		"-e", "NAME=gormes",
		"gormes-task-123",
		"bash", "-l", "-c", "echo hi",
	}
	if !reflect.DeepEqual(runner.calls[1].Args, wantFirstExec) {
		t.Fatalf("first exec args = %#v, want %#v", runner.calls[1].Args, wantFirstExec)
	}
	if runner.calls[1].Timeout != 5*time.Second {
		t.Fatalf("first exec timeout = %v, want %v", runner.calls[1].Timeout, 5*time.Second)
	}

	wantSecondExec := []string{
		"exec",
		"-w", "/workspace",
		"gormes-task-123",
		"bash", "-c", "pwd",
	}
	if !reflect.DeepEqual(runner.calls[2].Args, wantSecondExec) {
		t.Fatalf("second exec args = %#v, want %#v", runner.calls[2].Args, wantSecondExec)
	}
	if runner.calls[2].Timeout != 90*time.Second {
		t.Fatalf("second exec timeout = %v, want %v", runner.calls[2].Timeout, 90*time.Second)
	}
}

func TestBackendCleanupStopsAndRemovesContainer(t *testing.T) {
	runner := &fakeRunner{
		results: []CommandResult{
			{Output: "container-123", ExitCode: 0},
			{Output: "", ExitCode: 0},
			{Output: "", ExitCode: 0},
		},
	}
	backend := New(runner, Config{
		TaskID: "task-123",
	})

	if _, err := backend.Container(context.Background()); err != nil {
		t.Fatalf("Container() error = %v", err)
	}
	if err := backend.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if err := backend.Cleanup(context.Background()); err != nil {
		t.Fatalf("second Cleanup() error = %v", err)
	}

	wantCalls := [][]string{
		{
			"run", "-d",
			"--name", "gormes-task-123",
			"--hostname", "gormes-task-123",
			"--workdir", "/root",
			"--label", "gormes_task_id=task-123",
			"--label", "hermes_task_id=task-123",
			"--cap-drop", "ALL",
			"--cap-add", "DAC_OVERRIDE",
			"--cap-add", "CHOWN",
			"--cap-add", "FOWNER",
			"--security-opt", "no-new-privileges",
			"--pids-limit", "256",
			"--tmpfs", "/tmp:rw,nosuid,size=512m",
			"--tmpfs", "/var/tmp:rw,noexec,nosuid,size=256m",
			"--tmpfs", "/run:rw,noexec,nosuid,size=64m",
			"--cpus", "1",
			"--memory", "5120m",
			"--storage-opt", "size=51200m",
			"--tmpfs", "/workspace:rw,exec,nosuid,size=1024m",
			"nikolaik/python-nodejs:python3.11-nodejs20",
			"sleep", "2h",
		},
		{"stop", "gormes-task-123"},
		{"rm", "-f", "gormes-task-123"},
	}

	if len(runner.calls) != len(wantCalls) {
		t.Fatalf("len(calls) = %d, want %d", len(runner.calls), len(wantCalls))
	}
	for i, want := range wantCalls {
		if !reflect.DeepEqual(runner.calls[i].Args, want) {
			t.Fatalf("call[%d] args = %#v, want %#v", i, runner.calls[i].Args, want)
		}
	}
}

func TestConfigRunArgsMountCWDReplacesWorkspaceTmpfs(t *testing.T) {
	cfg := Config{
		TaskID:              "task-123",
		MountCWDToWorkspace: true,
		HostCWD:             "/Users/alice/project",
	}

	args, err := cfg.Normalized().RunArgs()
	if err != nil {
		t.Fatalf("RunArgs() error = %v", err)
	}

	if slicesContain(args, "--tmpfs", "/workspace:rw,exec,nosuid,size=1024m") {
		t.Fatalf("RunArgs() unexpectedly contains tmpfs workspace mount: %#v", args)
	}
	if !slicesContain(args, "-v", "/Users/alice/project:/workspace") {
		t.Fatalf("RunArgs() missing cwd mount: %#v", args)
	}
}

type fakeRunner struct {
	results []CommandResult
	errors  []error
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

	if len(r.errors) > 0 {
		err := r.errors[0]
		r.errors = r.errors[1:]
		if err != nil {
			return CommandResult{}, err
		}
	}
	if len(r.results) == 0 {
		return CommandResult{}, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}

func slicesContain(args []string, key, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
