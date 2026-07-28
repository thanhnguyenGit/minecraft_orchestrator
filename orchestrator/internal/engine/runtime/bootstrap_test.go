package runtime

import (
	"fmt"
	"testing"

	"minecraft_orchestrator/internal/mc_protocol/client"
)

func TestBootstrapBotsCreatesTenOfflineProfiles(t *testing.T) {
	next := 0
	bots, err := bootstrapBots(func() string {
		next++
		return fmt.Sprintf("bot_%d", next)
	})
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
		if got, want := [16]byte(bot.ProfileID), client.OfflineUUID(bot.Username); got != want {
			t.Fatalf("profile UUID for %q = %x, want %x", bot.Username, got, want)
		}
	}
}
