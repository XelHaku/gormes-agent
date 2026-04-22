// Package discord adapts Discord bot traffic into the shared gateway chassis.
package discord

import "github.com/bwmarrin/discordgo"

// discordSession is the narrow surface of *discordgo.Session the adapter uses.
type discordSession interface {
	Open() error
	Close() error
	AddHandler(handler interface{}) func()
	ChannelMessageSend(channelID, content string) (*discordgo.Message, error)
	ChannelMessageEdit(channelID, messageID, content string) (*discordgo.Message, error)
	MessageReactionAdd(channelID, messageID, emoji string) error
	MessageReactionRemoveMe(channelID, messageID, emoji string) error
}
