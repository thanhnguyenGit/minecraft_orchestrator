package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"minecraft_orchestrator/internal/engine/model"
)

func TestRuntimeBootstrapsEntityBeforeStartingMineflayerHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	profileID := model.ProfileID{0x01}
	clock := &runtimeClock{onWait: func() {
		cancel()
	}}
	hostScript, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "bots", "src", "host.ts"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(HostConfig{Host: "127.0.0.1", Port: 25565, Auth: "offline", Version: "1.21.11", NodeBinary: "node", HostScript: hostScript}, []BotSpec{{ProfileID: profileID, Username: "king_crimson_bot"}}, clock)
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
