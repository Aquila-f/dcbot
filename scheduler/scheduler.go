package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/robfig/cron/v3"
)

const taskTimeout = 30 * time.Second

type messageSender interface {
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type Scheduler struct {
	cron           *cron.Cron
	sender         messageSender
	adminChannelID string
}

func New(session *discordgo.Session, adminChannelID string, loc *time.Location) *Scheduler {
	return newWithSender(session, adminChannelID, loc)
}

func newWithSender(sender messageSender, adminChannelID string, loc *time.Location) *Scheduler {
	return &Scheduler{
		cron:           cron.New(cron.WithLocation(loc)),
		sender:         sender,
		adminChannelID: adminChannelID,
	}
}

func (s *Scheduler) Register(spec string, t Task) error {
	_, err := s.cron.AddFunc(spec, func() { s.run(t) })
	if err != nil {
		return fmt.Errorf("register task %q: %w", t.Name(), err)
	}
	return nil
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	<-s.cron.Stop().Done()
}

func (s *Scheduler) run(t Task) {
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	payload, err := t.Build(ctx)
	if err != nil {
		s.reportFailure(t.Name(), fmt.Errorf("build: %w", err))
		return
	}

	if _, err := s.sender.ChannelMessageSendComplex(payload.ChannelID, &discordgo.MessageSend{
		Content: payload.Content,
		Embeds:  payload.Embeds,
	}); err != nil {
		s.reportFailure(t.Name(), fmt.Errorf("send: %w", err))
		return
	}
}

func (s *Scheduler) reportFailure(taskName string, err error) {
	msg := fmt.Sprintf("[scheduler] task %q failed: %v", taskName, err)
	log.Print(msg)

	if s.adminChannelID == "" {
		return
	}
	if _, sendErr := s.sender.ChannelMessageSendComplex(s.adminChannelID, &discordgo.MessageSend{
		Content: msg,
	}); sendErr != nil {
		log.Printf("[scheduler] failed to notify admin channel: %v", sendErr)
	}
}
