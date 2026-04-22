package discord

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type sendCall struct {
	channelID string
	text      string
}

type editCall struct {
	channelID string
	messageID string
	text      string
}

type mockClient struct {
	mu          sync.Mutex
	selfID      string
	handler     func(InboundMessage)
	nextMessage int
	texts       []string
	sends       []sendCall
	edits       []editCall
	typingCalls int
	opened      bool
	closed      bool

	SendErr error
	EditErr error

	SendFn func(channelID, text string) (string, error)
	EditFn func(channelID, messageID, text string) error
}

var _ Client = (*mockClient)(nil)

func newMockClient(selfID string) *mockClient {
	return &mockClient{
		selfID:      selfID,
		nextMessage: 1000,
	}
}

func (m *mockClient) Open() error {
	m.mu.Lock()
	m.opened = true
	m.mu.Unlock()
	return nil
}

func (m *mockClient) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockClient) SelfID() string {
	return m.selfID
}

func (m *mockClient) SetMessageHandler(handler func(InboundMessage)) {
	m.mu.Lock()
	m.handler = handler
	m.mu.Unlock()
}

func (m *mockClient) Send(channelID string, text string) (string, error) {
	m.mu.Lock()
	id := fmt.Sprintf("msg-%d", m.nextMessage)
	m.nextMessage++
	m.texts = append(m.texts, text)
	m.sends = append(m.sends, sendCall{channelID: channelID, text: text})
	sendFn := m.SendFn
	sendErr := m.SendErr
	m.mu.Unlock()

	if sendFn != nil {
		return sendFn(channelID, text)
	}
	if sendErr != nil {
		return "", sendErr
	}
	return id, nil
}

func (m *mockClient) Edit(channelID, messageID, text string) error {
	m.mu.Lock()
	m.texts = append(m.texts, text)
	m.edits = append(m.edits, editCall{channelID: channelID, messageID: messageID, text: text})
	editFn := m.EditFn
	editErr := m.EditErr
	m.mu.Unlock()
	if editFn != nil {
		return editFn(channelID, messageID, text)
	}
	if editErr != nil {
		return editErr
	}
	return nil
}

func (m *mockClient) Typing(_ string) error {
	m.mu.Lock()
	m.typingCalls++
	m.mu.Unlock()
	return nil
}

func (m *mockClient) pushMessage(msg InboundMessage) {
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		m.mu.Lock()
		handler := m.handler
		m.mu.Unlock()
		if handler != nil {
			handler(msg)
			return
		}
		if time.Now().After(deadline) {
			panic("discord mock handler was not set before pushMessage")
		}
		time.Sleep(time.Millisecond)
	}
}

func (m *mockClient) sentTexts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.texts))
	copy(out, m.texts)
	return out
}

func (m *mockClient) lastSentText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.texts) == 0 {
		return ""
	}
	return m.texts[len(m.texts)-1]
}

func (m *mockClient) typingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.typingCalls
}

func (m *mockClient) sendCalls() []sendCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]sendCall, len(m.sends))
	copy(out, m.sends)
	return out
}

func (m *mockClient) editCalls() []editCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]editCall, len(m.edits))
	copy(out, m.edits)
	return out
}

func errEditFailed() error {
	return errors.New("edit failed")
}
