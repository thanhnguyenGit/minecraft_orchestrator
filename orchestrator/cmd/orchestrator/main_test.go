package main

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"minecraft_orchestrator/internal/bus"
	"minecraft_orchestrator/internal/config"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

func TestBootstrapPublishesEnvironmentConnectCommand(t *testing.T) {
	t.Setenv("BOT_ID", "env_bot")
	t.Setenv("MINECRAFT_HOST", "127.0.0.1")
	t.Setenv("MINECRAFT_PORT", "25566")
	t.Setenv("MINECRAFT_USERNAME", "env_user")
	t.Setenv("MINECRAFT_AUTH", "offline")
	t.Setenv("MINECRAFT_VERSION", "1.21.11")
	publisher := &recordingCommandPublisher{}

	cfg := config.LoadConfig()

	if err := bootstrap(context.Background(), publisher,cfg); err != nil {
		t.Fatalf("bootstrap() error = %v", err)
	}

	command := publisher.command
	if command == nil {
		t.Fatal("bootstrap() did not publish a command")
	}
	connect := command.GetConnect()
	if command.GetBotId() != "env_bot" || connect.GetHost() != "127.0.0.1" ||
		connect.GetPort() != 25566 || connect.GetUsername() != "env_user" ||
		connect.GetAuth() != "offline" || connect.GetVersion() != "1.21.11" {
		t.Fatalf("connect command = %#v", command)
	}
}

func TestStreamEventsStartsAtNewMessagesAndForwardsInOrder(t *testing.T) {
	reader := &scriptedEventReader{responses: [][]bus.StreamEvent{{
		{ID: "1-0", Event: &orchestratorv1.BotEvent{BotId: "first"}},
		{ID: "2-0", Event: &orchestratorv1.BotEvent{BotId: "second"}},
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan bus.StreamEvent, 2)
	done := make(chan error, 1)
	go func() {
		done <- streamEvents(ctx, reader, events)
	}()

	if event := <-events; event.ID != "1-0" || event.Event.GetBotId() != "first" {
		t.Fatalf("first event = %#v", event)
	}
	if event := <-events; event.ID != "2-0" || event.Event.GetBotId() != "second" {
		t.Fatalf("second event = %#v", event)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("streamEvents() error = %v", err)
	}

	if got, want := reader.positions(), []string{"$", "2-0"}; !equalStrings(got, want) {
		t.Fatalf("reader positions = %v, want %v", got, want)
	}
}

func TestPrintEventsWritesStreamIDAndProtobufJSON(t *testing.T) {
	events := make(chan bus.StreamEvent, 1)
	events <- bus.StreamEvent{
		ID:    "42-0",
		Event: &orchestratorv1.BotEvent{
			BotId: "king_crimson", 
			MessageId: "event-1",
		},
	}
	close(events)

	var output bytes.Buffer
	if err := printEvents(&output, events); err != nil {
		t.Fatalf("printEvents() error = %v", err)
	}

	if got, want := output.String(), "42-0 {\"bot_id\":\"king_crimson\",\"message_id\":\"event-1\"}\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

type scriptedEventReader struct {
	mu        sync.Mutex
	responses [][]bus.StreamEvent
	from      []string
	index     int
}

type recordingCommandPublisher struct {
	command *orchestratorv1.BotCommand
}

func (p *recordingCommandPublisher) PublishCommand(_ context.Context, command *orchestratorv1.BotCommand) (string, error) {
	p.command = command
	return "1-0", nil
}

func (r *scriptedEventReader) ReadEvents(ctx context.Context, from string) ([]bus.StreamEvent, error) {
	r.mu.Lock()
	r.from = append(r.from, from)
	if r.index < len(r.responses) {
		response := r.responses[r.index]
		r.index++
		r.mu.Unlock()
		return response, nil
	}
	r.mu.Unlock()

	<-ctx.Done()
	return nil, ctx.Err()
}

func (r *scriptedEventReader) positions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.from...)
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestStreamEventsReturnsAfterCancellation(t *testing.T) {
	reader := &scriptedEventReader{}
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan bus.StreamEvent)
	done := make(chan error, 1)
	go func() { done <- streamEvents(ctx, reader, events) }()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("streamEvents() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("streamEvents() did not stop after cancellation")
	}
}
