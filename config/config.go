package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	defaultRoleMessageHeader = "**Role Assignment**\n\nReact to this message to receive a role.\nRemoving your reaction will revoke the role automatically."
	defaultTZ                = "Asia/Taipei"

	defaultLLMEndpoint    = "http://localhost:8000/v1/chat/completions"
	defaultLLMModel       = "gemma-pro"
	defaultLLMMaxTokens   = 10240
	defaultLLMTemperature = 0.9
	defaultLLMHistory     = 5
	defaultLLMTimeoutSec  = 30
)

type LLMConfig struct {
	Endpoint         string
	Model            string
	MaxTokens        int
	Temperature      float64
	SystemPromptPath string
	HistoryDepth     int
	RequestTimeout   time.Duration
}

type AppConfig struct {
	Token             string
	RoleChannelID     string
	AdminChannelID    string
	LeetcodeChannelID string
	RoleMessageHeader string
	Location          *time.Location
	LLM               LLMConfig
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
		LLM:               loadLLMConfig(),
	}, nil
}

func loadLLMConfig() LLMConfig {
	endpoint := os.Getenv("LLM_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultLLMEndpoint
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = defaultLLMModel
	}

	maxTokens := defaultLLMMaxTokens
	if v, err := strconv.Atoi(os.Getenv("LLM_MAX_TOKENS")); err == nil && v > 0 {
		maxTokens = v
	}

	temperature := defaultLLMTemperature
	if v, err := strconv.ParseFloat(os.Getenv("LLM_TEMPERATURE"), 64); err == nil && v >= 0 {
		temperature = v
	}

	historyDepth := defaultLLMHistory
	if v, err := strconv.Atoi(os.Getenv("LLM_HISTORY_DEPTH")); err == nil && v >= 0 {
		historyDepth = v
	}

	timeoutSec := defaultLLMTimeoutSec
	if v, err := strconv.Atoi(os.Getenv("LLM_TIMEOUT_SECONDS")); err == nil && v > 0 {
		timeoutSec = v
	}

	return LLMConfig{
		Endpoint:         endpoint,
		Model:            model,
		MaxTokens:        maxTokens,
		Temperature:      temperature,
		SystemPromptPath: os.Getenv("LLM_SYSTEM_PROMPT_PATH"),
		HistoryDepth:     historyDepth,
		RequestTimeout:   time.Duration(timeoutSec) * time.Second,
	}
}
