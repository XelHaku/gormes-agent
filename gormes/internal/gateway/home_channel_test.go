package gateway

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestHomeChannels_SetFromInboundRecordsOwnerAndResolvesPlatformTarget(t *testing.T) {
	homes := NewHomeChannels()
	home, ok := homes.SetFromInbound(InboundEvent{
		Platform: " Telegram ",
		ChatID:   " 42 ",
		ChatName: " Ops Room ",
		UserID:   " u7 ",
		UserName: " Ada ",
		ThreadID: " topic-9 ",
	})
	if !ok {
		t.Fatal("SetFromInbound returned ok=false for a complete inbound event")
	}
	if home.Platform != "telegram" || home.ChatID != "42" || home.ChatName != "Ops Room" ||
		home.SetByUserID != "u7" || home.SetByUserName != "Ada" || home.ThreadID != "topic-9" {
		t.Fatalf("home = %+v, want normalized chat and owner metadata", home)
	}

	target, err := ParseDeliveryTarget("telegram", nil)
	if err != nil {
		t.Fatalf("ParseDeliveryTarget: %v", err)
	}
	got, err := ResolveDeliveryTarget(target, homes)
	if err != nil {
		t.Fatalf("ResolveDeliveryTarget: %v", err)
	}
	want := DeliveryTarget{Platform: "telegram", ChatID: "42", ThreadID: "topic-9", IsHome: true}
	if got != want {
		t.Fatalf("resolved target = %+v, want %+v", got, want)
	}
}

func TestResolveDeliveryTarget_PlatformTargetRequiresHomeChannel(t *testing.T) {
	target, err := ParseDeliveryTarget("discord", nil)
	if err != nil {
		t.Fatalf("ParseDeliveryTarget: %v", err)
	}
	if _, err := ResolveDeliveryTarget(target, NewHomeChannels()); err == nil {
		t.Fatal("ResolveDeliveryTarget error = nil, want missing-home error")
	}
}

func TestResolveDeliveryTarget_ExplicitTargetBypassesHomeChannel(t *testing.T) {
	homes := NewHomeChannels()
	homes.SetFromInbound(InboundEvent{Platform: "telegram", ChatID: "home"})

	target := DeliveryTarget{Platform: "telegram", ChatID: "explicit", ThreadID: "thread", IsExplicit: true}
	got, err := ResolveDeliveryTarget(target, homes)
	if err != nil {
		t.Fatalf("ResolveDeliveryTarget: %v", err)
	}
	if got != target {
		t.Fatalf("resolved explicit target = %+v, want unchanged %+v", got, target)
	}
}

func TestManager_Inbound_SetHomeRecordsHomeChannelAndAcknowledges(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}
	homes := NewHomeChannels()

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		HomeChannels: homes,
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram",
		ChatID:   "42",
		ChatName: "Ops Room",
		UserID:   "u7",
		UserName: "Ada",
		Kind:     EventSetHome,
		Text:     "/sethome",
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		_, ok := homes.Lookup("telegram")
		return ok && len(tg.sentSnapshot()) == 1
	})

	home, ok := homes.Lookup("telegram")
	if !ok {
		t.Fatal("home channel was not recorded")
	}
	if home.ChatID != "42" || home.ChatName != "Ops Room" || home.SetByUserID != "u7" {
		t.Fatalf("home = %+v, want inbound chat and owner metadata", home)
	}
	if got := tg.sentSnapshot()[0].Text; !strings.Contains(got, "Home channel set to **Ops Room**") {
		t.Fatalf("ack = %q, want home-channel confirmation", got)
	}
	if n := len(fk.submitsSnapshot()); n != 0 {
		t.Fatalf("sethome should not submit to kernel, got %d submits", n)
	}
}
