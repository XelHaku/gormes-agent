package gateway

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"
)

const defaultStateMirrorInterval = 30 * time.Second

type stateMirrorDocument struct {
	HomeChannels []HomeChannel           `json:"home_channels"`
	Directory    []ChannelDirectoryEntry `json:"directory"`
	UpdatedAt    time.Time               `json:"updated_at"`
}

// StateMirror writes operator-facing gateway state to one deterministic JSON
// surface so home-channel ownership and the channel directory survive beyond
// process memory.
type StateMirror struct {
	homes     *HomeChannels
	directory *ChannelDirectory
	path      string
	now       func() time.Time
}

type StateMirrorRefresher struct {
	mirror          *StateMirror
	ticker          *time.Ticker
	stop            chan struct{}
	stopOnce        sync.Once
	wg              sync.WaitGroup
	log             *slog.Logger
	lastFingerprint string
	lastMu          sync.Mutex
}

func NewStateMirror(homes *HomeChannels, directory *ChannelDirectory, path string) *StateMirror {
	return &StateMirror{
		homes:     homes,
		directory: directory,
		path:      path,
		now:       time.Now,
	}
}

func (m *StateMirror) Path() string {
	if m == nil {
		return ""
	}
	return m.path
}

func (m *StateMirror) Write() error {
	if m == nil {
		return errors.New("gateway: nil StateMirror")
	}
	if m.now == nil {
		m.now = time.Now
	}
	doc := m.snapshotDocument()
	doc.UpdatedAt = m.now().UTC()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONAtomic(m.path, data, 0o644)
}

func (m *StateMirror) StartRefresh(interval time.Duration, log *slog.Logger) *StateMirrorRefresher {
	if m == nil {
		return nil
	}
	if interval <= 0 {
		interval = defaultStateMirrorInterval
	}
	if log == nil {
		log = slog.Default()
	}

	r := &StateMirrorRefresher{
		mirror: m,
		ticker: time.NewTicker(interval),
		stop:   make(chan struct{}, 1),
		log:    log,
	}
	r.wg.Add(1)
	go r.loop()
	return r
}

func (r *StateMirrorRefresher) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		close(r.stop)
		r.ticker.Stop()
		r.wg.Wait()
	})
}

func (r *StateMirrorRefresher) loop() {
	defer r.wg.Done()
	r.sync()
	for {
		select {
		case <-r.ticker.C:
			r.sync()
		case <-r.stop:
			return
		}
	}
}

func (r *StateMirrorRefresher) sync() {
	doc := r.mirror.snapshotDocument()
	fingerprint, err := stateMirrorFingerprint(doc)
	if err != nil {
		r.log.Warn("gateway state mirror fingerprint failed", "err", err)
		return
	}

	r.lastMu.Lock()
	same := fingerprint == r.lastFingerprint
	r.lastMu.Unlock()
	if same {
		if _, err := os.Stat(r.mirror.path); err == nil {
			return
		}
	}

	if err := r.mirror.Write(); err != nil {
		r.log.Warn("gateway state mirror write failed", "path", r.mirror.path, "err", err)
		return
	}

	r.lastMu.Lock()
	r.lastFingerprint = fingerprint
	r.lastMu.Unlock()
}

func mirrorHomeChannels(homes *HomeChannels) []HomeChannel {
	snapshot := homes.Snapshot()
	if snapshot == nil {
		return []HomeChannel{}
	}
	return snapshot
}

func mirrorDirectory(directory *ChannelDirectory) []ChannelDirectoryEntry {
	snapshot := directory.Snapshot()
	if snapshot == nil {
		return []ChannelDirectoryEntry{}
	}
	return snapshot
}

func (m *StateMirror) snapshotDocument() stateMirrorDocument {
	return stateMirrorDocument{
		HomeChannels: mirrorHomeChannels(m.homes),
		Directory:    mirrorDirectory(m.directory),
	}
}

func stateMirrorFingerprint(doc stateMirrorDocument) (string, error) {
	doc.UpdatedAt = time.Time{}
	data, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
