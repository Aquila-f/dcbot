package scheduler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

type fakeSender struct {
	mu    sync.Mutex
	sends []sendCall
	err   error
}

type sendCall struct {
	channelID string
	data      *discordgo.MessageSend
}

func (f *fakeSender) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, sendCall{channelID: channelID, data: data})
	if f.err != nil {
		return nil, f.err
	}
	return &discordgo.Message{}, nil
}

func (f *fakeSender) calls() []sendCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sendCall(nil), f.sends...)
}

type fakeTask struct {
	name    string
	payload Payload
	err     error
}

func (t fakeTask) Name() string                                { return t.name }
func (t fakeTask) Build(_ context.Context) (Payload, error)    { return t.payload, t.err }

func newTestScheduler(sender messageSender, adminChannelID string) *Scheduler {
	return newWithSender(sender, adminChannelID, time.UTC)
}

func TestRegister_InvalidSpec(t *testing.T) {
	s := newTestScheduler(&fakeSender{}, "admin")
	err := s.Register("not a cron expr", fakeTask{name: "x"})
	if err == nil {
		t.Fatal("expected error for invalid cron spec, got nil")
	}
}

func TestRegister_ValidSpec(t *testing.T) {
	s := newTestScheduler(&fakeSender{}, "admin")
	if err := s.Register("0 9 * * *", fakeTask{name: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_SuccessSendsPayload(t *testing.T) {
	sender := &fakeSender{}
	s := newTestScheduler(sender, "admin")

	s.run(fakeTask{
		name:    "demo",
		payload: Payload{ChannelID: "target-channel", Content: "hello"},
	})

	calls := sender.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 send, got %d", len(calls))
	}
	if calls[0].channelID != "target-channel" {
		t.Errorf("channelID = %q, want %q", calls[0].channelID, "target-channel")
	}
	if calls[0].data.Content != "hello" {
		t.Errorf("content = %q, want %q", calls[0].data.Content, "hello")
	}
}

func TestRun_BuildFailureNotifiesAdmin(t *testing.T) {
	sender := &fakeSender{}
	s := newTestScheduler(sender, "admin-chan")

	s.run(fakeTask{name: "boom", err: errors.New("kaboom")})

	calls := sender.calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 admin notification, got %d", len(calls))
	}
	if calls[0].channelID != "admin-chan" {
		t.Errorf("channelID = %q, want admin-chan", calls[0].channelID)
	}
	if !strings.Contains(calls[0].data.Content, `"boom"`) || !strings.Contains(calls[0].data.Content, "kaboom") {
		t.Errorf("admin message missing context: %q", calls[0].data.Content)
	}
}

func TestRun_SendFailureNotifiesAdmin(t *testing.T) {
	sender := &fakeSender{err: errors.New("discord down")}
	s := newTestScheduler(sender, "admin-chan")

	s.run(fakeTask{
		name:    "demo",
		payload: Payload{ChannelID: "target-channel", Content: "hello"},
	})

	calls := sender.calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 sends (send attempt + admin notify), got %d", len(calls))
	}
	if calls[0].channelID != "target-channel" {
		t.Errorf("first send should target task channel, got %q", calls[0].channelID)
	}
	if calls[1].channelID != "admin-chan" {
		t.Errorf("second send should target admin, got %q", calls[1].channelID)
	}
}

func TestRun_NoAdminChannelSkipsNotify(t *testing.T) {
	sender := &fakeSender{}
	s := newTestScheduler(sender, "")

	s.run(fakeTask{name: "boom", err: errors.New("kaboom")})

	if calls := sender.calls(); len(calls) != 0 {
		t.Fatalf("expected no sends when admin channel empty, got %d", len(calls))
	}
}
