package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultPtyReadTimeout   = 200 * time.Millisecond
	DefaultPtyReadChunkSize = 64 * 1024
	MaxPtyWriteBytes        = 64 * 1024
	MaxPtyCols              = 1000
	MaxPtyRows              = 1000
)

var (
	ErrPtyUnavailable    = errors.New("cli: pty unavailable")
	ErrInvalidPtyMessage = errors.New("cli: invalid pty message")
)

type PtyUnavailableError struct {
	GOOS   string
	Reason string
}

func (e *PtyUnavailableError) Error() string {
	if e == nil {
		return ErrPtyUnavailable.Error()
	}
	if e.Reason == "" {
		return fmt.Sprintf("%s: %s", ErrPtyUnavailable, e.GOOS)
	}
	return fmt.Sprintf("%s: %s", ErrPtyUnavailable, e.Reason)
}

func (e *PtyUnavailableError) Is(target error) bool {
	return target == ErrPtyUnavailable
}

type PtyInvalidMessageError struct {
	Reason string
}

func (e *PtyInvalidMessageError) Error() string {
	if e == nil || e.Reason == "" {
		return ErrInvalidPtyMessage.Error()
	}
	return fmt.Sprintf("%s: %s", ErrInvalidPtyMessage, e.Reason)
}

func (e *PtyInvalidMessageError) Is(target error) bool {
	return target == ErrInvalidPtyMessage
}

type PtySize struct {
	Cols int
	Rows int
}

type PtySpawnRequest struct {
	Argv []string
	CWD  string
	Env  map[string]string
	Cols int
	Rows int
}

type PtySession interface {
	Read(timeout time.Duration, maxBytes int) ([]byte, error)
	Write(data []byte) error
	Resize(cols, rows int) error
	Close() error
	IsAlive() bool
	PID() int
}

type PtySpawnFunc func(context.Context, PtySpawnRequest) (PtySession, error)

type PtyAdapterConfig struct {
	RuntimeGOOS string
	Spawn       PtySpawnFunc
}

type PtyAdapter struct {
	session PtySession
}

func NewPtyAdapter(ctx context.Context, req PtySpawnRequest, cfg PtyAdapterConfig) (*PtyAdapter, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	goos := cfg.RuntimeGOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if !ptyPlatformAvailable(goos) {
		return nil, &PtyUnavailableError{
			GOOS:   goos,
			Reason: ptyUnavailableReason(goos),
		}
	}

	normalized, err := normalizePtySpawnRequest(req)
	if err != nil {
		return nil, err
	}

	spawn := cfg.Spawn
	if spawn == nil {
		spawn = spawnPtySession
	}

	session, err := spawn(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, io.ErrClosedPipe
	}

	return NewPtyAdapterForSession(session), nil
}

func NewPtyAdapterForSession(session PtySession) *PtyAdapter {
	return &PtyAdapter{session: session}
}

func PtyAvailable() bool {
	return ptyPlatformAvailable(runtime.GOOS)
}

func (a *PtyAdapter) Read(timeout time.Duration, maxBytes int) ([]byte, error) {
	session, err := a.requireSession()
	if err != nil {
		return nil, err
	}
	if timeout < 0 {
		return nil, invalidPtyMessage("read timeout must be non-negative")
	}
	if maxBytes <= 0 {
		return nil, invalidPtyMessage("read chunk size must be positive")
	}
	if maxBytes > DefaultPtyReadChunkSize {
		maxBytes = DefaultPtyReadChunkSize
	}

	return session.Read(timeout, maxBytes)
}

func (a *PtyAdapter) Write(data []byte) error {
	session, err := a.requireSession()
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return invalidPtyMessage("write payload must be non-empty")
	}
	if len(data) > MaxPtyWriteBytes {
		return invalidPtyMessage("write payload exceeds %d bytes", MaxPtyWriteBytes)
	}

	return session.Write(append([]byte(nil), data...))
}

func (a *PtyAdapter) Resize(cols, rows int) error {
	session, err := a.requireSession()
	if err != nil {
		return err
	}
	if err := validatePtySize(cols, rows); err != nil {
		return err
	}

	return session.Resize(cols, rows)
}

func (a *PtyAdapter) HandleClientMessage(raw []byte) error {
	cols, rows, resize, err := parsePtyResizeMessage(raw)
	if err != nil {
		return err
	}
	if resize {
		return a.Resize(cols, rows)
	}

	return a.Write(raw)
}

func (a *PtyAdapter) Close() error {
	session, err := a.requireSession()
	if err != nil {
		return err
	}
	return session.Close()
}

func (a *PtyAdapter) IsAlive() bool {
	session, err := a.requireSession()
	if err != nil {
		return false
	}
	return session.IsAlive()
}

func (a *PtyAdapter) PID() int {
	session, err := a.requireSession()
	if err != nil {
		return 0
	}
	return session.PID()
}

func (a *PtyAdapter) requireSession() (PtySession, error) {
	if a == nil || a.session == nil {
		return nil, io.EOF
	}
	return a.session, nil
}

func normalizePtySpawnRequest(req PtySpawnRequest) (PtySpawnRequest, error) {
	if len(req.Argv) == 0 || strings.TrimSpace(req.Argv[0]) == "" {
		return PtySpawnRequest{}, invalidPtyMessage("argv[0] is required")
	}
	if req.Cols == 0 {
		req.Cols = 80
	}
	if req.Rows == 0 {
		req.Rows = 24
	}
	if err := validatePtySize(req.Cols, req.Rows); err != nil {
		return PtySpawnRequest{}, err
	}

	req.Argv = append([]string(nil), req.Argv...)
	if req.Env != nil {
		env := make(map[string]string, len(req.Env))
		for k, v := range req.Env {
			env[k] = v
		}
		req.Env = env
	}

	return req, nil
}

func parsePtyResizeMessage(raw []byte) (cols, rows int, resize bool, err error) {
	const prefix = "\x1b[RESIZE:"
	if !bytes.HasPrefix(raw, []byte(prefix)) {
		return 0, 0, false, nil
	}
	if !bytes.HasSuffix(raw, []byte("]")) {
		return 0, 0, true, invalidPtyMessage("resize message must terminate with ]")
	}

	body := string(raw[len(prefix) : len(raw)-1])
	parts := strings.Split(body, ";")
	if len(parts) != 2 {
		return 0, 0, true, invalidPtyMessage("resize message must contain cols and rows")
	}

	cols, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, true, invalidPtyMessage("resize cols must be an integer")
	}
	rows, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, true, invalidPtyMessage("resize rows must be an integer")
	}
	if err := validatePtySize(cols, rows); err != nil {
		return 0, 0, true, err
	}

	return cols, rows, true, nil
}

func validatePtySize(cols, rows int) error {
	if cols < 1 || rows < 1 {
		return invalidPtyMessage("terminal size must be positive")
	}
	if cols > MaxPtyCols || rows > MaxPtyRows {
		return invalidPtyMessage("terminal size exceeds %dx%d", MaxPtyCols, MaxPtyRows)
	}
	return nil
}

func invalidPtyMessage(format string, args ...any) error {
	return &PtyInvalidMessageError{Reason: fmt.Sprintf(format, args...)}
}

func ptyPlatformAvailable(goos string) bool {
	return goos == "linux"
}

func ptyUnavailableReason(goos string) string {
	if goos == "windows" {
		return "pseudo-terminals are unavailable on native Windows; use WSL"
	}
	return fmt.Sprintf("pseudo-terminals are unavailable on %s in this adapter", goos)
}
