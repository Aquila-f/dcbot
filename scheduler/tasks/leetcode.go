package tasks

import (
	"bytes"
	"context"
	"dcbot/scheduler"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	defaultLeetcodeEndpoint = "https://leetcode.com/graphql"
	leetcodeBaseURL         = "https://leetcode.com"
	leetcodeQuery           = `query questionOfToday { activeDailyCodingChallengeQuestion { date link question { difficulty frontendQuestionId: questionFrontendId title titleSlug topicTags { name } } } }`
)

const (
	colorEasy    = 0x00B8A3
	colorMedium  = 0xFFC01E
	colorHard    = 0xEF4743
	colorUnknown = 0x95A5A6
)

type LeetcodeDaily struct {
	ChannelID string
	Client    *http.Client
	Endpoint  string
}

func (l *LeetcodeDaily) Name() string { return "leetcode-daily" }

func (l *LeetcodeDaily) Build(ctx context.Context) (scheduler.Payload, error) {
	endpoint := l.Endpoint
	if endpoint == "" {
		endpoint = defaultLeetcodeEndpoint
	}
	client := l.Client
	if client == nil {
		client = http.DefaultClient
	}

	body, err := json.Marshal(map[string]string{"query": leetcodeQuery})
	if err != nil {
		return scheduler.Payload{}, fmt.Errorf("marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return scheduler.Payload{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return scheduler.Payload{}, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return scheduler.Payload{}, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var parsed struct {
		Data struct {
			ActiveDailyCodingChallengeQuestion struct {
				Date     string `json:"date"`
				Link     string `json:"link"`
				Question struct {
					Difficulty string `json:"difficulty"`
					ID         string `json:"frontendQuestionId"`
					Title      string `json:"title"`
					TitleSlug  string `json:"titleSlug"`
					TopicTags  []struct {
						Name string `json:"name"`
					} `json:"topicTags"`
				} `json:"question"`
			} `json:"activeDailyCodingChallengeQuestion"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return scheduler.Payload{}, fmt.Errorf("decode: %w", err)
	}

	q := parsed.Data.ActiveDailyCodingChallengeQuestion
	if q.Question.Title == "" {
		return scheduler.Payload{}, fmt.Errorf("empty question in response")
	}

	tags := make([]string, len(q.Question.TopicTags))
	for i, t := range q.Question.TopicTags {
		tags[i] = t.Name
	}
	tagField := strings.Join(tags, ", ")
	if tagField == "" {
		tagField = "—"
	}

	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("%s. %s", q.Question.ID, q.Question.Title),
		URL:   leetcodeBaseURL + q.Link,
		Color: difficultyColor(q.Question.Difficulty),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Difficulty", Value: q.Question.Difficulty, Inline: true},
			{Name: "Tags", Value: tagField, Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "LeetCode Daily · " + q.Date,
		},
	}

	return scheduler.Payload{
		ChannelID: l.ChannelID,
		Embeds:    []*discordgo.MessageEmbed{embed},
	}, nil
}

func difficultyColor(d string) int {
	switch d {
	case "Easy":
		return colorEasy
	case "Medium":
		return colorMedium
	case "Hard":
		return colorHard
	default:
		return colorUnknown
	}
}
