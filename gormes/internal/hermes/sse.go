package hermes

import (
	"bufio"
	"context"
	"io"
	"strings"
)

// sseFrame is one SSE event (`event:` line + accumulated `data:` lines).
type sseFrame struct {
	event string
	data  string
}

// sseReader is a pull-based SSE parser with a bounded internal buffer
// (1 MB per line — generous for any sane payload; prevents unbounded growth).
type sseReader struct {
	sc *bufio.Scanner
	r  io.Reader
}

func newSSEReader(r io.Reader) *sseReader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	return &sseReader{sc: sc, r: r}
}

// Next returns the next SSE frame. Context is checked before each read attempt.
// Returns ctx.Err() if cancelled before reading starts, io.EOF at stream end.
// bufio.Scanner lacks mid-read cancellation; callers should set reader timeouts.
func (r *sseReader) Next(ctx context.Context) (*sseFrame, error) {
	var f sseFrame
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if !r.sc.Scan() {
			if err := r.sc.Err(); err != nil {
				return nil, err
			}
			if f.data != "" || f.event != "" {
				return &f, nil
			}
			return nil, io.EOF
		}

		line := r.sc.Text()
		if line == "" {
			if f.data != "" || f.event != "" {
				return &f, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") { // SSE comment / keepalive
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			f.event = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			if f.data != "" {
				f.data += "\n"
			}
			f.data += strings.TrimPrefix(line, "data: ")
			continue
		}
	}
}
