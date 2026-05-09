package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

const defaultRoleMessageHeader = "**Role Assignment**\n\nReact to this message to receive a role.\nRemoving your reaction will revoke the role automatically."

type AppConfig struct {
	Token             string
	RoleChannelID     string
	AdminChannelID    string
	RoleMessageHeader string
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

	header := os.Getenv("ROLE_MESSAGE_HEADER")
	if header == "" {
		header = defaultRoleMessageHeader
	}

	return &AppConfig{
		Token:             token,
		RoleChannelID:     channelID,
		AdminChannelID:    adminChannelID,
		RoleMessageHeader: header,
	}, nil
}
