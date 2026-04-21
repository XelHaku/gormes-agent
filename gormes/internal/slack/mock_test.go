package slack

import (
	"context"
	"errors"
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
	ackCount   map[string]int
	callLog    []string
	outputLog  []outputCall
	threadByTS map[string]string

	AckErr    error
	PostErr   error
	UpdateErr error
	AckFn     func(string) error
	RunErr    error
	RunFn     func(context.Context, func(Event)) error
}

var _ Client = (*mockClient)(nil)

func newMockClient() *mockClient {
	return &mockClient{
		events:     make(chan Event, 16),
		nextTS:     1000,
		acked:      make(map[string]bool),
		ackCount:   make(map[string]int),
		threadByTS: make(map[string]string),
	}
}

func (m *mockClient) AuthTest(context.Context) (string, error) {
	return "UBOT", nil
}

func (m *mockClient) Run(ctx context.Context, fn func(Event)) error {
	if m.RunFn != nil {
		return m.RunFn(ctx, fn)
	}
	if m.RunErr != nil {
		return m.RunErr
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-m.events:
			fn(e)
		}
	}
}

func (m *mockClient) Ack(requestID string) error {
	m.mu.Lock()
	m.ackCount[requestID]++
	m.callLog = append(m.callLog, "ack:"+requestID)
	ackFn := m.AckFn
	ackErr := m.AckErr
	m.mu.Unlock()
	if requestID != "" {
		if ackFn != nil {
			if err := ackFn(requestID); err != nil {
				return err
			}
		}
		if ackErr != nil {
			return ackErr
		}
		m.mu.Lock()
		m.acked[requestID] = true
		m.mu.Unlock()
	}
	return nil
}

func (m *mockClient) PostMessage(_ context.Context, channelID, threadTS, text string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.PostErr != nil {
		return "", m.PostErr
	}
	ts := fmt.Sprintf("1711111111.%06d", m.nextTS)
	m.nextTS++
	m.threadByTS[ts] = threadTS
	m.callLog = append(m.callLog, "post:"+ts)
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
	if m.UpdateErr != nil {
		return m.UpdateErr
	}
	m.callLog = append(m.callLog, "update:"+ts)
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

func (m *mockClient) calls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.callLog))
	copy(out, m.callLog)
	return out
}

func (m *mockClient) ackAttempts(requestID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ackCount[requestID]
}

func (m *mockClient) rememberThread(ts, threadTS string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.threadByTS[ts] = threadTS
}

func errUpdateFailed() error {
	return errors.New("update failed")
}
