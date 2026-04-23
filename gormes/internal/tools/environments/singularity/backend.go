package singularity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRuntime = "apptainer"
	defaultTaskID  = "default"
	defaultImage   = "docker://nikolaik/python-nodejs:python3.11-nodejs20"
	defaultWorkdir = "/root"
	defaultDiskMB  = 51200
)

var errNilRunner = errors.New("singularity: nil runner")

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
	Runtime              string
	Image                string
	TaskID               string
	CWD                  string
	Timeout              time.Duration
	DiskMB               int
	PersistentFilesystem bool
	ScratchDir           string
	SandboxDir           string
	Username             string
	HomeDir              string
}

type ExecRequest struct {
	Command string
	Login   bool
	Env     map[string]string
	Timeout time.Duration
}

type Backend struct {
	runner       Runner
	config       Config
	workdir      string
	overlayReady bool
}

func (c Config) Normalized() Config {
	if strings.TrimSpace(c.Runtime) == "" {
		c.Runtime = defaultRuntime
	}
	if strings.TrimSpace(c.Image) == "" {
		c.Image = defaultImage
	}
	if strings.TrimSpace(c.TaskID) == "" {
		c.TaskID = defaultTaskID
	}
	if strings.TrimSpace(c.CWD) == "" {
		c.CWD = defaultWorkdir
	}
	if c.DiskMB <= 0 {
		c.DiskMB = defaultDiskMB
	}
	if strings.TrimSpace(c.Username) == "" {
		c.Username = strings.TrimSpace(os.Getenv("USER"))
	}
	if strings.TrimSpace(c.HomeDir) == "" {
		if home, err := os.UserHomeDir(); err == nil {
			c.HomeDir = home
		}
	}
	return c
}

func (c Config) ResolvedScratchDir() string {
	n := c.Normalized()
	switch {
	case strings.TrimSpace(n.ScratchDir) != "":
		return filepath.Clean(n.ScratchDir)
	case strings.TrimSpace(n.SandboxDir) != "":
		return filepath.Join(filepath.Clean(n.SandboxDir), "singularity")
	case strings.TrimSpace(n.Username) != "":
		return filepath.Join("/scratch", n.Username, "hermes-agent")
	case strings.TrimSpace(n.HomeDir) != "":
		return filepath.Join(filepath.Clean(n.HomeDir), ".hermes", "sandboxes", "singularity")
	default:
		return filepath.Join(os.TempDir(), "gormes", "sandboxes", "singularity")
	}
}

func (c Config) SessionDir() string {
	n := c.Normalized()
	return filepath.Join(n.ResolvedScratchDir(), n.TaskID)
}

func (c Config) OverlayPath() string {
	n := c.Normalized()
	return filepath.Join(n.ResolvedScratchDir(), "overlays", n.TaskID+".img")
}

func New(runner Runner, cfg Config) *Backend {
	n := cfg.Normalized()
	return &Backend{
		runner:  runner,
		config:  n,
		workdir: n.CWD,
	}
}

func (b *Backend) WorkingDir() string {
	return b.workdir
}

func (b *Backend) Execute(ctx context.Context, req ExecRequest) (CommandResult, error) {
	if b.runner == nil {
		return CommandResult{}, errNilRunner
	}
	if err := os.MkdirAll(b.sessionDir(), 0o755); err != nil {
		return CommandResult{}, err
	}
	if err := b.ensureOverlay(ctx); err != nil {
		return CommandResult{}, err
	}
	return b.runner.Run(ctx, CommandRequest{
		Args:    b.execArgs(req),
		Timeout: b.effectiveTimeout(req.Timeout),
	})
}

func (b *Backend) Cleanup(context.Context) error {
	if b.config.PersistentFilesystem {
		return nil
	}
	return os.RemoveAll(b.sessionDir())
}

func (b *Backend) ensureOverlay(ctx context.Context) error {
	if !b.config.PersistentFilesystem || b.overlayReady {
		return nil
	}

	overlayPath := b.overlayPath()
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(overlayPath); err == nil {
		b.overlayReady = true
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if _, err := b.runner.Run(ctx, CommandRequest{
		Args: []string{
			b.config.Runtime,
			"overlay", "create",
			"--size", strconv.Itoa(b.config.DiskMB),
			overlayPath,
		},
		Timeout: b.config.Timeout,
	}); err != nil {
		return err
	}
	b.overlayReady = true
	return nil
}

func (b *Backend) execArgs(req ExecRequest) []string {
	args := []string{
		b.config.Runtime,
		"exec",
		"--containall",
		"--no-home",
		"--workdir", b.sessionDir(),
	}
	if b.config.PersistentFilesystem {
		args = append(args, "--overlay", b.overlayPath())
	} else {
		args = append(args, "--writable-tmpfs")
	}
	args = append(args, "--pwd", b.workdir)
	if env := envArg(req.Env); env != "" {
		args = append(args, "--env", env)
	}
	args = append(args, b.config.Image)
	args = append(args, shellArgs(req.Command, req.Login)...)
	return args
}

func (b *Backend) effectiveTimeout(requested time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	return b.config.Timeout
}

func (b *Backend) sessionDir() string {
	return b.config.SessionDir()
}

func (b *Backend) overlayPath() string {
	return b.config.OverlayPath()
}

func envArg(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+env[key])
	}
	return strings.Join(parts, ",")
}

func shellArgs(command string, login bool) []string {
	if login {
		return []string{"bash", "-l", "-c", command}
	}
	return []string{"bash", "-c", command}
}
