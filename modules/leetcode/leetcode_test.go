package leetcode

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleResponse = `{
  "data": {
    "activeDailyCodingChallengeQuestion": {
      "date": "2026-05-13",
      "link": "/problems/two-sum/",
      "question": {
        "difficulty": "Easy",
        "frontendQuestionId": "1",
        "title": "Two Sum",
        "titleSlug": "two-sum",
        "topicTags": [{"name": "Array"}, {"name": "Hash Table"}]
      }
    }
  }
}`

const sampleRatings = `[
  {"ID": 1,    "Rating": 1234.5},
  {"ID": 9999, "Rating": 2500.0}
]`

func newMockServer(t *testing.T, status int, body string, capture *http.Request) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			*capture = *r.Clone(r.Context())
			b, _ := io.ReadAll(r.Body)
			capture.Body = io.NopCloser(strings.NewReader(string(b)))
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

func TestBuild_Success(t *testing.T) {
	var seen http.Request
	daily := newMockServer(t, 200, sampleResponse, &seen)
	defer daily.Close()
	rating := newMockServer(t, 200, sampleRatings, nil)
	defer rating.Close()

	task := &LeetcodeDaily{ChannelID: "chan-1", Endpoint: daily.URL, RatingEndpoint: rating.URL}
	payload, err := task.Build(context.Background())
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if payload.ChannelID != "chan-1" {
		t.Errorf("ChannelID = %q, want chan-1", payload.ChannelID)
	}
	if len(payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(payload.Embeds))
	}
	e := payload.Embeds[0]
	if e.Title != "1. Two Sum" {
		t.Errorf("Title = %q, want %q", e.Title, "1. Two Sum")
	}
	if e.URL != "https://leetcode.com/problems/two-sum/" {
		t.Errorf("URL = %q", e.URL)
	}
	if e.Color != colorEasy {
		t.Errorf("Color = %#x, want %#x", e.Color, colorEasy)
	}
	if len(e.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(e.Fields))
	}
	if e.Fields[0].Value != "Easy" {
		t.Errorf("Difficulty field = %q", e.Fields[0].Value)
	}
	if e.Fields[1].Value != "1235" {
		t.Errorf("Rating field = %q, want 1235 (rounded from 1234.5)", e.Fields[1].Value)
	}
	if e.Fields[2].Value != "||Array, Hash Table||" {
		t.Errorf("Tags field = %q", e.Fields[2].Value)
	}
	if e.Footer == nil || !strings.Contains(e.Footer.Text, "2026-05-13") {
		t.Errorf("Footer = %+v", e.Footer)
	}

	// Verify request shape.
	if seen.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", seen.Method)
	}
	if seen.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", seen.Header.Get("Content-Type"))
	}
	var sentBody map[string]string
	body, _ := io.ReadAll(seen.Body)
	if err := json.Unmarshal(body, &sentBody); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if !strings.Contains(sentBody["query"], "activeDailyCodingChallengeQuestion") {
		t.Errorf("query missing expected field: %q", sentBody["query"])
	}
}

func TestBuild_RatingNotFound(t *testing.T) {
	daily := newMockServer(t, 200, sampleResponse, nil)
	defer daily.Close()
	rating := newMockServer(t, 200, `[{"ID": 9999, "Rating": 2500}]`, nil)
	defer rating.Close()

	task := &LeetcodeDaily{ChannelID: "c", Endpoint: daily.URL, RatingEndpoint: rating.URL}
	payload, err := task.Build(context.Background())
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if got := payload.Embeds[0].Fields[1].Value; got != missingValue {
		t.Errorf("Rating field = %q, want %q", got, missingValue)
	}
}

func TestBuild_RatingEndpointFails(t *testing.T) {
	daily := newMockServer(t, 200, sampleResponse, nil)
	defer daily.Close()
	rating := newMockServer(t, 500, "boom", nil)
	defer rating.Close()

	task := &LeetcodeDaily{ChannelID: "c", Endpoint: daily.URL, RatingEndpoint: rating.URL}
	payload, err := task.Build(context.Background())
	if err != nil {
		t.Fatalf("rating fetch should be best-effort, got error: %v", err)
	}
	if got := payload.Embeds[0].Fields[1].Value; got != missingValue {
		t.Errorf("Rating field = %q, want %q", got, missingValue)
	}
}

func TestBuild_HTTPError(t *testing.T) {
	srv := newMockServer(t, 500, "boom", nil)
	defer srv.Close()

	task := &LeetcodeDaily{ChannelID: "c", Endpoint: srv.URL}
	if _, err := task.Build(context.Background()); err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestBuild_MalformedJSON(t *testing.T) {
	srv := newMockServer(t, 200, "not json", nil)
	defer srv.Close()

	task := &LeetcodeDaily{ChannelID: "c", Endpoint: srv.URL}
	if _, err := task.Build(context.Background()); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestBuild_EmptyQuestion(t *testing.T) {
	srv := newMockServer(t, 200, `{"data":{"activeDailyCodingChallengeQuestion":{}}}`, nil)
	defer srv.Close()

	task := &LeetcodeDaily{ChannelID: "c", Endpoint: srv.URL}
	if _, err := task.Build(context.Background()); err == nil {
		t.Fatal("expected error for empty question")
	}
}

func TestDifficultyColor(t *testing.T) {
	cases := map[string]int{
		"Easy":    colorEasy,
		"Medium":  colorMedium,
		"Hard":    colorHard,
		"":        colorUnknown,
		"Unknown": colorUnknown,
	}
	for in, want := range cases {
		if got := difficultyColor(in); got != want {
			t.Errorf("difficultyColor(%q) = %#x, want %#x", in, got, want)
		}
	}
}
