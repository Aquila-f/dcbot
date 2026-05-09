package handlers

import (
	"dcbot/store"
	"dcbot/util"
	"log"

	"github.com/bwmarrin/discordgo"
)

func ReactionAdd(_ *discordgo.Session, st *store.RoleStore) func(*discordgo.Session, *discordgo.MessageReactionAdd) {
	return func(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
		emoji := util.EmojiFromReaction(r.Emoji)
		log.Printf("[reaction] add: msgID=%s storedID=%s emoji=%s userID=%s guildID=%s",
			r.MessageID, st.MessageID(), emoji, r.UserID, r.GuildID)

		if r.MessageID != st.MessageID() {
			log.Printf("[reaction] ignored: message ID mismatch")
			return
		}
		if r.UserID == s.State.User.ID {
			return
		}
		roleID, ok := st.RoleForEmoji(emoji)
		if !ok {
			log.Printf("[reaction] unknown emoji %s, removing", emoji)
			if err := s.MessageReactionRemove(r.ChannelID, r.MessageID, util.EmojiForAPI(emoji), r.UserID); err != nil {
				log.Printf("failed to remove unknown reaction %s from %s: %v", emoji, r.UserID, err)
			}
			return
		}

		log.Printf("[reaction] adding role %s to user %s", roleID, r.UserID)
		if err := s.GuildMemberRoleAdd(r.GuildID, r.UserID, roleID); err != nil {
			log.Printf("failed to add role %s to %s: %v", roleID, r.UserID, err)
		}
	}
}

func ReactionRemove(_ *discordgo.Session, st *store.RoleStore) func(*discordgo.Session, *discordgo.MessageReactionRemove) {
	return func(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
		if r.MessageID != st.MessageID() {
			return
		}
		if r.UserID == s.State.User.ID {
			return
		}

		emoji := util.EmojiFromReaction(r.Emoji)
		roleID, ok := st.RoleForEmoji(emoji)
		if !ok {
			return
		}

		if err := s.GuildMemberRoleRemove(r.GuildID, r.UserID, roleID); err != nil {
			log.Printf("failed to remove role %s from %s: %v", roleID, r.UserID, err)
		}
	}
}
