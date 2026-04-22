package discord

import (
	"sync"

	"github.com/bwmarrin/discordgo"
)

type mockSession struct {
	mu              sync.Mutex
	opened          bool
	closed          bool
	handlers        []interface{}
	sent            []mockSent
	edits           []mockEdit
	reactionsAdded  []mockReaction
	reactionsRemove []mockReaction
	nextMsgID       int
	sendErr         error
	editErr         error
	reactionErr     error
}

type mockSent struct{ ChannelID, Content, MsgID string }
type mockEdit struct{ ChannelID, MsgID, Content string }
type mockReaction struct{ ChannelID, MsgID, Emoji string }

func newMockSession() *mockSession {
	return &mockSession{nextMsgID: 1000}
}

func (m *mockSession) Open() error {
	m.mu.Lock()
	m.opened = true
	m.mu.Unlock()
	return nil
}

func (m *mockSession) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockSession) AddHandler(handler interface{}) func() {
	m.mu.Lock()
	m.handlers = append(m.handlers, handler)
	m.mu.Unlock()
	return func() {}
}

func (m *mockSession) ChannelMessageSend(channelID, content string) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	id := nextID(&m.nextMsgID)
	m.sent = append(m.sent, mockSent{ChannelID: channelID, Content: content, MsgID: id})
	return &discordgo.Message{ID: id, ChannelID: channelID, Content: content}, nil
}

func (m *mockSession) ChannelMessageEdit(channelID, messageID, content string) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.editErr != nil {
		return nil, m.editErr
	}
	m.edits = append(m.edits, mockEdit{ChannelID: channelID, MsgID: messageID, Content: content})
	return &discordgo.Message{ID: messageID, ChannelID: channelID, Content: content}, nil
}

func (m *mockSession) MessageReactionAdd(channelID, messageID, emoji string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reactionErr != nil {
		return m.reactionErr
	}
	m.reactionsAdded = append(m.reactionsAdded, mockReaction{channelID, messageID, emoji})
	return nil
}

func (m *mockSession) MessageReactionRemoveMe(channelID, messageID, emoji string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reactionsRemove = append(m.reactionsRemove, mockReaction{channelID, messageID, emoji})
	return nil
}

func (m *mockSession) deliver(msg *discordgo.MessageCreate) bool {
	m.mu.Lock()
	handlers := append([]interface{}{}, m.handlers...)
	m.mu.Unlock()
	for _, h := range handlers {
		if fn, ok := h.(func(*discordgo.Session, *discordgo.MessageCreate)); ok {
			fn(nil, msg)
			return true
		}
	}
	return false
}

func (m *mockSession) sentSnapshot() []mockSent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockSent, len(m.sent))
	copy(out, m.sent)
	return out
}

func (m *mockSession) editsSnapshot() []mockEdit {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockEdit, len(m.edits))
	copy(out, m.edits)
	return out
}

func (m *mockSession) reactionsAddedSnapshot() []mockReaction {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockReaction, len(m.reactionsAdded))
	copy(out, m.reactionsAdded)
	return out
}

func nextID(n *int) string {
	id := *n
	*n++
	return intToString(id)
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
