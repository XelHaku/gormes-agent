package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

type realSession struct {
	s *discordgo.Session
}

var _ discordSession = (*realSession)(nil)

func NewRealSession(token string) (discordSession, error) {
	if token == "" {
		return nil, fmt.Errorf("discord: empty bot token")
	}
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("discord: new session: %w", err)
	}
	s.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent
	return &realSession{s: s}, nil
}

func (r *realSession) Open() error  { return r.s.Open() }
func (r *realSession) Close() error { return r.s.Close() }

func (r *realSession) AddHandler(handler interface{}) func() {
	return r.s.AddHandler(handler)
}

func (r *realSession) ChannelMessageSend(channelID, content string) (*discordgo.Message, error) {
	return r.s.ChannelMessageSend(channelID, content)
}

func (r *realSession) ChannelMessageEdit(channelID, messageID, content string) (*discordgo.Message, error) {
	return r.s.ChannelMessageEdit(channelID, messageID, content)
}

func (r *realSession) MessageReactionAdd(channelID, messageID, emoji string) error {
	return r.s.MessageReactionAdd(channelID, messageID, emoji)
}

func (r *realSession) MessageReactionRemoveMe(channelID, messageID, emoji string) error {
	return r.s.MessageReactionRemove(channelID, messageID, emoji, "@me")
}
