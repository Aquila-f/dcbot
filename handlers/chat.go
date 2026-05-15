package handlers

import (
	"context"
	"dcbot/llm"
	"log"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	fallbackReply       = "X_X"
	discordMessageLimit = 2000
)

type ChatHandler struct {
	llm     *llm.Client
	persona *llm.Persona
	depth   int
}

func NewChatHandler(client *llm.Client, persona *llm.Persona, depth int) *ChatHandler {
	if depth <= 0 {
		depth = 5
	}
	return &ChatHandler{llm: client, persona: persona, depth: depth}
}

func (h *ChatHandler) Handle(s *discordgo.Session, m *discordgo.MessageCreate) {
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

func (h *ChatHandler) triggered(s *discordgo.Session, m *discordgo.MessageCreate, botID string) bool {
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
func (h *ChatHandler) fetchHistory(s *discordgo.Session, m *discordgo.MessageCreate) []*discordgo.Message {
	if h.depth == 0 {
		return nil
	}
	if m.MessageReference != nil && m.MessageReference.MessageID != "" {
		return h.fetchReplyChain(s, m.MessageReference, h.depth)
	}
	return h.fetchTimeWindow(s, m.ChannelID, m.ID, h.depth)
}

func (h *ChatHandler) fetchReplyChain(s *discordgo.Session, ref *discordgo.MessageReference, depth int) []*discordgo.Message {
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
	// chain is newest-first; reverse to chronological.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func (h *ChatHandler) fetchTimeWindow(s *discordgo.Session, channelID, beforeID string, depth int) []*discordgo.Message {
	msgs, err := s.ChannelMessages(channelID, depth, beforeID, "", "")
	if err != nil {
		log.Printf("[chat] fetch history: %v", err)
		return nil
	}
	// Discord returns newest-first; reverse to chronological.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs
}

func (h *ChatHandler) buildMessages(history []*discordgo.Message, current *discordgo.MessageCreate, botID string) []llm.Message {
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
