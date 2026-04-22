package homeassistant

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

const defaultCooldown = 30 * time.Second

// Config captures the first-pass Home Assistant event contract.
type Config struct {
	WatchDomains   []string
	WatchEntities  []string
	IgnoreEntities []string
	WatchAll       bool
	Cooldown       time.Duration
}

// StateChangeEvent is the WebSocket-neutral Home Assistant state payload.
type StateChangeEvent struct {
	EntityID           string
	FriendlyName       string
	OldState           string
	NewState           string
	Unit               string
	CurrentTemperature string
	TargetTemperature  string
}

// Client is the minimal Home Assistant surface used by the adapter contract.
type Client interface {
	Events() <-chan StateChangeEvent
	SendNotification(ctx context.Context, title, message string) (string, error)
	Close() error
}

// Bot adapts Home Assistant state changes into the shared gateway contract.
type Bot struct {
	cfg    Config
	client Client
	log    *slog.Logger

	watchDomains  map[string]struct{}
	watchEntities map[string]struct{}
	ignore        map[string]struct{}

	mu       sync.Mutex
	lastSeen map[string]time.Time
	now      func() time.Time
}

var _ gateway.Channel = (*Bot)(nil)

func New(cfg Config, client Client, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = defaultCooldown
	}
	return &Bot{
		cfg:           cfg,
		client:        client,
		log:           log,
		watchDomains:  toSet(cfg.WatchDomains),
		watchEntities: toSet(cfg.WatchEntities),
		ignore:        toSet(cfg.IgnoreEntities),
		lastSeen:      map[string]time.Time{},
		now:           time.Now,
	}
}

func (b *Bot) Name() string { return "homeassistant" }

func (b *Bot) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	events := b.client.Events()
	for {
		select {
		case <-ctx.Done():
			_ = b.client.Close()
			return nil
		case msg, ok := <-events:
			if !ok {
				_ = b.client.Close()
				return nil
			}
			ev, ok := b.toInboundEvent(msg)
			if !ok {
				continue
			}
			select {
			case inbox <- ev:
			case <-ctx.Done():
				_ = b.client.Close()
				return nil
			}
		}
	}
}

func (b *Bot) Send(ctx context.Context, _ string, text string) (string, error) {
	text = trim(text)
	if text == "" {
		return "", fmt.Errorf("homeassistant: notification text is empty")
	}
	return b.client.SendNotification(ctx, "Hermes Agent", text)
}

func (b *Bot) toInboundEvent(msg StateChangeEvent) (gateway.InboundEvent, bool) {
	entityID := trim(msg.EntityID)
	if entityID == "" {
		return gateway.InboundEvent{}, false
	}
	if _, ignored := b.ignore[entityID]; ignored {
		return gateway.InboundEvent{}, false
	}
	if !b.shouldWatch(entityID) {
		return gateway.InboundEvent{}, false
	}
	if trim(msg.NewState) == "" || trim(msg.OldState) == trim(msg.NewState) {
		return gateway.InboundEvent{}, false
	}
	if !b.allowByCooldown(entityID) {
		return gateway.InboundEvent{}, false
	}

	message := formatStateChange(msg)
	if message == "" {
		return gateway.InboundEvent{}, false
	}

	kind, body := gateway.ParseInboundText(message)
	return gateway.InboundEvent{
		Platform: "homeassistant",
		ChatID:   "ha_events",
		ChatName: "Home Assistant Events",
		UserID:   "homeassistant",
		UserName: "Home Assistant",
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) shouldWatch(entityID string) bool {
	domain := entityDomain(entityID)
	if len(b.watchDomains) > 0 || len(b.watchEntities) > 0 {
		_, domainMatch := b.watchDomains[domain]
		_, entityMatch := b.watchEntities[entityID]
		return domainMatch || entityMatch
	}
	return b.cfg.WatchAll
}

func (b *Bot) allowByCooldown(entityID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	last := b.lastSeen[entityID]
	if !last.IsZero() && now.Sub(last) < b.cfg.Cooldown {
		return false
	}
	b.lastSeen[entityID] = now
	return true
}

func formatStateChange(msg StateChangeEvent) string {
	entityID := trim(msg.EntityID)
	friendly := trim(msg.FriendlyName)
	if friendly == "" {
		friendly = entityID
	}
	oldState := trim(msg.OldState)
	newState := trim(msg.NewState)
	domain := entityDomain(entityID)

	switch domain {
	case "climate":
		current := fallback(trim(msg.CurrentTemperature), "?")
		target := fallback(trim(msg.TargetTemperature), "?")
		return fmt.Sprintf("[Home Assistant] %s: HVAC mode changed from '%s' to '%s' (current: %s, target: %s)", friendly, oldState, newState, current, target)
	case "sensor":
		unit := trim(msg.Unit)
		return fmt.Sprintf("[Home Assistant] %s: changed from %s%s to %s%s", friendly, oldState, unit, newState, unit)
	case "binary_sensor":
		return fmt.Sprintf("[Home Assistant] %s: %s (was %s)", friendly, onOffWord(newState, "triggered", "cleared"), onOffWord(oldState, "triggered", "cleared"))
	case "light", "switch", "fan":
		return fmt.Sprintf("[Home Assistant] %s: turned %s", friendly, onOffWord(newState, "on", "off"))
	default:
		return fmt.Sprintf("[Home Assistant] %s (%s): changed from '%s' to '%s'", friendly, entityID, oldState, newState)
	}
}

func entityDomain(entityID string) string {
	entityID = trim(entityID)
	if i := strings.IndexByte(entityID, '.'); i >= 0 {
		return entityID[:i]
	}
	return ""
}

func onOffWord(state, on, off string) string {
	if strings.EqualFold(trim(state), "on") {
		return on
	}
	return off
}

func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func toSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = trim(value)
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}

func trim(value string) string {
	return strings.TrimSpace(value)
}
