package gateway

import "testing"

var (
	_ Channel            = (*fakeChannel)(nil)
	_ MessageEditor      = (*fakeChannel)(nil)
	_ PlaceholderCapable = (*fakeChannel)(nil)
	_ TypingCapable      = (*fakeChannel)(nil)
	_ ReactionCapable    = (*fakeChannel)(nil)
)

func TestChannel_NameStable(t *testing.T) {
	ch := newFakeChannel("test")
	if got := ch.Name(); got != "test" {
		t.Errorf("Name() = %q, want %q", got, "test")
	}
}
