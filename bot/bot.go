package bot

import (
	"dcbot/config"
	"dcbot/handlers"
	"dcbot/scheduler"
	"dcbot/store"
	"dcbot/util"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session        *discordgo.Session
	cfg            *config.AppConfig
	store          *store.RoleStore
	scheduler      *scheduler.Scheduler
	registeredCmds []*discordgo.ApplicationCommand
}

func New(cfg *config.AppConfig, st *store.RoleStore) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMessageReactions |
		discordgo.IntentsGuildMembers

	return &Bot{
		session:   session,
		cfg:       cfg,
		store:     st,
		scheduler: scheduler.New(session, cfg.AdminChannelID, cfg.Location),
	}, nil
}

func (b *Bot) Start() error {
	cmdHandler := handlers.NewCommandHandler(b.store, b.cfg.RoleChannelID, b.cfg.AdminChannelID, b.updateRoleMessage)

	b.session.AddHandler(handlers.ReactionAdd(b.session, b.store))
	b.session.AddHandler(handlers.ReactionRemove(b.session, b.store))
	b.session.AddHandler(cmdHandler.Handle)

	errCh := make(chan error, 1)
	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as %s", s.State.User.Username)
		b.registerCommands(s, r)
		if err := b.syncOnStartup(s); err != nil {
			errCh <- fmt.Errorf("startup sync error: %v", err)
			return
		}
		errCh <- nil
	})

	if err := b.session.Open(); err != nil {
		return err
	}

	if err := <-errCh; err != nil {
		b.Stop()
		return err
	}

	b.scheduler.Start()
	return nil
}

func (b *Bot) Stop() {
	b.scheduler.Stop()
	for _, cmd := range b.registeredCmds {
		guilds := b.session.State.Guilds
		for _, g := range guilds {
			if err := b.session.ApplicationCommandDelete(b.session.State.User.ID, g.ID, cmd.ID); err != nil {
				log.Printf("failed to delete command %s: %v", cmd.Name, err)
			}
		}
	}
	b.session.Close()
}

func (b *Bot) registerCommands(s *discordgo.Session, r *discordgo.Ready) {
	for _, guild := range r.Guilds {
		for _, cmd := range handlers.Commands {
			registered, err := s.ApplicationCommandCreate(s.State.User.ID, guild.ID, cmd)
			if err != nil {
				log.Printf("failed to register command %s in guild %s: %v", cmd.Name, guild.ID, err)
				continue
			}
			b.registeredCmds = append(b.registeredCmds, registered)
		}
	}
}

func (b *Bot) syncOnStartup(s *discordgo.Session) error {
	msgID, err := b.ensureRoleMessage(s)
	if err != nil {
		return fmt.Errorf("ensureRoleMessage: %w", err)
	}

	if err := b.syncReactions(s, msgID); err != nil {
		log.Printf("syncReactions warning: %v", err)
	}

	if err := b.syncMemberRoles(s, msgID); err != nil {
		log.Printf("syncMemberRoles warning: %v", err)
	}

	return nil
}

func (b *Bot) ensureRoleMessage(s *discordgo.Session) (string, error) {
	// Try saved ID first as a fast path, then fall back to scanning the channel.
	msgID, mappings, err := func() (string, map[string]string, error) {
		if savedID := b.store.MessageID(); savedID != "" {
			msg, err := s.ChannelMessage(b.cfg.RoleChannelID, savedID)
			if err == nil {
				return msg.ID, parseRoleMessage(msg.Content), nil
			}
			log.Printf("saved message not found (%v), scanning channel", err)
		}
		return b.findExistingRoleMessage(s)
	}()

	if err != nil {
		return "", fmt.Errorf("failed to locate role message: %w", err)
	}

	if msgID != "" {
		// Discord is source of truth — sync store from message content.
		if err := b.store.SetMappings(mappings); err != nil {
			log.Printf("failed to sync mappings from Discord: %v", err)
		}
		if err := b.store.SetMessageID(msgID); err != nil {
			return "", fmt.Errorf("failed to save message_id: %w", err)
		}
		log.Printf("loaded %d role mapping(s) from Discord message %s", len(mappings), msgID)
		return msgID, nil
	}

	// No message on Discord — create one from whatever is in the store.
	content := buildRoleMessageContent(b.cfg.RoleMessageHeader, b.store.Roles())
	msg, err := s.ChannelMessageSend(b.cfg.RoleChannelID, content)
	if err != nil {
		return "", fmt.Errorf("failed to send role message: %w", err)
	}
	if err := b.store.SetMessageID(msg.ID); err != nil {
		return "", fmt.Errorf("failed to save message_id: %w", err)
	}
	return msg.ID, nil
}

func (b *Bot) findExistingRoleMessage(s *discordgo.Session) (string, map[string]string, error) {
	msgs, err := s.ChannelMessages(b.cfg.RoleChannelID, 1, "", "", "")
	if err != nil {
		return "", nil, err
	}
	if len(msgs) == 1 && msgs[0].Author.ID == s.State.User.ID {
		return msgs[0].ID, parseRoleMessage(msgs[0].Content), nil
	}
	return "", nil, nil
}

var roleLineRe = regexp.MustCompile(`^(.+) → <@&(\d+)>$`)

func parseRoleMessage(content string) map[string]string {
	parts := strings.SplitN(content, "\n---\n", 2)
	if len(parts) < 2 {
		return nil
	}
	roles := make(map[string]string)
	for _, line := range strings.Split(strings.TrimRight(parts[1], "\n"), "\n") {
		if m := roleLineRe.FindStringSubmatch(line); m != nil {
			roles[m[1]] = m[2]
		}
	}
	return roles
}

func (b *Bot) updateRoleMessage() error {
	msgID := b.store.MessageID()
	if msgID == "" {
		return nil
	}
	content := buildRoleMessageContent(b.cfg.RoleMessageHeader, b.store.Roles())
	_, err := b.session.ChannelMessageEdit(b.cfg.RoleChannelID, msgID, content)
	return err
}

func (b *Bot) syncReactions(s *discordgo.Session, msgID string) error {
	roles := b.store.Roles()

	msg, err := s.ChannelMessage(b.cfg.RoleChannelID, msgID)
	if err != nil {
		return err
	}

	existingReactions := make(map[string]bool)
	for _, r := range msg.Reactions {
		existingReactions[util.EmojiFromReaction(*r.Emoji)] = true
	}

	for emoji := range roles {
		if !existingReactions[emoji] {
			if err := s.MessageReactionAdd(b.cfg.RoleChannelID, msgID, util.EmojiForAPI(emoji)); err != nil {
				log.Printf("failed to add reaction %s: %v", emoji, err)
			}
		}
	}

	for emoji := range existingReactions {
		if _, mapped := roles[emoji]; !mapped {
			if err := s.MessageReactionRemove(b.cfg.RoleChannelID, msgID, util.EmojiForAPI(emoji), s.State.User.ID); err != nil {
				log.Printf("failed to remove stale reaction %s: %v", emoji, err)
			}
		}
	}

	return nil
}

func (b *Bot) syncMemberRoles(s *discordgo.Session, msgID string) error {
	roles := b.store.Roles()
	if len(roles) == 0 {
		return nil
	}

	ch, err := s.Channel(b.cfg.RoleChannelID)
	if err != nil {
		return fmt.Errorf("failed to resolve guild from role channel: %w", err)
	}
	guildID := ch.GuildID

	for emoji, roleID := range roles {
		after := ""
		for {
			users, err := s.MessageReactions(b.cfg.RoleChannelID, msgID, util.EmojiForAPI(emoji), 100, "", after)
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
