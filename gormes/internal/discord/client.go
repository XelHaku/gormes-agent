package discord

type InboundMessage struct {
	ID           string
	ChannelID    string
	GuildID      string
	AuthorID     string
	Content      string
	IsDM         bool
	MentionedBot bool
}

type Client interface {
	Open() error
	Close() error
	SelfID() string
	SetMessageHandler(func(InboundMessage))
	Send(channelID, text string) (string, error)
	Edit(channelID, messageID, text string) error
	Typing(channelID string) error
}

func SessionKey(channelID string) string {
	return "discord:" + channelID
}
