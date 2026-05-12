package scheduler

import (
	"context"

	"github.com/bwmarrin/discordgo"
)

type Task interface {
	Name() string
	Build(ctx context.Context) (Payload, error)
}

type Payload struct {
	ChannelID string
	Content   string
	Embeds    []*discordgo.MessageEmbed
}
