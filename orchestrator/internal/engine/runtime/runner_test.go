package runtime

import (
	"context"
	"testing"
	"time"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
)

func TestSessionRunnerStartsOneWorkerPerProfile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	profileID := model.ProfileID{0x01}
	started := make(chan BotSpec, 2)
	runner := newSessionRunner(ctx, []BotSpec{{ProfileID: profileID, Username: "king_crimson_bot"}})
	runner.sleep = func(context.Context, time.Duration) error { return nil }
	runner.runWorker = func(ctx context.Context, bot BotSpec) error {
		started <- bot
		<-ctx.Done()
		return nil
	}

	if err := runner.Apply([]network.Intent{
		{ProfileID: profileID, Kind: network.IntentStartSession},
		{ProfileID: profileID, Kind: network.IntentStartSession},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	select {
	case got := <-started:
		if got.ProfileID != profileID {
			t.Fatalf("worker bot = %#v, want profile %x", got, profileID)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	select {
	case duplicate := <-started:
		t.Fatalf("duplicate worker started for %#v", duplicate)
	case <-time.After(20 * time.Millisecond):
	}

	if err := runner.Apply([]network.Intent{{ProfileID: profileID, Kind: network.IntentStopSession}}); err != nil {
		t.Fatalf("stop Apply() error = %v", err)
	}
	runner.Wait()
}
