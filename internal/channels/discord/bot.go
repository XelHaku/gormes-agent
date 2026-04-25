package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

const (
	ackEmoji        = "👀"
	placeholderText = "⏳"
)

type Config struct {
	AllowedChannelID  string
	FirstRunDiscovery bool
}

type Bot struct {
	cfg     Config
	session discordSession
	log     *slog.Logger

	reactionsMu sync.Mutex
	reactions   map[string]bool

	threadsMu sync.RWMutex
	threads   map[string]discordThread
}

type discordThread struct {
	id       string
	parentID string
	name     string
}

var (
	_ gateway.Channel            = (*Bot)(nil)
	_ gateway.DisconnectCapable  = (*Bot)(nil)
	_ gateway.MessageEditor      = (*Bot)(nil)
	_ gateway.PlaceholderCapable = (*Bot)(nil)
	_ gateway.ReactionCapable    = (*Bot)(nil)
)

func New(cfg Config, session discordSession, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	return &Bot{
		cfg:       cfg,
		session:   session,
		log:       log,
		reactions: map[string]bool{},
		threads:   map[string]discordThread{},
	}
}

func (b *Bot) Name() string { return "discord" }

func isForumChannel(ch *discordgo.Channel) bool {
	return ch != nil && ch.Type == discordgo.ChannelTypeGuildForum
}

func (b *Bot) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	b.session.AddHandler(func(_ *discordgo.Session, t *discordgo.ThreadCreate) {
		if t == nil {
			return
		}
		ev, ok := b.toThreadLifecycleEvent(t.Channel)
		if !ok {
			return
		}
		select {
		case inbox <- ev:
		case <-ctx.Done():
		}
	})
	b.session.AddHandler(func(_ *discordgo.Session, t *discordgo.ThreadUpdate) {
		if t == nil {
			return
		}
		ev, ok := b.toThreadLifecycleEvent(t.Channel)
		if !ok {
			return
		}
		select {
		case inbox <- ev:
		case <-ctx.Done():
		}
	})
	b.session.AddHandler(func(_ *discordgo.Session, t *discordgo.ThreadDelete) {
		if t == nil {
			return
		}
		ev, ok := b.toThreadLifecycleEvent(t.Channel)
		if !ok {
			return
		}
		ev.ThreadLifecycle.State = gateway.ThreadLifecycleClosed
		select {
		case inbox <- ev:
		case <-ctx.Done():
		}
	})
	b.session.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		if m == nil || m.Message == nil {
			return
		}
		if m.Author == nil || m.Author.Bot {
			return
		}
		ev, ok := b.toInboundEvent(m.Message)
		if !ok {
			return
		}
		select {
		case inbox <- ev:
		case <-ctx.Done():
		}
	})
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("discord: open session: %w", err)
	}
	<-ctx.Done()
	_ = b.Disconnect(ctx)
	return nil
}

func (b *Bot) Disconnect(context.Context) error {
	return b.session.Close()
}

func (b *Bot) toInboundEvent(m *discordgo.Message) (gateway.InboundEvent, bool) {
	text := strings.TrimSpace(m.Content)
	kind, body := gateway.ParseInboundText(text)

	userID := ""
	if m.Author != nil {
		userID = m.Author.ID
	}
	chatID := m.ChannelID
	threadID := ""
	chatName := ""
	if thread, ok := b.threadForMessageChannel(m.ChannelID); ok {
		chatID = thread.parentID
		threadID = thread.id
		chatName = thread.name
	}
	return gateway.InboundEvent{
		Platform: "discord",
		ChatID:   chatID,
		ChatName: chatName,
		UserID:   userID,
		ThreadID: threadID,
		MsgID:    m.ID,
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) rememberThread(ch *discordgo.Channel) {
	if ch == nil || !ch.IsThread() || strings.TrimSpace(ch.ID) == "" {
		return
	}
	parentID := strings.TrimSpace(ch.ParentID)
	if parentID == "" {
		return
	}
	b.threadsMu.Lock()
	defer b.threadsMu.Unlock()
	b.threads[ch.ID] = discordThread{
		id:       strings.TrimSpace(ch.ID),
		parentID: parentID,
		name:     strings.TrimSpace(ch.Name),
	}
}

func (b *Bot) threadForMessageChannel(channelID string) (discordThread, bool) {
	b.threadsMu.RLock()
	defer b.threadsMu.RUnlock()
	thread, ok := b.threads[strings.TrimSpace(channelID)]
	return thread, ok
}

func (b *Bot) toThreadLifecycleEvent(ch *discordgo.Channel) (gateway.InboundEvent, bool) {
	if ch == nil || !ch.IsThread() || strings.TrimSpace(ch.ID) == "" || strings.TrimSpace(ch.ParentID) == "" {
		return gateway.InboundEvent{}, false
	}
	b.rememberThread(ch)

	archived := false
	locked := false
	if ch.ThreadMetadata != nil {
		archived = ch.ThreadMetadata.Archived
		locked = ch.ThreadMetadata.Locked
	}

	state := gateway.ThreadLifecycleOpen
	switch {
	case locked:
		state = gateway.ThreadLifecycleClosed
	case archived:
		state = gateway.ThreadLifecycleArchived
	}

	threadID := strings.TrimSpace(ch.ID)
	parentID := strings.TrimSpace(ch.ParentID)
	name := strings.TrimSpace(ch.Name)
	return gateway.InboundEvent{
		Platform: "discord",
		ChatID:   parentID,
		ChatName: name,
		ThreadID: threadID,
		Kind:     gateway.EventThreadLifecycle,
		ThreadLifecycle: &gateway.ThreadLifecycleEvent{
			ID:       threadID,
			ParentID: parentID,
			Name:     name,
			State:    state,
			Archived: archived,
			Locked:   locked,
		},
	}, true
}

func (b *Bot) Send(_ context.Context, chatID, text string) (string, error) {
	msg, err := b.session.ChannelMessageSend(chatID, text)
	if err != nil {
		return "", fmt.Errorf("discord: send: %w", err)
	}
	return msg.ID, nil
}

func (b *Bot) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	return b.Send(ctx, chatID, placeholderText)
}

func (b *Bot) EditMessage(_ context.Context, chatID, msgID, text string) error {
	if _, err := b.session.ChannelMessageEdit(chatID, msgID, text); err != nil {
		return fmt.Errorf("discord: edit: %w", err)
	}
	return nil
}

func (b *Bot) ReactToMessage(_ context.Context, chatID, msgID string) (func(), error) {
	if err := b.session.MessageReactionAdd(chatID, msgID, ackEmoji); err != nil {
		return nil, fmt.Errorf("discord: reaction add: %w", err)
	}

	key := chatID + ":" + msgID
	return func() {
		b.reactionsMu.Lock()
		if b.reactions[key] {
			b.reactionsMu.Unlock()
			return
		}
		b.reactions[key] = true
		b.reactionsMu.Unlock()
		_ = b.session.MessageReactionRemoveMe(chatID, msgID, ackEmoji)
	}, nil
}
