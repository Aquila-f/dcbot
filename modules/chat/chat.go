package chat

import (
	"context"
	"dcbot/config"
	"dcbot/domain"
	"dcbot/llm"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	fallbackReply       = "X_X"
	discordMessageLimit = 2000
)

var (
	_ domain.Module          = (*Handler)(nil)
	_ domain.EventSubscriber = (*Handler)(nil)
)

type Handler struct {
	llm     *llm.Client
	persona *llm.Persona
	depth   int
}

func (h *Handler) Name() string { return "chat" }

func (h *Handler) RegisterHandlers(s *discordgo.Session) {
	s.AddHandler(h.Handle)
}

func New(cfg config.LLMConfig, loc *time.Location) (*Handler, error) {
	persona, err := llm.NewPersona(cfg.SystemPromptPath, loc)
	if err != nil {
		return nil, fmt.Errorf("load persona: %w", err)
	}
	depth := cfg.HistoryDepth
	if depth <= 0 {
		depth = 5
	}
	return &Handler{llm: llm.New(cfg), persona: persona, depth: depth}, nil
}

func (h *Handler) Handle(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil {
		return
	}
	botID := s.State.User.ID
	if m.Author.ID == botID {
		return
	}
	if m.Author.Bot {
		return
	}
	if m.Type != discordgo.MessageTypeDefault && m.Type != discordgo.MessageTypeReply {
		return
	}

	if !h.triggered(s, m, botID) {
		return
	}

	history := h.fetchHistory(s, m)
	msgs := h.buildMessages(history, m, botID)

	ctx, cancel := context.WithTimeout(context.Background(), h.llm.Timeout())
	defer cancel()

	reply, err := h.llm.Complete(ctx, msgs)
	if err != nil {
		log.Printf("[chat] llm error: %v", err)
		reply = fallbackReply
	}
	if reply == "" {
		reply = fallbackReply
	}
	if len(reply) > discordMessageLimit {
		reply = reply[:discordMessageLimit]
	}

	_, err = s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content:   reply,
		Reference: m.Reference(),
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			RepliedUser: true,
		},
	})
	if err != nil {
		log.Printf("[chat] failed to send reply: %v", err)
	}
}

func (h *Handler) triggered(s *discordgo.Session, m *discordgo.MessageCreate, botID string) bool {
	for _, u := range m.Mentions {
		if u.ID == botID {
			return true
		}
	}
	if m.MessageReference != nil && m.MessageReference.MessageID != "" {
		ref, err := s.ChannelMessage(m.MessageReference.ChannelID, m.MessageReference.MessageID)
		if err == nil && ref.Author != nil && ref.Author.ID == botID {
			return true
		}
	}
	return false
}

// fetchHistory returns messages in chronological order (oldest first), excluding m itself.
// Sources, combined and deduped by message ID:
//  1. Reply chain walked up from m.MessageReference (up to depth)
//  2. Time-window fill from m.ChannelID before m.ID (until depth total)
//  3. Thread parent message (always-on, exempt from depth budget)
func (h *Handler) fetchHistory(s *discordgo.Session, m *discordgo.MessageCreate) []*discordgo.Message {
	if h.depth <= 0 {
		return nil
	}

	seen := map[string]bool{m.ID: true}
	collected := make([]*discordgo.Message, 0, h.depth+1)

	add := func(msg *discordgo.Message) {
		if msg == nil || msg.ID == "" || seen[msg.ID] {
			return
		}
		seen[msg.ID] = true
		collected = append(collected, msg)
	}

	if m.MessageReference != nil && m.MessageReference.MessageID != "" {
		for _, msg := range h.walkReplyChain(s, m.MessageReference, h.depth) {
			add(msg)
			if len(collected) >= h.depth {
				break
			}
		}
	}

	if len(collected) < h.depth {
		need := h.depth - len(collected)
		// over-fetch to absorb dedup losses; toLLMMessage will filter again later
		for _, msg := range h.fetchTimeWindow(s, m.ChannelID, m.ID, need+5) {
			add(msg)
			if len(collected) >= h.depth {
				break
			}
		}
	}

	if parent := h.fetchThreadParent(s, m.ChannelID); parent != nil {
		add(parent)
	}

	sort.SliceStable(collected, func(i, j int) bool {
		return collected[i].Timestamp.Before(collected[j].Timestamp)
	})
	return collected
}

// walkReplyChain returns messages in newest-first order (caller sorts globally).
func (h *Handler) walkReplyChain(s *discordgo.Session, ref *discordgo.MessageReference, depth int) []*discordgo.Message {
	chain := make([]*discordgo.Message, 0, depth)
	cur := ref
	for i := 0; i < depth && cur != nil && cur.MessageID != ""; i++ {
		msg, err := s.ChannelMessage(cur.ChannelID, cur.MessageID)
		if err != nil {
			log.Printf("[chat] fetch reply parent %s: %v", cur.MessageID, err)
			break
		}
		chain = append(chain, msg)
		cur = msg.MessageReference
	}
	return chain
}

// fetchTimeWindow returns messages in newest-first order (caller sorts globally).
func (h *Handler) fetchTimeWindow(s *discordgo.Session, channelID, beforeID string, limit int) []*discordgo.Message {
	msgs, err := s.ChannelMessages(channelID, limit, beforeID, "", "")
	if err != nil {
		log.Printf("[chat] fetch history: %v", err)
		return nil
	}
	return msgs
}

// fetchThreadParent returns the starter message that a thread was opened from,
// or nil if the channel isn't a thread or has no accessible starter message.
func (h *Handler) fetchThreadParent(s *discordgo.Session, channelID string) *discordgo.Message {
	ch, err := s.Channel(channelID)
	if err != nil || ch == nil || !ch.IsThread() || ch.ParentID == "" {
		return nil
	}
	// For threads created from a message, thread ID == starter message ID, living in the parent channel.
	parent, err := s.ChannelMessage(ch.ParentID, channelID)
	if err != nil {
		return nil
	}
	return parent
}

func (h *Handler) buildMessages(history []*discordgo.Message, current *discordgo.MessageCreate, botID string) []llm.Message {
	out := h.persona.Messages()
	for _, msg := range history {
		if conv, ok := toLLMMessage(msg, botID); ok {
			out = append(out, conv)
		}
	}
	// current message is always the user's trigger
	out = append(out, llm.Message{
		Role:    "user",
		Content: formatUserContent(current.Author.Username, current.Content, botID),
	})
	return out
}

func toLLMMessage(msg *discordgo.Message, botID string) (llm.Message, bool) {
	if msg == nil || msg.Author == nil {
		return llm.Message{}, false
	}
	if msg.Type != discordgo.MessageTypeDefault && msg.Type != discordgo.MessageTypeReply {
		return llm.Message{}, false
	}
	if msg.Interaction != nil {
		return llm.Message{}, false
	}
	if msg.Author.ID == botID {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			return llm.Message{}, false
		}
		return llm.Message{Role: "assistant", Content: content}, true
	}
	if msg.Author.Bot {
		return llm.Message{}, false
	}
	content := formatUserContent(msg.Author.Username, msg.Content, botID)
	if strings.TrimSpace(content) == "" {
		return llm.Message{}, false
	}
	return llm.Message{Role: "user", Content: content}, true
}

var mentionAnyRe = regexp.MustCompile(`<@!?(\d+)>`)

func formatUserContent(username, content, botID string) string {
	stripped := mentionAnyRe.ReplaceAllStringFunc(content, func(match string) string {
		m := mentionAnyRe.FindStringSubmatch(match)
		if len(m) == 2 && m[1] == botID {
			return ""
		}
		return match
	})
	stripped = strings.TrimSpace(stripped)
	if username == "" {
		return stripped
	}
	return username + ": " + stripped
}
