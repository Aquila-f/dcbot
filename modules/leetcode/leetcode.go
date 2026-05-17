package leetcode

import (
	"bytes"
	"context"
	"dcbot/domain"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	defaultLeetcodeEndpoint = "https://leetcode.com/graphql"
	defaultRatingEndpoint   = "https://cdn.jsdelivr.net/gh/zerotrac/leetcode_problem_rating@gh-pages/data.json"
	leetcodeBaseURL         = "https://leetcode.com"
	leetcodeQuery           = `query questionOfToday { activeDailyCodingChallengeQuestion { date link question { difficulty frontendQuestionId: questionFrontendId title titleSlug topicTags { name } } } }`
	missingValue            = "—"
	dailyCronSpec           = "0 9 * * *"
)

const (
	colorEasy    = 0x00B8A3
	colorMedium  = 0xFFC01E
	colorHard    = 0xEF4743
	colorUnknown = 0x95A5A6
)

var (
	_ domain.Module       = (*Module)(nil)
	_ domain.TaskProvider = (*Module)(nil)
)

// Module wires LeetcodeDaily into the bot as a scheduled task provider.
type Module struct {
	daily *LeetcodeDaily
}

// New returns a Module that schedules the daily LeetCode posting.
func New(channelID string) *Module {
	return &Module{daily: &LeetcodeDaily{ChannelID: channelID}}
}

func (m *Module) Name() string { return "leetcode" }

func (m *Module) Tasks() []domain.ScheduledTask {
	return []domain.ScheduledTask{{Spec: dailyCronSpec, Task: m.daily}}
}

type LeetcodeDaily struct {
	ChannelID      string
	Client         *http.Client
	Endpoint       string
	RatingEndpoint string
}

type dailyQuestion struct {
	Date       string
	Link       string
	ID         string
	Title      string
	Difficulty string
	Tags       []string
}

func (l *LeetcodeDaily) Name() string { return "leetcode-daily" }

func (l *LeetcodeDaily) Build(ctx context.Context) (domain.Payload, error) {
	client := l.Client
	if client == nil {
		client = http.DefaultClient
	}

	q, err := l.fetchDaily(ctx, client)
	if err != nil {
		return domain.Payload{}, err
	}

	rating := l.fetchRating(ctx, client, q.ID)

	return domain.Payload{
		ChannelID: l.ChannelID,
		Embeds:    []*discordgo.MessageEmbed{buildEmbed(q, rating)},
	}, nil
}

func (l *LeetcodeDaily) fetchDaily(ctx context.Context, client *http.Client) (dailyQuestion, error) {
	endpoint := l.Endpoint
	if endpoint == "" {
		endpoint = defaultLeetcodeEndpoint
	}

	body, err := json.Marshal(map[string]string{"query": leetcodeQuery})
	if err != nil {
		return dailyQuestion{}, fmt.Errorf("marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return dailyQuestion{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return dailyQuestion{}, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return dailyQuestion{}, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
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
		return dailyQuestion{}, fmt.Errorf("decode: %w", err)
	}

	raw := parsed.Data.ActiveDailyCodingChallengeQuestion
	if raw.Question.Title == "" {
		return dailyQuestion{}, fmt.Errorf("empty question in response")
	}

	tags := make([]string, len(raw.Question.TopicTags))
	for i, t := range raw.Question.TopicTags {
		tags[i] = t.Name
	}

	return dailyQuestion{
		Date:       raw.Date,
		Link:       raw.Link,
		ID:         raw.Question.ID,
		Title:      raw.Question.Title,
		Difficulty: raw.Question.Difficulty,
		Tags:       tags,
	}, nil
}

// fetchRating is best-effort: returns "" if rating cannot be retrieved or
// the question has no contest rating. Errors are logged but never returned.
func (l *LeetcodeDaily) fetchRating(ctx context.Context, client *http.Client, frontendID string) string {
	id, err := strconv.Atoi(frontendID)
	if err != nil {
		return ""
	}

	endpoint := l.RatingEndpoint
	if endpoint == "" {
		endpoint = defaultRatingEndpoint
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		log.Printf("[leetcode-daily] rating: build request: %v", err)
		return ""
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[leetcode-daily] rating: http: %v", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[leetcode-daily] rating: http %d", resp.StatusCode)
		return ""
	}

	var entries []struct {
		ID     int     `json:"ID"`
		Rating float64 `json:"Rating"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		log.Printf("[leetcode-daily] rating: decode: %v", err)
		return ""
	}

	for _, e := range entries {
		if e.ID == id {
			return strconv.Itoa(int(math.Round(e.Rating)))
		}
	}
	return ""
}

func buildEmbed(q dailyQuestion, rating string) *discordgo.MessageEmbed {
	tagField := missingValue
	if len(q.Tags) > 0 {
		tagField = "||" + strings.Join(q.Tags, ", ") + "||"
	}

	ratingField := rating
	if ratingField == "" {
		ratingField = missingValue
	}

	return &discordgo.MessageEmbed{
		Title: fmt.Sprintf("%s. %s", q.ID, q.Title),
		URL:   leetcodeBaseURL + q.Link,
		Color: difficultyColor(q.Difficulty),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Difficulty", Value: q.Difficulty, Inline: true},
			{Name: "Rating", Value: ratingField, Inline: true},
			{Name: "Tags", Value: tagField, Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "LeetCode Daily · " + q.Date,
		},
	}
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
