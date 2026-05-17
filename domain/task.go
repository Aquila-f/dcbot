package domain

import (
	"context"

	"github.com/bwmarrin/discordgo"
)

// Task is a unit of work that the scheduler runs on a cron tick.
type Task interface {
	Name() string
	Build(ctx context.Context) (Payload, error)
}

// Payload is the message a Task wants the scheduler to deliver.
type Payload struct {
	ChannelID string
	Content   string
	Embeds    []*discordgo.MessageEmbed
}

// ScheduledTask pairs a cron spec with the task to run on that schedule.
type ScheduledTask struct {
	Spec string
	Task Task
}

// TaskProvider is implemented by modules that own cron-scheduled tasks.
type TaskProvider interface {
	Tasks() []ScheduledTask
}
