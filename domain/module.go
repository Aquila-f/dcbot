package domain

import (
	"context"

	"github.com/bwmarrin/discordgo"
)

// Module is the base tag for components plugged into the bot.
// Name is used in startup and error logs so we know which module reported.
type Module interface {
	Name() string
}

// CommandProvider is implemented by modules that expose slash commands.
type CommandProvider interface {
	Commands() []Command
}

// EventSubscriber is implemented by modules that need to attach gateway
// event handlers (reactions, messages, members, etc.).
type EventSubscriber interface {
	RegisterHandlers(s *discordgo.Session)
}

// ReadyHook is implemented by modules that need to run work after the
// gateway READY event (e.g. reconciling state with Discord).
// The ctx is cancelled when the bot is stopping.
type ReadyHook interface {
	OnReady(ctx context.Context, s *discordgo.Session, r *discordgo.Ready) error
}
