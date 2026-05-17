package roles

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var roleLineRe = regexp.MustCompile(`^(.+) → <@&(\d+)>$`)

func (m *Manager) ensureRoleMessage(s *discordgo.Session) (string, error) {
	// Try saved ID first as a fast path, then fall back to scanning the channel.
	msgID, mappings, err := func() (string, map[string]string, error) {
		if savedID := m.store.MessageID(); savedID != "" {
			msg, err := s.ChannelMessage(m.roleChannelID, savedID)
			if err == nil {
				return msg.ID, parseRoleMessage(msg.Content), nil
			}
			log.Printf("saved message not found (%v), scanning channel", err)
		}
		return m.findExistingRoleMessage(s)
	}()

	if err != nil {
		return "", fmt.Errorf("failed to locate role message: %w", err)
	}

	if msgID != "" {
		// Discord is source of truth — sync store from message content.
		if err := m.store.SetMappings(mappings); err != nil {
			log.Printf("failed to sync mappings from Discord: %v", err)
		}
		if err := m.store.SetMessageID(msgID); err != nil {
			return "", fmt.Errorf("failed to save message_id: %w", err)
		}
		log.Printf("loaded %d role mapping(s) from Discord message %s", len(mappings), msgID)
		return msgID, nil
	}

	// No message on Discord — create one from whatever is in the store.
	content := buildRoleMessageContent(m.roleMsgHeader, m.store.Roles())
	msg, err := s.ChannelMessageSend(m.roleChannelID, content)
	if err != nil {
		return "", fmt.Errorf("failed to send role message: %w", err)
	}
	if err := m.store.SetMessageID(msg.ID); err != nil {
		return "", fmt.Errorf("failed to save message_id: %w", err)
	}
	return msg.ID, nil
}

func (m *Manager) findExistingRoleMessage(s *discordgo.Session) (string, map[string]string, error) {
	msgs, err := s.ChannelMessages(m.roleChannelID, 1, "", "", "")
	if err != nil {
		return "", nil, err
	}
	if len(msgs) == 1 && msgs[0].Author.ID == s.State.User.ID {
		return msgs[0].ID, parseRoleMessage(msgs[0].Content), nil
	}
	return "", nil, nil
}

func parseRoleMessage(content string) map[string]string {
	parts := strings.SplitN(content, "\n---\n", 2)
	if len(parts) < 2 {
		return nil
	}
	roles := make(map[string]string)
	for _, line := range strings.Split(strings.TrimRight(parts[1], "\n"), "\n") {
		if mm := roleLineRe.FindStringSubmatch(line); mm != nil {
			roles[mm[1]] = mm[2]
		}
	}
	return roles
}

func (m *Manager) updateRoleMessage(s *discordgo.Session) error {
	msgID := m.store.MessageID()
	if msgID == "" {
		return nil
	}
	content := buildRoleMessageContent(m.roleMsgHeader, m.store.Roles())
	_, err := s.ChannelMessageEdit(m.roleChannelID, msgID, content)
	return err
}

func buildRoleMessageContent(header string, roles map[string]string) string {
	if len(roles) == 0 {
		return header + "\n\nNo roles configured yet. An admin can use `/addrole` to add roles."
	}

	emojis := make([]string, 0, len(roles))
	for emoji := range roles {
		emojis = append(emojis, emoji)
	}
	sort.Strings(emojis)

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n---\n")
	for _, emoji := range emojis {
		fmt.Fprintf(&sb, "%s → <@&%s>\n", emoji, roles[emoji])
	}
	return sb.String()
}
