package roles

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

func (m *Manager) handleReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if r.MessageID != m.store.MessageID() {
		log.Printf("[reaction] ignored: message ID mismatch")
		return
	}
	if r.UserID == s.State.User.ID {
		return
	}

	emoji := EmojiFromReaction(r.Emoji)
	log.Printf("[reaction] add: msgID=%s storedID=%s emoji=%s userID=%s guildID=%s",
		r.MessageID, m.store.MessageID(), emoji, r.UserID, r.GuildID)

	roleID, ok := m.store.RoleForEmoji(emoji)
	if !ok {
		log.Printf("[reaction] unknown emoji %s, removing", emoji)
		if err := s.MessageReactionRemove(r.ChannelID, r.MessageID, EmojiForAPI(emoji), r.UserID); err != nil {
			log.Printf("failed to remove unknown reaction %s from %s: %v", emoji, r.UserID, err)
		}
		return
	}

	log.Printf("[reaction] adding role %s to user %s", roleID, r.UserID)
	if err := s.GuildMemberRoleAdd(r.GuildID, r.UserID, roleID); err != nil {
		log.Printf("failed to add role %s to %s: %v", roleID, r.UserID, err)
	}
}

func (m *Manager) handleReactionRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
	if r.MessageID != m.store.MessageID() {
		return
	}
	if r.UserID == s.State.User.ID {
		return
	}

	emoji := EmojiFromReaction(r.Emoji)
	roleID, ok := m.store.RoleForEmoji(emoji)
	if !ok {
		return
	}

	if err := s.GuildMemberRoleRemove(r.GuildID, r.UserID, roleID); err != nil {
		log.Printf("failed to remove role %s from %s: %v", roleID, r.UserID, err)
	}
}
