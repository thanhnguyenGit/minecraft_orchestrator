package runtime

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestBootstrapBotsCreatesOneBotProfileID(t *testing.T) {
	bots, err := bootstrapBots(func() string { return "bot_test" })
	if err != nil {
		t.Fatalf("bootstrapBots() error = %v", err)
	}
	if len(bots) != BootstrapBotCount {
		t.Fatalf("bot count = %d, want %d", len(bots), BootstrapBotCount)
	}

	seen := make(map[string]struct{}, len(bots))
	for _, bot := range bots {
		if _, exists := seen[bot.Username]; exists {
			t.Fatalf("duplicate generated username %q", bot.Username)
		}
		seen[bot.Username] = struct{}{}
		if bot.ProfileID == (model.ProfileID{}) {
			t.Fatalf("profile ID for %q is zero", bot.Username)
		}
	}
}

func TestOfflineProfileIDIsDeterministic(t *testing.T) {
	id1 := offlineProfileID("bot_test")
	id2 := offlineProfileID("bot_test")
	if id1 != id2 {
		t.Fatalf("offlineProfileID() = %x and %x, want equal", id1, id2)
	}
	id3 := offlineProfileID("bot_other")
	if id1 == id3 {
		t.Fatal("offlineProfileID() for different usernames are equal")
	}
}
