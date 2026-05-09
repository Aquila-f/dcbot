package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	Token          string
	RoleChannelID  string
	AdminChannelID string
}

func Load() (*AppConfig, error) {
	_ = godotenv.Load()

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN is not set")
	}

	channelID := os.Getenv("ROLE_CHANNEL_ID")
	if channelID == "" {
		return nil, fmt.Errorf("ROLE_CHANNEL_ID is not set")
	}

	adminChannelID := os.Getenv("ADMIN_CHANNEL_ID")
	if adminChannelID == "" {
		return nil, fmt.Errorf("ADMIN_CHANNEL_ID is not set")
	}

	return &AppConfig{
		Token:          token,
		RoleChannelID:  channelID,
		AdminChannelID: adminChannelID,
	}, nil
}
