package domain

import "github.com/bwmarrin/discordgo"

// InteractionHandler handles a Discord application command interaction.
type InteractionHandler func(s *discordgo.Session, i *discordgo.InteractionCreate)

// Command pairs an application command definition with the handler that runs it.
type Command struct {
	Definition *discordgo.ApplicationCommand
	Handler    InteractionHandler
}
