package roles

import (
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/bwmarrin/discordgo"
)

func (m *Manager) sync(ctx context.Context, s *discordgo.Session) error {
	msgID, err := m.ensureRoleMessage(s)
	if err != nil {
		return fmt.Errorf("ensureRoleMessage: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.syncReactions(s, msgID); err != nil {
		log.Printf("syncReactions warning: %v", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.syncMemberRoles(s, msgID); err != nil {
		log.Printf("syncMemberRoles warning: %v", err)
	}

	return nil
}

func (m *Manager) syncReactions(s *discordgo.Session, msgID string) error {
	mappings := m.store.Roles()

	msg, err := s.ChannelMessage(m.roleChannelID, msgID)
	if err != nil {
		return err
	}

	existingReactions := make(map[string]bool)
	for _, r := range msg.Reactions {
		existingReactions[EmojiFromReaction(*r.Emoji)] = true
	}

	for emoji := range mappings {
		if !existingReactions[emoji] {
			if err := s.MessageReactionAdd(m.roleChannelID, msgID, EmojiForAPI(emoji)); err != nil {
				log.Printf("failed to add reaction %s: %v", emoji, err)
			}
		}
	}

	for emoji := range existingReactions {
		if _, mapped := mappings[emoji]; !mapped {
			if err := s.MessageReactionRemove(m.roleChannelID, msgID, EmojiForAPI(emoji), s.State.User.ID); err != nil {
				log.Printf("failed to remove stale reaction %s: %v", emoji, err)
			}
		}
	}

	return nil
}

func (m *Manager) syncMemberRoles(s *discordgo.Session, msgID string) error {
	mappings := m.store.Roles()
	if len(mappings) == 0 {
		return nil
	}

	ch, err := s.Channel(m.roleChannelID)
	if err != nil {
		return fmt.Errorf("failed to resolve guild from role channel: %w", err)
	}
	guildID := ch.GuildID

	for emoji, roleID := range mappings {
		after := ""
		for {
			users, err := s.MessageReactions(m.roleChannelID, msgID, EmojiForAPI(emoji), 100, "", after)
			if err != nil {
				log.Printf("failed to fetch reactions for %s: %v", emoji, err)
				break
			}

			for _, u := range users {
				if u.ID == s.State.User.ID {
					continue
				}
				member, err := s.GuildMember(guildID, u.ID)
				if err != nil {
					continue
				}
				if !hasMemberRole(member, roleID) {
					if err := s.GuildMemberRoleAdd(guildID, u.ID, roleID); err != nil {
						log.Printf("sync: failed to add role to %s: %v", u.ID, err)
					}
				}
			}

			if len(users) < 100 {
				break
			}
			after = users[len(users)-1].ID
		}
	}

	return nil
}

func hasMemberRole(member *discordgo.Member, roleID string) bool {
	return slices.Contains(member.Roles, roleID)
}
