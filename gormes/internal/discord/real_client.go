package discord

import (
	"sync"

	"github.com/bwmarrin/discordgo"
)

type realClient struct {
	session *discordgo.Session
	selfID  string

	mu      sync.Mutex
	handler func(InboundMessage)
}

var _ Client = (*realClient)(nil)

func NewRealClient(token string) (Client, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	s.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent
	s.StateEnabled = true
	s.ShouldReconnectOnError = true

	self, err := s.User("@me")
	if err != nil {
		return nil, err
	}

	c := &realClient{
		session: s,
		selfID:  self.ID,
	}
	s.AddHandler(c.handleMessageCreate)
	return c, nil
}

func (c *realClient) Open() error {
	return c.session.Open()
}

func (c *realClient) Close() error {
	return c.session.Close()
}

func (c *realClient) SelfID() string {
	return c.selfID
}

func (c *realClient) SetMessageHandler(handler func(InboundMessage)) {
	c.mu.Lock()
	c.handler = handler
	c.mu.Unlock()
}

func (c *realClient) Send(channelID, text string) (string, error) {
	msg, err := c.session.ChannelMessageSend(channelID, text)
	if err != nil {
		return "", err
	}
	return msg.ID, nil
}

func (c *realClient) Edit(channelID, messageID, text string) error {
	_, err := c.session.ChannelMessageEdit(channelID, messageID, text)
	return err
}

func (c *realClient) Typing(channelID string) error {
	return c.session.ChannelTyping(channelID)
}

func (c *realClient) handleMessageCreate(_ *discordgo.Session, m *discordgo.MessageCreate) {
	if m == nil || m.Message == nil || m.Author == nil {
		return
	}
	if m.Author.ID == c.selfID {
		return
	}

	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler == nil {
		return
	}

	handler(InboundMessage{
		ID:           m.ID,
		ChannelID:    m.ChannelID,
		GuildID:      m.GuildID,
		AuthorID:     m.Author.ID,
		Content:      m.Content,
		IsDM:         m.GuildID == "",
		MentionedBot: messageMentionsUser(m.Message, c.selfID),
	})
}

func messageMentionsUser(m *discordgo.Message, userID string) bool {
	if m == nil || userID == "" {
		return false
	}
	for _, user := range m.Mentions {
		if user != nil && user.ID == userID {
			return true
		}
	}
	return false
}
