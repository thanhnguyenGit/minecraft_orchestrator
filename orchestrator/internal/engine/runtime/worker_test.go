package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/mc_protocol/client"
)

func TestWorkerStopsAfterProfileMismatch(t *testing.T) {
	events := make(chan client.Event, 1)
	events <- client.Event{Phase: client.PhaseLogin, Message: client.LoginSuccess{UUID: [16]byte{0x02}}}
	close(events)
	session := &fakeSession{events: events, waitErr: errors.New("profile mismatch")}
	inbox := network.NewInbox()
	worker := Worker{
		Bot:     BotSpec{ProfileID: model.ProfileID{0x01}, Username: "king_crimson_bot"},
		Inbox:   inbox,
		Factory: func(client.Config) (Session, error) { return session, nil },
		Sleep:   func(context.Context, time.Duration) error { t.Fatal("mismatched profile must not retry"); return nil },
	}

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	batch := inbox.Drain()
	if len(batch.Events) != 2 {
		t.Fatalf("events = %#v, want Connecting then Failed", batch.Events)
	}
	if batch.Events[0].Kind != network.EventConnecting {
		t.Fatalf("first event = %#v, want Connecting", batch.Events[0])
	}
	if failed := batch.Events[1]; failed.Kind != network.EventSessionFailed || failed.Failure == "" {
		t.Fatalf("second event = %#v, want terminal failure", failed)
	}
	if !session.closed {
		t.Fatal("mismatched session was not closed")
	}
}

type fakeSession struct {
	events  chan client.Event
	waitErr error
	closed  bool
}

func (s *fakeSession) Start(context.Context) error { return nil }
func (s *fakeSession) Events() <-chan client.Event { return s.events }
func (s *fakeSession) Wait() error                 { return s.waitErr }
func (s *fakeSession) Close() error {
	s.closed = true
	return nil
}
