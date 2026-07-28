package core

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestWorldIndexesBotByProfileID(t *testing.T) {
	w := NewWorld()
	profileID := model.ProfileID{0x01}
	var bundle Bundle
	bundle.Set(model.CBot, model.Bot{
		ProfileID: profileID,
		Username:  "king_crimson_bot",
	})

	entity := mustCreateEntity(t, w, bundle)

	if got, found := w.botIndex[profileID]; !found || got != entity {
		t.Fatalf("botIndex[%v] = %v, found=%v; want %v", profileID, got, found, entity)
	}
}
