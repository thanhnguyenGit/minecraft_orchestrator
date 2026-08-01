package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"minecraft_orchestrator/internal/engine/model"
)

const BootstrapBotCount = 7

type BotSpec struct {
	ProfileID model.ProfileID
	Username  string
}

func BootstrapBots() ([]BotSpec, error) {
	return bootstrapBots(randomUsername, randomProfileID)
}

func bootstrapBots(generateUsername func() string, generateProfileID func() model.ProfileID) ([]BotSpec, error) {
	if generateUsername == nil || generateProfileID == nil {
		return nil, errors.New("bootstrap identity generators are required")
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
			ProfileID: generateProfileID(),
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

func randomUsername() string {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return "bot_" + hex.EncodeToString(raw)
}

func randomProfileID() model.ProfileID {
	var profileID model.ProfileID
	if _, err := rand.Read(profileID[:]); err != nil {
		return model.ProfileID{}
	}
	return profileID
}
