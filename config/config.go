package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

const (
	defaultRoleMessageHeader = "**Role Assignment**\n\nReact to this message to receive a role.\nRemoving your reaction will revoke the role automatically."
	defaultTZ                = "Asia/Taipei"
)

type AppConfig struct {
	Token             string
	RoleChannelID     string
	AdminChannelID    string
	LeetcodeChannelID string
	RoleMessageHeader string
	Location          *time.Location
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

	tz := os.Getenv("TZ")
	if tz == "" {
		tz = defaultTZ
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid TZ %q: %w", tz, err)
	}

	return &AppConfig{
		Token:             token,
		RoleChannelID:     channelID,
		AdminChannelID:    adminChannelID,
		LeetcodeChannelID: os.Getenv("LEETCODE_CHANNEL_ID"),
		RoleMessageHeader: header,
		Location:          loc,
	}, nil
}
