package core

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestCreateAndDestroyMirroredBot(t *testing.T) {
	world := NewWorld()
	var bundle Bundle
	bundle.Set(model.CBot, model.Bot{ProfileID: model.ProfileID{1}, Username: "bot"})
	bundle.Set(model.CSession, model.Session{})
	bundle.Set(model.CPosition, model.Position{})
	bundle.Set(model.CRotation, model.Rotation{})
	bundle.Set(model.CVelocity, model.Velocity{})
	bundle.Set(model.CHealth, model.Health{Current: 20, Max: 20})
	bundle.Set(model.CGameMode, model.GameModeSurvival)
	bundle.Set(model.CInventory, model.Inventory{})
	bundle.Set(model.CEffects, model.Effects{})

	created, err := (CreateCommand{Bundle: bundle}).validate(world, NewShadowState(world))
	if err != nil {
		t.Fatalf("validate create: %v", err)
	}
	if err := created.apply(world); err != nil {
		t.Fatalf("apply create: %v", err)
	}
	if _, err := world.BotEntity(Entity{Index: 1, Generation: 1}); err != nil {
		t.Fatalf("created bot entity: %v", err)
	}
}
