package handlers

import (
	"dcbot/store"
	"dcbot/util"
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

var Commands = []*discordgo.ApplicationCommand{
	{
		Name:        "addrole",
		Description: "Map an emoji to a role for reaction-based role assignment",
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
	},
	{
		Name:        "removerole",
		Description: "Remove an emoji-to-role mapping",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "emoji",
				Description: "The emoji mapping to remove",
				Required:    true,
			},
		},
	},
}

type CommandHandler struct {
	store          *store.RoleStore
	roleChannelID  string
	adminChannelID string
	updateMessage  func() error
}

func NewCommandHandler(st *store.RoleStore, roleChannelID, adminChannelID string, updateMessage func() error) *CommandHandler {
	return &CommandHandler{
		store:          st,
		roleChannelID:  roleChannelID,
		adminChannelID: adminChannelID,
		updateMessage:  updateMessage,
	}
}

func (h *CommandHandler) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	if i.ChannelID != h.adminChannelID {
		reply(s, i, "This command can only be used in the admin channel.")
		return
	}

	if !hasManageRoles(i.Member) {
		reply(s, i, "You need the Manage Roles permission to use this command.")
		return
	}

	switch i.ApplicationCommandData().Name {
	case "addrole":
		h.handleAddRole(s, i)
	case "removerole":
		h.handleRemoveRole(s, i)
	}
}

func (h *CommandHandler) handleAddRole(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	if err := h.store.Add(emoji, role.ID); err != nil {
		reply(s, i, fmt.Sprintf("Error: %s", err.Error()))
		return
	}

	if err := h.updateMessage(); err != nil {
		_ = h.store.Remove(emoji)
		reply(s, i, fmt.Sprintf("Error: failed to update role message: %s", err.Error()))
		return
	}

	msgID := h.store.MessageID()
	if msgID != "" {
		if err := s.MessageReactionAdd(h.roleChannelID, msgID, util.EmojiForAPI(emoji)); err != nil {
			log.Printf("failed to add reaction %s to message: %v", emoji, err)
		}
	}

	reply(s, i, fmt.Sprintf("✅ Added: %s → <@&%s>", emoji, role.ID))
}

func (h *CommandHandler) handleRemoveRole(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	emoji := strings.TrimSpace(options[0].StringValue())

	roleID, _ := h.store.RoleForEmoji(emoji)

	if err := h.store.Remove(emoji); err != nil {
		reply(s, i, fmt.Sprintf("Error: %s", err.Error()))
		return
	}

	if err := h.updateMessage(); err != nil {
		_ = h.store.Add(emoji, roleID)
		reply(s, i, fmt.Sprintf("Error: failed to update role message: %s", err.Error()))
		return
	}

	msgID := h.store.MessageID()
	if msgID != "" {
		if err := s.MessageReactionsRemoveEmoji(h.roleChannelID, msgID, util.EmojiForAPI(emoji)); err != nil {
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
