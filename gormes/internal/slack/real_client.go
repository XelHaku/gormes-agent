package slack

import (
	"context"
	"sync"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type realClient struct {
	api    *slackapi.Client
	socket *socketmode.Client
	events <-chan socketmode.Event
	runFn  func(context.Context) error
	ackFn  func(socketmode.Request) error

	mu      sync.Mutex
	pending map[string]socketmode.Request
}

var _ Client = (*realClient)(nil)

func NewRealClient(botToken, appToken string) Client {
	api := slackapi.New(botToken, slackapi.OptionAppLevelToken(appToken))
	socket := socketmode.New(api)
	return &realClient{
		api:     api,
		socket:  socket,
		events:  socket.Events,
		runFn:   socket.RunContext,
		ackFn:   func(req socketmode.Request) error { return socket.Ack(req) },
		pending: make(map[string]socketmode.Request),
	}
}

func (c *realClient) AuthTest(ctx context.Context) (string, error) {
	resp, err := c.api.AuthTestContext(ctx)
	if err != nil {
		return "", err
	}
	return resp.UserID, nil
}

func (c *realClient) Run(ctx context.Context, fn func(Event)) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.runFn(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			err := <-errCh
			if err == nil || ctx.Err() != nil {
				return nil
			}
			return err
		case err := <-errCh:
			if ctx.Err() != nil {
				return nil
			}
			return err
		case evt, ok := <-c.events:
			if !ok {
				err := <-errCh
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			c.handleSocketEvent(evt, fn)
		}
	}
}

func (c *realClient) Ack(requestID string) error {
	if requestID == "" {
		return nil
	}

	c.mu.Lock()
	req, ok := c.pending[requestID]
	c.mu.Unlock()
	if !ok {
		return nil
	}
	if err := c.ackFn(req); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.pending, requestID)
	c.mu.Unlock()
	return nil
}

func (c *realClient) PostMessage(ctx context.Context, channelID, threadTS, text string) (string, error) {
	opts := []slackapi.MsgOption{slackapi.MsgOptionText(text, false)}
	if threadTS != "" {
		opts = append(opts, slackapi.MsgOptionTS(threadTS))
	}
	_, ts, err := c.api.PostMessageContext(ctx, channelID, opts...)
	if err != nil {
		return "", err
	}
	return ts, nil
}

func (c *realClient) UpdateMessage(ctx context.Context, channelID, ts, text string) error {
	_, _, _, err := c.api.UpdateMessageContext(ctx, channelID, ts, slackapi.MsgOptionText(text, false))
	return err
}

func (c *realClient) handleSocketEvent(evt socketmode.Event, fn func(Event)) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		c.handleEventsAPI(evt, fn)
	case socketmode.EventTypeInteractive, socketmode.EventTypeSlashCommand:
		_ = c.autoAck(evt.Request)
	}
}

func (c *realClient) handleEventsAPI(evt socketmode.Event, fn func(Event)) {
	if evt.Request == nil {
		return
	}

	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		_ = c.autoAck(evt.Request)
		return
	}
	if eventsAPIEvent.Type != slackevents.CallbackEvent {
		_ = c.autoAck(evt.Request)
		return
	}

	msg, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.MessageEvent)
	if !ok || msg == nil {
		_ = c.autoAck(evt.Request)
		return
	}

	requestID := evt.Request.EnvelopeID
	c.mu.Lock()
	c.pending[requestID] = *evt.Request
	c.mu.Unlock()

	fn(Event{
		RequestID: requestID,
		ChannelID: msg.Channel,
		UserID:    msg.User,
		Text:      msg.Text,
		Timestamp: msg.TimeStamp,
		ThreadTS:  msg.ThreadTimeStamp,
		SubType:   msg.SubType,
		BotID:     msg.BotID,
	})
}

func (c *realClient) autoAck(req *socketmode.Request) error {
	if req == nil {
		return nil
	}
	return c.ackFn(*req)
}
