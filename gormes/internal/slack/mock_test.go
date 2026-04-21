package slack

import (
	"context"
	"fmt"
	"sync"
)

type outputCall struct {
	channelID string
	threadTS  string
	ts        string
	text      string
	updated   bool
}

type mockClient struct {
	mu         sync.Mutex
	events     chan Event
	nextTS     int
	acked      map[string]bool
	outputLog  []outputCall
	threadByTS map[string]string
}

var _ Client = (*mockClient)(nil)

func newMockClient() *mockClient {
	return &mockClient{
		events:     make(chan Event, 16),
		nextTS:     1000,
		acked:      make(map[string]bool),
		threadByTS: make(map[string]string),
	}
}

func (m *mockClient) AuthTest(context.Context) (string, error) {
	return "UBOT", nil
}

func (m *mockClient) Run(ctx context.Context, fn func(Event)) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-m.events:
			fn(e)
		}
	}
}

func (m *mockClient) Ack(requestID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if requestID != "" {
		m.acked[requestID] = true
	}
}

func (m *mockClient) PostMessage(_ context.Context, channelID, threadTS, text string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ts := fmt.Sprintf("1711111111.%06d", m.nextTS)
	m.nextTS++
	m.threadByTS[ts] = threadTS
	m.outputLog = append(m.outputLog, outputCall{
		channelID: channelID,
		threadTS:  threadTS,
		ts:        ts,
		text:      text,
	})
	return ts, nil
}

func (m *mockClient) UpdateMessage(_ context.Context, channelID, ts, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputLog = append(m.outputLog, outputCall{
		channelID: channelID,
		threadTS:  m.threadByTS[ts],
		ts:        ts,
		text:      text,
		updated:   true,
	})
	return nil
}

func (m *mockClient) pushEvent(e Event) {
	m.events <- e
}

func (m *mockClient) wasAcked(requestID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.acked[requestID]
}

func (m *mockClient) outputs() []outputCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]outputCall, len(m.outputLog))
	copy(out, m.outputLog)
	return out
}

func (m *mockClient) lastOutputText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.outputLog) == 0 {
		return ""
	}
	return m.outputLog[len(m.outputLog)-1].text
}

func (m *mockClient) lastThreadTS() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.outputLog) == 0 {
		return ""
	}
	return m.outputLog[len(m.outputLog)-1].threadTS
}
