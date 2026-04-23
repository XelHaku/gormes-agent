package docker

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTaskID       = "default"
	defaultImage        = "nikolaik/python-nodejs:python3.11-nodejs20"
	defaultWorkdir      = "/root"
	workspaceDir        = "/workspace"
	defaultCPU          = 1
	defaultMemoryMB     = 5120
	defaultDiskMB       = 51200
	currentTaskIDLabel  = "gormes_task_id"
	legacyTaskIDLabel   = "hermes_task_id"
	containerSleepHours = "2h"
)

var errNilRunner = errors.New("docker: nil runner")

type CommandRequest struct {
	Args    []string
	Timeout time.Duration
}

type CommandResult struct {
	Output   string
	ExitCode int
}

type Runner interface {
	Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

type Config struct {
	Image                string
	TaskID               string
	CWD                  string
	Timeout              time.Duration
	CPU                  int
	MemoryMB             int
	DiskMB               int
	PersistentFilesystem bool
	PersistenceRoot      string
	ForwardEnv           map[string]string
	Volumes              []string
	MountCWDToWorkspace  bool
	HostCWD              string
}

type ExecRequest struct {
	Command string
	Login   bool
	Env     map[string]string
	Timeout time.Duration
}

type Backend struct {
	runner  Runner
	config  Config
	workdir string
	started bool
	name    string
}

func (c Config) Normalized() Config {
	if strings.TrimSpace(c.Image) == "" {
		c.Image = defaultImage
	}
	if strings.TrimSpace(c.TaskID) == "" {
		c.TaskID = defaultTaskID
	}
	if c.CPU <= 0 {
		c.CPU = defaultCPU
	}
	if c.MemoryMB <= 0 {
		c.MemoryMB = defaultMemoryMB
	}
	if c.DiskMB <= 0 {
		c.DiskMB = defaultDiskMB
	}
	if strings.TrimSpace(c.CWD) == "" {
		if c.MountCWDToWorkspace {
			c.CWD = workspaceDir
		} else {
			c.CWD = defaultWorkdir
		}
	}
	return c
}

func (c Config) ContainerName() string {
	return "gormes-" + c.Normalized().TaskID
}

func (c Config) RunArgs() ([]string, error) {
	n := c.Normalized()
	name := n.ContainerName()
	args := []string{
		"run", "-d",
		"--name", name,
		"--hostname", name,
		"--workdir", n.CWD,
		"--label", currentTaskIDLabel + "=" + n.TaskID,
		"--label", legacyTaskIDLabel + "=" + n.TaskID,
		"--cap-drop", "ALL",
		"--cap-add", "DAC_OVERRIDE",
		"--cap-add", "CHOWN",
		"--cap-add", "FOWNER",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "256",
		"--tmpfs", "/tmp:rw,nosuid,size=512m",
		"--tmpfs", "/var/tmp:rw,noexec,nosuid,size=256m",
		"--tmpfs", "/run:rw,noexec,nosuid,size=64m",
		"--cpus", itoa(n.CPU),
		"--memory", itoa(n.MemoryMB) + "m",
		"--storage-opt", "size=" + itoa(n.DiskMB) + "m",
	}
	args = append(args, envArgs(n.ForwardEnv)...)

	if n.PersistentFilesystem {
		if strings.TrimSpace(n.PersistenceRoot) == "" {
			return nil, errors.New("docker: persistence root required when persistence is enabled")
		}
		root := filepath.Join(n.PersistenceRoot, n.TaskID)
		if !n.MountCWDToWorkspace {
			args = append(args, "-v", filepath.Join(root, "workspace")+":"+workspaceDir)
		}
		args = append(args, "-v", filepath.Join(root, "root")+":"+defaultWorkdir)
	} else if !n.MountCWDToWorkspace {
		args = append(args, "--tmpfs", workspaceDir+":rw,exec,nosuid,size=1024m")
	}

	if n.MountCWDToWorkspace {
		if strings.TrimSpace(n.HostCWD) == "" {
			return nil, errors.New("docker: host cwd required when mounting cwd")
		}
		args = append(args, "-v", n.HostCWD+":"+workspaceDir)
	}

	for _, volume := range n.Volumes {
		if strings.TrimSpace(volume) == "" {
			continue
		}
		args = append(args, "-v", volume)
	}

	args = append(args, n.Image, "sleep", containerSleepHours)
	return args, nil
}

func New(runner Runner, cfg Config) *Backend {
	n := cfg.Normalized()
	return &Backend{
		runner:  runner,
		config:  n,
		workdir: n.CWD,
		name:    n.ContainerName(),
	}
}

func (b *Backend) WorkingDir() string {
	return b.workdir
}

func (b *Backend) Container(ctx context.Context) (string, error) {
	if b.started {
		return b.name, nil
	}
	if b.runner == nil {
		return "", errNilRunner
	}
	args, err := b.config.RunArgs()
	if err != nil {
		return "", err
	}
	if _, err := b.runner.Run(ctx, CommandRequest{
		Args:    args,
		Timeout: b.config.Timeout,
	}); err != nil {
		return "", err
	}
	b.started = true
	return b.name, nil
}

func (b *Backend) Execute(ctx context.Context, req ExecRequest) (CommandResult, error) {
	if _, err := b.Container(ctx); err != nil {
		return CommandResult{}, err
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = b.config.Timeout
	}
	args := []string{"exec", "-w", b.workdir}
	args = append(args, envArgs(req.Env)...)
	args = append(args, b.name)
	args = append(args, shellArgs(req.Command, req.Login)...)
	return b.runner.Run(ctx, CommandRequest{
		Args:    args,
		Timeout: timeout,
	})
}

func (b *Backend) Cleanup(ctx context.Context) error {
	if !b.started {
		return nil
	}
	if _, err := b.runner.Run(ctx, CommandRequest{
		Args: []string{"stop", b.name},
	}); err != nil {
		return err
	}
	if _, err := b.runner.Run(ctx, CommandRequest{
		Args: []string{"rm", "-f", b.name},
	}); err != nil {
		return err
	}
	b.started = false
	return nil
}

func envArgs(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	args := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		args = append(args, "-e", key+"="+env[key])
	}
	return args
}

func shellArgs(command string, login bool) []string {
	if login {
		return []string{"bash", "-l", "-c", command}
	}
	return []string{"bash", "-c", command}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
