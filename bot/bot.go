package bot

import (
	"context"
	"dcbot/config"
	"dcbot/domain"
	"dcbot/modules/chat"
	"dcbot/modules/leetcode"
	"dcbot/modules/roles"
	"dcbot/scheduler"
	"fmt"
	"log"

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
	if err := b.buildModules(); err != nil {
		return err
	}

	// AddHandler must run before Open or early events get dropped.
	readyErr := b.installCoreHandlers()
	b.registerEventSubscribers()
	if err := b.registerTaskProviders(); err != nil {
		return err
	}

	if err := b.session.Open(); err != nil {
		return err
	}
	if err := <-readyErr; err != nil {
		b.Stop()
		return err
	}

	b.scheduler.Start()
	return nil
}

func (b *Bot) buildModules() error {
	rolesHandler, err := roles.New(b.cfg.Roles, b.cfg.AdminChannelID)
	if err != nil {
		return fmt.Errorf("init roles handler: %w", err)
	}
	chatHandler, err := chat.New(b.cfg.LLM, b.cfg.Location)
	if err != nil {
		return fmt.Errorf("init chat handler: %w", err)
	}

	b.modules = []domain.Module{rolesHandler, chatHandler}
	if b.cfg.LeetcodeChannelID != "" {
		b.modules = append(b.modules, leetcode.New(b.cfg.LeetcodeChannelID))
	}
	return nil
}

// installCoreHandlers wires the bot's own session listeners: slash-command
// dispatch, and the READY-time orchestration that runs module command
// registration + ready hooks.
func (b *Bot) installCoreHandlers() <-chan error {
	b.session.AddHandler(b.dispatchInteraction)

	readyErr := make(chan error, 1)
	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as %s", s.State.User.Username)
		b.registerCommandProviders(s, r)
		readyErr <- b.runReadyHooks(s, r)
	})
	return readyErr
}

func (b *Bot) Stop() {
	b.cancel()
	b.scheduler.Stop()
	appID := b.session.State.User.ID
	for _, cmd := range b.registeredCmds {
		if err := b.session.ApplicationCommandDelete(appID, cmd.GuildID, cmd.ID); err != nil {
			log.Printf("failed to delete command %s in guild %s: %v", cmd.Name, cmd.GuildID, err)
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
			log.Printf("[%s] /%s — %s", m.Name(), cmd.Definition.Name, cmd.Definition.Description)
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

func (b *Bot) registerTaskProviders() error {
	for _, m := range b.modules {
		tp, ok := m.(domain.TaskProvider)
		if !ok {
			continue
		}
		for _, st := range tp.Tasks() {
			if err := b.scheduler.Register(st.Spec, st.Task); err != nil {
				return fmt.Errorf("module %s: register %s: %w", m.Name(), st.Task.Name(), err)
			}
			log.Printf("[%s] scheduled %s @ %q", m.Name(), st.Task.Name(), st.Spec)
		}
	}
	return nil
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
