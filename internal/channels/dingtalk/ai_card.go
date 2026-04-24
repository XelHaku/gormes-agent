package dingtalk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

const (
	defaultAICardContentKey = "content"
	aiCardFallbackPrefix    = "fallback:"
)

// AICardConfig freezes the SDK-neutral DingTalk AI Card streaming contract.
type AICardConfig struct {
	TemplateID string
	RobotCode  string
	ContentKey string
}

// AICardCreateRequest is the SDK-neutral create-card request shape.
type AICardCreateRequest struct {
	TemplateID     string               `json:"templateId"`
	OutTrackID     string               `json:"outTrackId"`
	CallbackType   string               `json:"callbackType"`
	CardData       AICardCreateCardData `json:"cardData"`
	SupportForward bool                 `json:"supportForward"`
}

type AICardCreateCardData struct {
	CardParamMap map[string]string `json:"cardParamMap"`
}

// AICardDeliverRequest is the SDK-neutral deliver-card request shape.
type AICardDeliverRequest struct {
	OutTrackID  string `json:"outTrackId"`
	OpenSpaceID string `json:"openSpaceId"`
	UserIDType  int    `json:"userIdType"`
	RobotCode   string `json:"robotCode"`
	SpaceType   string `json:"spaceType"`
}

// AICardStreamingUpdateRequest is the DingTalk streaming update payload.
type AICardStreamingUpdateRequest struct {
	OutTrackID string `json:"outTrackId"`
	GUID       string `json:"guid"`
	Key        string `json:"key"`
	Content    string `json:"content"`
	IsFull     bool   `json:"isFull"`
	IsFinalize bool   `json:"isFinalize"`
	IsError    bool   `json:"isError"`
}

// AICardClient is the minimal real-SDK binding seam. The real DingTalk SDK
// adapter should translate these requests directly to card create, deliver,
// and streaming_update calls.
type AICardClient interface {
	CreateCard(ctx context.Context, req AICardCreateRequest) error
	DeliverCard(ctx context.Context, req AICardDeliverRequest) error
	StreamingUpdate(ctx context.Context, req AICardStreamingUpdateRequest) error
}

// AICardBot adds placeholder/edit streaming support to the existing DingTalk
// bot without changing the plain session-webhook bot contract.
type AICardBot struct {
	*Bot

	cfg    AICardConfig
	client AICardClient

	trackID func() string
	guid    func() string

	mu     sync.Mutex
	states map[string]aiCardState
}

type aiCardState struct {
	chatID   string
	fallback bool
}

type AICardOption func(*AICardBot)

func WithAICardTrackID(fn func() string) AICardOption {
	return func(b *AICardBot) {
		if fn != nil {
			b.trackID = fn
		}
	}
}

func WithAICardGUID(fn func() string) AICardOption {
	return func(b *AICardBot) {
		if fn != nil {
			b.guid = fn
		}
	}
}

func NewAICardBot(base *Bot, cfg AICardConfig, client AICardClient, opts ...AICardOption) *AICardBot {
	b := &AICardBot{
		Bot:     base,
		cfg:     cfg,
		client:  client,
		trackID: defaultAICardTrackID,
		guid:    func() string { return uuid.NewString() },
		states:  map[string]aiCardState{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}
	return b
}

var (
	_ gateway.Channel                 = (*AICardBot)(nil)
	_ gateway.MessageEditor           = (*AICardBot)(nil)
	_ gateway.PlaceholderCapable      = (*AICardBot)(nil)
	_ gateway.FinalizingMessageEditor = (*AICardBot)(nil)
)

func (b *AICardBot) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	outTrackID, err := b.createAndDeliverCard(ctx, chatID)
	if err == nil {
		b.rememberState(outTrackID, aiCardState{chatID: chatID})
		return outTrackID, nil
	}
	return b.sendFallbackPlaceholder(ctx, chatID, err)
}

func (b *AICardBot) EditMessage(ctx context.Context, chatID, msgID, text string) error {
	return b.editMessage(ctx, chatID, msgID, text, false)
}

func (b *AICardBot) EditMessageFinal(ctx context.Context, chatID, msgID, text string, finalize bool) error {
	return b.editMessage(ctx, chatID, msgID, text, finalize)
}

func (b *AICardBot) editMessage(ctx context.Context, chatID, msgID, text string, finalize bool) error {
	if b == nil || b.Bot == nil {
		return errors.New("dingtalk: ai card bot is nil")
	}
	if msgID == "" {
		return errors.New("dingtalk: ai card message id required")
	}
	if b.isFallback(msgID) {
		if finalize {
			_, err := b.Send(ctx, chatID, text)
			return err
		}
		return nil
	}
	if b.client == nil {
		return b.fallbackOrError(ctx, chatID, msgID, text, finalize, errors.New("dingtalk: ai card client is nil"))
	}

	err := b.client.StreamingUpdate(ctx, AICardStreamingUpdateRequest{
		OutTrackID: msgID,
		GUID:       b.guid(),
		Key:        b.contentKey(),
		Content:    text,
		IsFull:     true,
		IsFinalize: finalize,
		IsError:    false,
	})
	if err != nil {
		return b.fallbackOrError(ctx, chatID, msgID, text, finalize, fmt.Errorf("dingtalk: ai card streaming update: %w", err))
	}
	if finalize {
		b.forgetState(msgID)
		b.Bot.fireDoneReaction(ctx, chatID)
	}
	return nil
}

func (b *AICardBot) fallbackOrError(ctx context.Context, chatID, msgID, text string, finalize bool, cause error) error {
	b.markFallback(msgID, chatID)
	if !finalize {
		return cause
	}
	if _, err := b.Send(ctx, chatID, text); err != nil {
		return fmt.Errorf("%v; fallback send: %w", cause, err)
	}
	b.forgetState(msgID)
	return nil
}

func (b *AICardBot) createAndDeliverCard(ctx context.Context, chatID string) (string, error) {
	if b == nil || b.Bot == nil {
		return "", errors.New("dingtalk: ai card bot is nil")
	}
	if b.client == nil {
		return "", errors.New("dingtalk: ai card client is nil")
	}
	if strings.TrimSpace(b.cfg.TemplateID) == "" {
		return "", errors.New("dingtalk: ai card template id is required")
	}

	cardCtx, err := b.cardContexts.Lookup(chatID)
	if err != nil {
		return "", err
	}

	outTrackID := strings.TrimSpace(b.trackID())
	if outTrackID == "" {
		return "", errors.New("dingtalk: ai card outTrackId is required")
	}

	create := AICardCreateRequest{
		TemplateID:   strings.TrimSpace(b.cfg.TemplateID),
		OutTrackID:   outTrackID,
		CallbackType: "STREAM",
		CardData: AICardCreateCardData{
			CardParamMap: map[string]string{b.contentKey(): ""},
		},
		SupportForward: true,
	}
	if err := b.client.CreateCard(ctx, create); err != nil {
		return "", fmt.Errorf("dingtalk: ai card create: %w", err)
	}

	deliver, err := b.deliverRequest(outTrackID, cardCtx)
	if err != nil {
		return "", err
	}
	if err := b.client.DeliverCard(ctx, deliver); err != nil {
		return "", fmt.Errorf("dingtalk: ai card deliver: %w", err)
	}
	return outTrackID, nil
}

func (b *AICardBot) deliverRequest(outTrackID string, cardCtx aiCardConversationContext) (AICardDeliverRequest, error) {
	conversationType := strings.TrimSpace(cardCtx.ConversationType)
	if conversationType == "2" {
		conversationID := strings.TrimSpace(cardCtx.ConversationID)
		if conversationID == "" {
			return AICardDeliverRequest{}, errors.New("dingtalk: group ai card requires conversation id")
		}
		return AICardDeliverRequest{
			OutTrackID:  outTrackID,
			OpenSpaceID: "dtv1.card//IM_GROUP." + conversationID,
			UserIDType:  1,
			RobotCode:   strings.TrimSpace(b.cfg.RobotCode),
			SpaceType:   "IM_GROUP",
		}, nil
	}

	senderStaffID := strings.TrimSpace(cardCtx.SenderStaffID)
	if senderStaffID == "" {
		return AICardDeliverRequest{}, errors.New("dingtalk: direct ai card requires sender staff id")
	}
	return AICardDeliverRequest{
		OutTrackID:  outTrackID,
		OpenSpaceID: "dtv1.card//IM_ROBOT." + senderStaffID,
		UserIDType:  1,
		RobotCode:   strings.TrimSpace(b.cfg.RobotCode),
		SpaceType:   "IM_ROBOT",
	}, nil
}

func (b *AICardBot) sendFallbackPlaceholder(ctx context.Context, chatID string, cause error) (string, error) {
	msgID, err := b.Bot.sendSessionWebhook(ctx, chatID, "⏳")
	if err != nil {
		return "", fmt.Errorf("%v; fallback placeholder: %w", cause, err)
	}
	fallbackID := aiCardFallbackPrefix + msgID
	b.rememberState(fallbackID, aiCardState{chatID: chatID, fallback: true})
	return fallbackID, nil
}

func (b *AICardBot) contentKey() string {
	key := strings.TrimSpace(b.cfg.ContentKey)
	if key == "" {
		return defaultAICardContentKey
	}
	return key
}

func (b *AICardBot) rememberState(msgID string, state aiCardState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.states[msgID] = state
}

func (b *AICardBot) markFallback(msgID, chatID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.states[msgID]
	state.chatID = chatID
	state.fallback = true
	b.states[msgID] = state
}

func (b *AICardBot) forgetState(msgID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.states, msgID)
}

func (b *AICardBot) isFallback(msgID string) bool {
	if strings.HasPrefix(msgID, aiCardFallbackPrefix) {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.states[msgID].fallback
}

func defaultAICardTrackID() string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) > 12 {
		id = id[:12]
	}
	return "gormes_" + id
}

type aiCardConversationContext struct {
	ConversationID   string
	ConversationType string
	SenderStaffID    string
}

type aiCardContexts struct {
	values sync.Map
}

func newAICardContexts() *aiCardContexts {
	return &aiCardContexts{}
}

func (c *aiCardContexts) Remember(chatID string, ctx aiCardConversationContext) {
	if c == nil {
		return
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	c.values.Store(chatID, ctx)
}

func (c *aiCardContexts) Lookup(chatID string) (aiCardConversationContext, error) {
	if c == nil {
		return aiCardConversationContext{}, errors.New("dingtalk: ai card context store is nil")
	}
	chatID = strings.TrimSpace(chatID)
	raw, ok := c.values.Load(chatID)
	if !ok {
		return aiCardConversationContext{}, fmt.Errorf("dingtalk: no ai card context for chat %q", chatID)
	}
	cardCtx, ok := raw.(aiCardConversationContext)
	if !ok {
		return aiCardConversationContext{}, fmt.Errorf("dingtalk: invalid ai card context for chat %q", chatID)
	}
	return cardCtx, nil
}
