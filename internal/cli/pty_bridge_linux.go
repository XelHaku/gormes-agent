//go:build linux

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type linuxPtySession struct {
	master   *os.File
	cmd      *exec.Cmd
	waitDone chan struct{}
	exited   atomic.Bool
	closed   atomic.Bool

	closeOnce sync.Once
	closeErr  error
}

func spawnPtySession(ctx context.Context, req PtySpawnRequest) (PtySession, error) {
	master, slave, err := openPtyPair(req.Cols, req.Rows)
	if err != nil {
		return nil, err
	}
	defer slave.Close()

	cmd := exec.CommandContext(ctx, req.Argv[0], req.Argv[1:]...)
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}
	if req.CWD != "" {
		cmd.Dir = req.CWD
	}
	if req.Env != nil {
		cmd.Env = ptyEnvList(req.Env)
	}

	if err := cmd.Start(); err != nil {
		_ = master.Close()
		return nil, err
	}

	session := &linuxPtySession{
		master:   master,
		cmd:      cmd,
		waitDone: make(chan struct{}),
	}
	go func() {
		_ = cmd.Wait()
		session.exited.Store(true)
		close(session.waitDone)
	}()

	return session, nil
}

func openPtyPair(cols, rows int) (*os.File, *os.File, error) {
	masterFD, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, &PtyUnavailableError{
			GOOS:   "linux",
			Reason: fmt.Sprintf("open /dev/ptmx: %v", err),
		}
	}
	masterOpen := true
	defer func() {
		if masterOpen {
			_ = unix.Close(masterFD)
		}
	}()

	if err := unix.IoctlSetPointerInt(masterFD, unix.TIOCSPTLCK, 0); err != nil {
		return nil, nil, fmt.Errorf("unlock pty: %w", err)
	}
	ptyNumber, err := unix.IoctlGetInt(masterFD, unix.TIOCGPTN)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve pty slave: %w", err)
	}

	slavePath := "/dev/pts/" + strconv.Itoa(ptyNumber)
	slaveFD, err := unix.Open(slavePath, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open pty slave %s: %w", slavePath, err)
	}
	slaveOpen := true
	defer func() {
		if slaveOpen {
			_ = unix.Close(slaveFD)
		}
	}()

	if err := setPtyWinsize(masterFD, cols, rows); err != nil {
		return nil, nil, err
	}

	master := os.NewFile(uintptr(masterFD), "pty-master")
	slave := os.NewFile(uintptr(slaveFD), "pty-slave")
	masterOpen = false
	slaveOpen = false

	return master, slave, nil
}

func (s *linuxPtySession) Read(timeout time.Duration, maxBytes int) ([]byte, error) {
	if s == nil || s.closed.Load() || s.master == nil {
		return nil, io.EOF
	}

	fd := int(s.master.Fd())
	pollFDs := []unix.PollFd{{
		Fd:     int32(fd),
		Events: unix.POLLIN | unix.POLLHUP | unix.POLLERR,
	}}

	for {
		n, err := unix.Poll(pollFDs, pollTimeoutMillis(timeout))
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			if isClosedFD(err) {
				return nil, io.EOF
			}
			return nil, err
		}
		if n == 0 {
			return []byte{}, nil
		}
		break
	}

	if pollFDs[0].Revents&unix.POLLNVAL != 0 {
		return nil, io.EOF
	}

	buf := make([]byte, maxBytes)
	n, err := unix.Read(fd, buf)
	if err != nil {
		if isClosedFD(err) {
			return nil, io.EOF
		}
		return nil, err
	}
	if n == 0 {
		return nil, io.EOF
	}

	return buf[:n], nil
}

func (s *linuxPtySession) Write(data []byte) error {
	if s == nil || s.closed.Load() || s.master == nil {
		return io.EOF
	}

	fd := int(s.master.Fd())
	for len(data) > 0 {
		n, err := unix.Write(fd, data)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			if isClosedFD(err) {
				return io.EOF
			}
			return err
		}
		if n <= 0 {
			return io.ErrClosedPipe
		}
		data = data[n:]
	}

	return nil
}

func (s *linuxPtySession) Resize(cols, rows int) error {
	if s == nil || s.closed.Load() || s.master == nil {
		return io.EOF
	}

	if err := setPtyWinsize(int(s.master.Fd()), cols, rows); err != nil {
		if isClosedFD(err) {
			return io.EOF
		}
		return err
	}

	return nil
}

func (s *linuxPtySession) Close() error {
	if s == nil {
		return io.EOF
	}
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.closeErr = s.close()
	})
	return s.closeErr
}

func (s *linuxPtySession) close() error {
	var closeErr error

	if s.cmd != nil && s.cmd.Process != nil && !s.exited.Load() {
		for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGTERM, syscall.SIGKILL} {
			if s.exited.Load() {
				break
			}
			signalPtyProcess(s.cmd.Process.Pid, sig)
			if waitForPtyExit(s.waitDone, 500*time.Millisecond) {
				break
			}
		}
	}

	if s.master != nil {
		if err := s.master.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			closeErr = err
		}
	}
	waitForPtyExit(s.waitDone, time.Second)

	return closeErr
}

func (s *linuxPtySession) IsAlive() bool {
	if s == nil || s.closed.Load() || s.exited.Load() {
		return false
	}
	select {
	case <-s.waitDone:
		return false
	default:
		return true
	}
}

func (s *linuxPtySession) PID() int {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

func setPtyWinsize(fd int, cols, rows int) error {
	return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{
		Row: uint16(rows),
		Col: uint16(cols),
	})
}

func signalPtyProcess(pid int, sig syscall.Signal) {
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, sig); err != nil && err != syscall.ESRCH {
		_ = syscall.Kill(pid, sig)
	}
}

func waitForPtyExit(done <-chan struct{}, timeout time.Duration) bool {
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func pollTimeoutMillis(timeout time.Duration) int {
	if timeout <= 0 {
		return 0
	}

	millis := int(timeout / time.Millisecond)
	if timeout%time.Millisecond != 0 {
		millis++
	}
	if millis < 1 {
		return 1
	}
	return millis
}

func isClosedFD(err error) bool {
	return errors.Is(err, unix.EIO) ||
		errors.Is(err, unix.EBADF) ||
		errors.Is(err, unix.EPIPE)
}

func ptyEnvList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}
