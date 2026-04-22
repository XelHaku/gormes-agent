package homeassistant

import (
	"context"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_Run_WatchesDomainsAndFormatsClimateEvents(t *testing.T) {
	mc := newMockClient()
	b := New(Config{WatchDomains: []string{"climate"}}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.push(StateChangeEvent{
		EntityID:     "sensor.office_temperature",
		FriendlyName: "Office Temperature",
		OldState:     "21",
		NewState:     "22",
		Unit:         "°C",
	})
	assertNoInbound(t, inbox)

	mc.push(StateChangeEvent{
		EntityID:           "climate.hallway",
		FriendlyName:       "Hall Thermostat",
		OldState:           "off",
		NewState:           "heat",
		CurrentTemperature: "21",
		TargetTemperature:  "23",
	})

	select {
	case ev := <-inbox:
		if ev.Platform != "homeassistant" || ev.ChatID != "ha_events" || ev.UserID != "homeassistant" {
			t.Fatalf("unexpected event identity: %+v", ev)
		}
		if ev.Kind != gateway.EventSubmit {
			t.Fatalf("Kind = %v, want %v", ev.Kind, gateway.EventSubmit)
		}
		want := "[Home Assistant] Hall Thermostat: HVAC mode changed from 'off' to 'heat' (current: 21, target: 23)"
		if ev.Text != want {
			t.Fatalf("Text = %q, want %q", ev.Text, want)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no inbound event")
	}
}

func TestBot_Run_SuppressesEventsInsideCooldown(t *testing.T) {
	mc := newMockClient()
	now := time.Unix(1_700_000_000, 0)
	b := New(Config{
		WatchAll: true,
		Cooldown: 30 * time.Second,
	}, mc, nil)
	b.now = func() time.Time { return now }
	inbox := make(chan gateway.InboundEvent, 2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	first := StateChangeEvent{
		EntityID:     "light.kitchen",
		FriendlyName: "Kitchen Light",
		OldState:     "off",
		NewState:     "on",
	}
	second := StateChangeEvent{
		EntityID:     "light.kitchen",
		FriendlyName: "Kitchen Light",
		OldState:     "on",
		NewState:     "off",
	}

	mc.push(first)
	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected first inbound event")
	}

	mc.push(second)
	assertNoInbound(t, inbox)

	now = now.Add(31 * time.Second)
	mc.push(first)
	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event after cooldown expiry")
	}
}

func TestBot_Send_UsesPersistentNotificationTitle(t *testing.T) {
	mc := newMockClient()
	b := New(Config{}, mc, nil)

	msgID, err := b.Send(context.Background(), "ignored", "all good")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if msgID != "notify-1" {
		t.Fatalf("Send() msgID = %q, want notify-1", msgID)
	}
	if len(mc.notifications) != 1 {
		t.Fatalf("notification count = %d, want 1", len(mc.notifications))
	}
	if mc.notifications[0].Title != "Hermes Agent" {
		t.Fatalf("title = %q, want Hermes Agent", mc.notifications[0].Title)
	}
	if mc.notifications[0].Message != "all good" {
		t.Fatalf("message = %q, want all good", mc.notifications[0].Message)
	}
}

type mockClient struct {
	events         chan StateChangeEvent
	notifications  []notification
	notificationID int
}

type notification struct {
	Title   string
	Message string
}

func newMockClient() *mockClient {
	return &mockClient{events: make(chan StateChangeEvent, 16)}
}

func (m *mockClient) Events() <-chan StateChangeEvent { return m.events }

func (m *mockClient) SendNotification(_ context.Context, title, message string) (string, error) {
	m.notifications = append(m.notifications, notification{Title: title, Message: message})
	m.notificationID++
	return "notify-" + string(rune('0'+m.notificationID)), nil
}

func (m *mockClient) Close() error { return nil }

func (m *mockClient) push(ev StateChangeEvent) {
	m.events <- ev
}

func assertNoInbound(t *testing.T, inbox <-chan gateway.InboundEvent) {
	t.Helper()
	select {
	case ev := <-inbox:
		t.Fatalf("expected no inbound event, got %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}
