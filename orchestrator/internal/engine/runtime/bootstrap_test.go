package runtime

import (
	"fmt"
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestBootstrapBotsCreatesSevenHostProfileIDs(t *testing.T) {
	next := 0
	bots, err := bootstrapBots(
		func() string { next++; return fmt.Sprintf("bot_%d", next) },
		func() model.ProfileID { return model.ProfileID{byte(next)} },
	)
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
