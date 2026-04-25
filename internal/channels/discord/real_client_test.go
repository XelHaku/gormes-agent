package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestNewRealSession_EnablesForumThreadLifecycleIntents(t *testing.T) {
	session, err := NewRealSession("token")
	if err != nil {
		t.Fatalf("NewRealSession() error = %v", err)
	}

	real, ok := session.(*realSession)
	if !ok {
		t.Fatalf("NewRealSession() returned %T, want *realSession", session)
	}

	want := discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent
	if got := real.s.Identify.Intents; got&want != want {
		t.Fatalf("Identify.Intents = %v, want all bits %v", got, want)
	}
}
