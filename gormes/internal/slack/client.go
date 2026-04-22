package slack

import "context"

type Event struct {
	RequestID string
	ChannelID string
	UserID    string
	Text      string
	Timestamp string
	ThreadTS  string
	SubType   string
	BotID     string
}

type Client interface {
	AuthTest(context.Context) (string, error)
	Run(context.Context, func(Event)) error
	Ack(requestID string) error
	PostMessage(ctx context.Context, channelID, threadTS, text string) (string, error)
	UpdateMessage(ctx context.Context, channelID, ts, text string) error
}

func SessionKey(channelID string) string {
	return "slack:" + channelID
}
