package kernel

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
)

// stableProxy listens on a fixed local port and forwards traffic to whatever
// backend URL is currently registered. Used by future Route-B tests so the
// kernel can keep pointing at one stable URL across a simulated server restart.
//
// Not used by the Task-3 t.Skip'd test — shipping here so the helpers file
// is complete for future reuse when Route-B ships.
type stableProxy struct {
	listener net.Listener
	backend  atomic.Value // string
	srv      *http.Server
}

func newStableProxy(t *testing.T) *stableProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("stableProxy listen: %v", err)
	}
	p := &stableProxy{listener: ln}
	p.backend.Store("")
	p.srv = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			backend, _ := p.backend.Load().(string)
			if backend == "" {
				http.Error(w, "no backend registered", http.StatusServiceUnavailable)
				return
			}
			outReq := r.Clone(r.Context())
			outReq.URL.Scheme = "http"
			outReq.URL.Host = backendHost(backend)
			outReq.RequestURI = ""
			resp, err := http.DefaultTransport.RoundTrip(outReq)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			for k, v := range resp.Header {
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
			w.WriteHeader(resp.StatusCode)
			br := bufio.NewReader(resp.Body)
			flusher, _ := w.(http.Flusher)
			buf := make([]byte, 1024)
			for {
				n, err := br.Read(buf)
				if n > 0 {
					_, _ = w.Write(buf[:n])
					if flusher != nil {
						flusher.Flush()
					}
				}
				if err != nil {
					return
				}
			}
		}),
	}
	go func() { _ = p.srv.Serve(ln) }()
	return p
}

func (p *stableProxy) URL() string {
	return fmt.Sprintf("http://%s", p.listener.Addr().String())
}

func (p *stableProxy) Rebind(backendURL string) {
	p.backend.Store(backendURL)
}

func (p *stableProxy) Close() { _ = p.srv.Close() }

func backendHost(u string) string {
	const prefix = "http://"
	if len(u) > len(prefix) && u[:len(prefix)] == prefix {
		return u[len(prefix):]
	}
	return u
}

// fiveTokenHandler returns an http.HandlerFunc that flushes 5 SSE "content"
// frames and then hangs open (no [DONE]) so the client sees a chaos-monkey
// disconnect, not a normal end-of-stream.
func fiveTokenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Hermes-Session-Id", "sess-reconnect")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 5; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(10 * time.Millisecond)
		}
		// Hang. The test triggers a chaos-monkey disconnect before we return.
		<-r.Context().Done()
	}
}

// tenTokenHandler emits 10 "y" tokens + finish_reason=stop + [DONE]. Used
// for the post-reconnect retry server.
func tenTokenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Hermes-Session-Id", "sess-reconnect")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 10; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"y\"}}]}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":10}}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// newRealKernel constructs a kernel wired to a real hermes.NewHTTPClient
// pointed at endpoint. Used by Route-B tests that need the genuine HTTP
// path, not MockClient.
func newRealKernel(t *testing.T, endpoint string) *Kernel {
	t.Helper()
	client := hermes.NewHTTPClient(endpoint, "")
	return New(Config{
		Model:     "hermes-agent",
		Endpoint:  endpoint,
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)
}
