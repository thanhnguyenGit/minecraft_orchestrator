package runtime

import (
	"context"
	"testing"
	"time"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/mc_protocol/client"
)

func TestRuntimeBootstrapsEntityBeforeStartingSessionWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	profileID := model.ProfileID{0x01}
	started := make(chan client.Config, 1)
	clock := &runtimeClock{onWait: func() {
		<-started
		cancel()
	}}
	runtime, err := NewRuntime(client.Config{Host: "127.0.0.1", Port: 25565}, []BotSpec{{ProfileID: profileID, Username: "king_crimson_bot"}}, func(cfg client.Config) (Session, error) {
		started <- cfg
		return &contextSession{events: make(chan client.Event)}, nil
	}, clock)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	defer runtime.Close()

	if err := runtime.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	views := runtime.World.MirroredBotViews()
	if len(views) != 1 || len(views[0].Bots) != 1 || views[0].Bots[0].ProfileID != profileID {
		t.Fatalf("World mirrored bots = %#v, want bootstrapped profile", views)
	}
}

type runtimeClock struct {
	now    time.Time
	onWait func()
}

func (c *runtimeClock) Now() time.Time { return c.now }
func (c *runtimeClock) Wait(context.Context, time.Duration) error {
	if c.onWait != nil {
		c.onWait()
	}
	return context.Canceled
}

type contextSession struct{ events chan client.Event }

func (s *contextSession) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		close(s.events)
	}()
	return nil
}
func (s *contextSession) Events() <-chan client.Event { return s.events }
func (*contextSession) Wait() error                   { return context.Canceled }
func (*contextSession) Close() error                  { return nil }
