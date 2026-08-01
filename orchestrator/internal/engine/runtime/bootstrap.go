package runtime

import (
	"errors"
	"fmt"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/mc_protocol/client"
)

const BootstrapBotCount = 7

func BootstrapBots() ([]BotSpec, error) {
	return bootstrapBots(client.GenRandomUserName)
}

func bootstrapBots(generateUsername func() string) ([]BotSpec, error) {
	if generateUsername == nil {
		return nil, errors.New("bootstrap username generator is required")
	}

	bots := make([]BotSpec, 0, BootstrapBotCount)
	seen := make(map[string]struct{}, BootstrapBotCount)
	for attempts := 0; len(bots) < BootstrapBotCount; attempts++ {
		if attempts >= BootstrapBotCount*100 {
			return nil, errors.New("unable to generate unique bootstrap bot usernames")
		}
		username := generateUsername()
		if username == "" {
			return nil, errors.New("generated bootstrap username is empty")
		}
		if _, exists := seen[username]; exists {
			continue
		}
		seen[username] = struct{}{}
		bots = append(bots, BotSpec{
			ProfileID: model.ProfileID(client.OfflineUUID(username)),
			Username:  username,
		})
	}

	for index, bot := range bots {
		if bot.ProfileID == (model.ProfileID{}) {
			return nil, fmt.Errorf("bootstrap bot %d has zero profile UUID", index)
		}
	}
	return bots, nil
}
