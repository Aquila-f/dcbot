package main

import (
	"context"
	"log"

	"dcbot/config"
	"dcbot/scheduler/tasks"

	"github.com/bwmarrin/discordgo"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.LeetcodeChannelID == "" {
		log.Fatal("LEETCODE_CHANNEL_ID is not set")
	}

	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatalf("session: %v", err)
	}
	if err := session.Open(); err != nil {
		log.Fatalf("open: %v", err)
	}
	defer session.Close()

	task := &tasks.LeetcodeDaily{ChannelID: cfg.LeetcodeChannelID}
	payload, err := task.Build(context.Background())
	if err != nil {
		log.Fatalf("build: %v", err)
	}

	msg, err := session.ChannelMessageSendComplex(payload.ChannelID, &discordgo.MessageSend{
		Content: payload.Content,
		Embeds:  payload.Embeds,
	})
	if err != nil {
		log.Fatalf("send: %v", err)
	}
	log.Printf("sent message %s to channel %s", msg.ID, payload.ChannelID)
}
