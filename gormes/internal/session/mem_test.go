package session

import (
	"context"
	"sync"
	"testing"
)

func TestMemMap_PutGetRoundTrip(t *testing.T) {
	m := NewMemMap()
	ctx := context.Background()

	if err := m.Put(ctx, "telegram:42", "sess-abc"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := m.Get(ctx, "telegram:42")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "sess-abc" {
		t.Errorf("Get = %q, want %q", got, "sess-abc")
	}
}

func TestMemMap_GetMissingReturnsEmpty(t *testing.T) {
	m := NewMemMap()
	got, err := m.Get(context.Background(), "telegram:999")
	if err != nil {
		t.Errorf("Get on missing key should not error, got %v", err)
	}
	if got != "" {
		t.Errorf("Get on missing key = %q, want \"\"", got)
	}
}

func TestMemMap_PutEmptyDeletes(t *testing.T) {
	m := NewMemMap()
	ctx := context.Background()
	_ = m.Put(ctx, "tui:default", "sess-x")
	_ = m.Put(ctx, "tui:default", "")
	got, _ := m.Get(ctx, "tui:default")
	if got != "" {
		t.Errorf("after Put(\"\"), Get = %q, want deleted (\"\")", got)
	}
}

func TestMemMap_CloseIdempotent(t *testing.T) {
	m := NewMemMap()
	if err := m.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close should be no-op, got %v", err)
	}
}

func TestMemMap_CtxCancelShortCircuits(t *testing.T) {
	m := NewMemMap()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Get(ctx, "k"); err == nil {
		t.Errorf("Get on canceled ctx should return ctx.Err(), got nil")
	}
	if err := m.Put(ctx, "k", "v"); err == nil {
		t.Errorf("Put on canceled ctx should return ctx.Err(), got nil")
	}
}

func TestMemMap_ConcurrentSafe(t *testing.T) {
	m := NewMemMap()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) { defer wg.Done(); _ = m.Put(context.Background(), "k", "v") }(i)
		go func(i int) { defer wg.Done(); _, _ = m.Get(context.Background(), "k") }(i)
	}
	wg.Wait()
}

func TestTUIKey(t *testing.T) {
	if TUIKey() != "tui:default" {
		t.Errorf("TUIKey() = %q, want %q", TUIKey(), "tui:default")
	}
}

func TestTelegramKey(t *testing.T) {
	if got := TelegramKey(5551234567); got != "telegram:5551234567" {
		t.Errorf("TelegramKey(5551234567) = %q, want %q", got, "telegram:5551234567")
	}
	if got := TelegramKey(-100); got != "telegram:-100" {
		t.Errorf("TelegramKey(-100) = %q, want %q", got, "telegram:-100")
	}
}

func TestDiscordKey(t *testing.T) {
	if got := DiscordKey("123456789"); got != "discord:123456789" {
		t.Errorf("DiscordKey(123456789) = %q, want %q", got, "discord:123456789")
	}
}
