package roles

import (
	"dcbot/domain"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var customEmojiRe = regexp.MustCompile(`^<a?:\w+:\d+>$`)

func isValidEmoji(s string) bool {
	if customEmojiRe.MatchString(s) {
		return true
	}
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}

var manageRolesPerm = int64(discordgo.PermissionManageRoles)

var addRoleCmd = &discordgo.ApplicationCommand{
	Name:                     "addrole",
	Description:              "Map an emoji to a role for reaction-based role assignment",
	DefaultMemberPermissions: &manageRolesPerm,
	Options: []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "emoji",
			Description: "The emoji to react with",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionRole,
			Name:        "role",
			Description: "The role to assign",
			Required:    true,
		},
	},
}

var removeRoleCmd = &discordgo.ApplicationCommand{
	Name:                     "removerole",
	Description:              "Remove an emoji-to-role mapping",
	DefaultMemberPermissions: &manageRolesPerm,
	Options: []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "emoji",
			Description: "The emoji mapping to remove",
			Required:    true,
		},
	},
}

// requireAdmin wraps a handler with the role-admin channel and permission checks.
func (m *Manager) requireAdmin(h domain.InteractionHandler) domain.InteractionHandler {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.ChannelID != m.adminChannelID {
			reply(s, i, "This command can only be used in the admin channel.")
			return
		}
		if !hasManageRoles(i.Member) {
			reply(s, i, "You need the Manage Roles permission to use this command.")
			return
		}
		h(s, i)
	}
}

func (m *Manager) handleAddRole(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	emoji := strings.TrimSpace(options[0].StringValue())
	role := options[1].RoleValue(s, i.GuildID)

	if !isValidEmoji(emoji) {
		reply(s, i, "Invalid emoji. Please use a standard emoji (🎮) or a server custom emoji (<:name:id>).")
		return
	}

	if err := checkBotHierarchy(s, i.GuildID, role.ID); err != nil {
		reply(s, i, err.Error())
		return
	}

	if err := m.store.Add(emoji, role.ID); err != nil {
		reply(s, i, fmt.Sprintf("Error: %s", err.Error()))
		return
	}

	if err := m.updateRoleMessage(s); err != nil {
		_ = m.store.Remove(emoji)
		reply(s, i, fmt.Sprintf("Error: failed to update role message: %s", err.Error()))
		return
	}

	msgID := m.store.MessageID()
	if msgID != "" {
		if err := s.MessageReactionAdd(m.roleChannelID, msgID, EmojiForAPI(emoji)); err != nil {
			log.Printf("failed to add reaction %s to message: %v", emoji, err)
		}
	}

	reply(s, i, fmt.Sprintf("✅ Added: %s → <@&%s>", emoji, role.ID))
}

func (m *Manager) handleRemoveRole(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	emoji := strings.TrimSpace(options[0].StringValue())

	roleID, _ := m.store.RoleForEmoji(emoji)

	if err := m.store.Remove(emoji); err != nil {
		reply(s, i, fmt.Sprintf("Error: %s", err.Error()))
		return
	}

	if err := m.updateRoleMessage(s); err != nil {
		_ = m.store.Add(emoji, roleID)
		reply(s, i, fmt.Sprintf("Error: failed to update role message: %s", err.Error()))
		return
	}

	msgID := m.store.MessageID()
	if msgID != "" {
		if err := s.MessageReactionsRemoveEmoji(m.roleChannelID, msgID, EmojiForAPI(emoji)); err != nil {
			log.Printf("failed to remove all reactions for %s: %v", emoji, err)
		}
	}

	reply(s, i, fmt.Sprintf("✅ Removed: %s", emoji))
}

func hasManageRoles(member *discordgo.Member) bool {
	if member == nil {
		return false
	}
	return member.Permissions&discordgo.PermissionManageRoles != 0
}

func checkBotHierarchy(s *discordgo.Session, guildID, targetRoleID string) error {
	botMember, err := s.GuildMember(guildID, s.State.User.ID)
	if err != nil {
		return fmt.Errorf("could not verify bot permissions")
	}

	guild, err := s.Guild(guildID)
	if err != nil {
		return fmt.Errorf("could not fetch guild info")
	}

	rolePos := func(roleID string) int {
		for _, r := range guild.Roles {
			if r.ID == roleID {
				return r.Position
			}
		}
		return -1
	}

	botHighest := 0
	for _, rID := range botMember.Roles {
		if pos := rolePos(rID); pos > botHighest {
			botHighest = pos
		}
	}

	if rolePos(targetRoleID) >= botHighest {
		return fmt.Errorf("bot's role is not high enough to assign this role")
	}
	return nil
}

func reply(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	}); err != nil {
		log.Printf("failed to send reply: %v", err)
	}
}
