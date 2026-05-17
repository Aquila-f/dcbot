package roles

import (
	"context"
	"dcbot/config"
	"dcbot/domain"
	"fmt"

	"github.com/bwmarrin/discordgo"
)

var (
	_ domain.Module          = (*Manager)(nil)
	_ domain.CommandProvider = (*Manager)(nil)
	_ domain.EventSubscriber = (*Manager)(nil)
	_ domain.ReadyHook       = (*Manager)(nil)
)

type Manager struct {
	store          *roleStore
	roleChannelID  string
	adminChannelID string
	roleMsgHeader  string
}

func New(cfg config.RolesConfig, adminChannelID string) (*Manager, error) {
	st, err := loadStore()
	if err != nil {
		return nil, fmt.Errorf("load role store: %w", err)
	}
	return &Manager{
		store:          st,
		roleChannelID:  cfg.ChannelID,
		adminChannelID: adminChannelID,
		roleMsgHeader:  cfg.MessageHeader,
	}, nil
}

func (m *Manager) Name() string { return "roles" }

func (m *Manager) RegisterHandlers(s *discordgo.Session) {
	s.AddHandler(m.handleReactionAdd)
	s.AddHandler(m.handleReactionRemove)
}

func (m *Manager) Commands() []domain.Command {
	return []domain.Command{
		{Definition: addRoleCmd, Handler: m.requireAdmin(m.handleAddRole)},
		{Definition: removeRoleCmd, Handler: m.requireAdmin(m.handleRemoveRole)},
	}
}

func (m *Manager) OnReady(ctx context.Context, s *discordgo.Session, _ *discordgo.Ready) error {
	return m.sync(ctx, s)
}
