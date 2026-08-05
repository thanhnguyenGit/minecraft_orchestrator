package runtime

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"minecraft_orchestrator/internal/engine/model"
)

const BootstrapBotCount = 1

type BotSpec struct {
	ProfileID model.ProfileID
	Username  string
}

func BootstrapBots() ([]BotSpec, error) {
	return bootstrapBots(randomUsername)
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
			ProfileID: offlineProfileID(username),
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

func offlineProfileID(username string) model.ProfileID {
	hash := md5.Sum([]byte("OfflinePlayer:" + username))
	var profileID model.ProfileID
	copy(profileID[:], hash[:])
	profileID[6] = (profileID[6] & 0x0f) | 0x30
	profileID[8] = (profileID[8] & 0x3f) | 0x80
	return profileID
}
