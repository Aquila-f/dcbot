package bot

import (
	"context"
	"dcbot/config"
	"dcbot/domain"
	"dcbot/modules/chat"
	"dcbot/modules/roles"
	"dcbot/scheduler"
	"dcbot/scheduler/tasks"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session        *discordgo.Session
	cfg            *config.AppConfig
	scheduler      *scheduler.Scheduler
	modules        []domain.Module
	commandTable   map[string]domain.InteractionHandler
	registeredCmds []*discordgo.ApplicationCommand
	rootCtx        context.Context
	cancel         context.CancelFunc
}

func New(cfg *config.AppConfig) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMessageReactions |
		discordgo.IntentsGuildMembers

	ctx, cancel := context.WithCancel(context.Background())
	return &Bot{
		session:      session,
		cfg:          cfg,
		scheduler:    scheduler.New(session, cfg.AdminChannelID, cfg.Location),
		commandTable: make(map[string]domain.InteractionHandler),
		rootCtx:      ctx,
		cancel:       cancel,
	}, nil
}

func (b *Bot) Start() error {
	rolesHandler, err := roles.New(b.cfg.Roles, b.cfg.AdminChannelID)
	if err != nil {
		return fmt.Errorf("init roles handler: %w", err)
	}

	chatHandler, err := chat.New(b.cfg.LLM, b.cfg.Location)
	if err != nil {
		return fmt.Errorf("init chat handler: %w", err)
	}
	b.modules = []domain.Module{rolesHandler, chatHandler}

	b.registerEventSubscribers()
	b.session.AddHandler(b.dispatchInteraction)

	readyErr := make(chan error, 1)
	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as %s", s.State.User.Username)
		b.registerCommandProviders(s, r)
		readyErr <- b.runReadyHooks(s, r)
	})

	if err := b.session.Open(); err != nil {
		return err
	}

	if err := <-readyErr; err != nil {
		b.Stop()
		return err
	}

	if b.cfg.LeetcodeChannelID != "" {
		if err := b.scheduler.Register("0 9 * * *", &tasks.LeetcodeDaily{ChannelID: b.cfg.LeetcodeChannelID}); err != nil {
			b.Stop()
			return fmt.Errorf("register leetcode-daily: %w", err)
		}
	}

	b.scheduler.Start()
	return nil
}

func (b *Bot) Stop() {
	b.cancel()
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

func (b *Bot) registerEventSubscribers() {
	for _, m := range b.modules {
		sub, ok := m.(domain.EventSubscriber)
		if !ok {
			continue
		}
		sub.RegisterHandlers(b.session)
		log.Printf("[%s] registered event handlers", m.Name())
	}
}

func (b *Bot) registerCommandProviders(s *discordgo.Session, r *discordgo.Ready) {
	for _, m := range b.modules {
		cp, ok := m.(domain.CommandProvider)
		if !ok {
			continue
		}
		for _, cmd := range cp.Commands() {
			b.commandTable[cmd.Definition.Name] = cmd.Handler
			log.Printf("[%s] /%s — %s (allowed: %s)",
				m.Name(),
				cmd.Definition.Name,
				cmd.Definition.Description,
				describePermissions(cmd.Definition.DefaultMemberPermissions),
			)
			for _, guild := range r.Guilds {
				registered, err := s.ApplicationCommandCreate(s.State.User.ID, guild.ID, cmd.Definition)
				if err != nil {
					log.Printf("[%s] failed to register command %s in guild %s: %v", m.Name(), cmd.Definition.Name, guild.ID, err)
					continue
				}
				b.registeredCmds = append(b.registeredCmds, registered)
			}
		}
	}
}

var knownPermissions = []struct {
	bit  int64
	name string
}{
	{discordgo.PermissionAdministrator, "Administrator"},
	{discordgo.PermissionManageRoles, "ManageRoles"},
	{discordgo.PermissionManageChannels, "ManageChannels"},
	{discordgo.PermissionManageMessages, "ManageMessages"},
	{discordgo.PermissionManageWebhooks, "ManageWebhooks"},
	{discordgo.PermissionKickMembers, "KickMembers"},
	{discordgo.PermissionBanMembers, "BanMembers"},
	{discordgo.PermissionModerateMembers, "ModerateMembers"},
}

// describePermissions renders DefaultMemberPermissions for log output.
// nil = no restriction (everyone); 0 = hidden from everyone except admins.
func describePermissions(p *int64) string {
	if p == nil {
		return "everyone"
	}
	if *p == 0 {
		return "admins only"
	}
	var names []string
	remaining := *p
	for _, kp := range knownPermissions {
		if remaining&kp.bit != 0 {
			names = append(names, kp.name)
			remaining &^= kp.bit
		}
	}
	if remaining != 0 {
		names = append(names, fmt.Sprintf("0x%x", remaining))
	}
	return strings.Join(names, "+")
}

func (b *Bot) runReadyHooks(s *discordgo.Session, r *discordgo.Ready) error {
	for _, m := range b.modules {
		hook, ok := m.(domain.ReadyHook)
		if !ok {
			continue
		}
		if err := hook.OnReady(b.rootCtx, s, r); err != nil {
			return fmt.Errorf("%s OnReady: %w", m.Name(), err)
		}
		log.Printf("[%s] ready hook complete", m.Name())
	}
	return nil
}

func (b *Bot) dispatchInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if h, ok := b.commandTable[i.ApplicationCommandData().Name]; ok {
		h(s, i)
	}
}
