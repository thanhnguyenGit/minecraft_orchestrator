package core

import (
	"testing"

	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/engine/scheduler"
)

func TestNetworkApplySystemAppliesReadyAndRelativeCorrection(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x01}
	stageMirroredBot(t, world, profileID)

	system := NetworkApplySystem{}
	err := system.Run(&scheduler.RunContext{
		World: world,
		Data: &TickData{Network: network.Batch{Events: []network.Event{
			{ProfileID: profileID, AttemptID: 7, Kind: network.EventConnecting},
			{ProfileID: profileID, AttemptID: 7, Kind: network.EventPlayReady, PlayerEntityID: 30011},
			{
				ProfileID: profileID,
				AttemptID: 7,
				Kind:      network.EventPositionCorrection,
				Correction: &network.PositionCorrection{
					Position: model.Position{X: 3, Y: 70, Z: 200},
					Rotation: model.Rotation{Yaw: 15, Pitch: 5},
					Relative: network.RelativePositionX | network.RelativeYaw,
				},
			},
		}}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	views := world.MirroredBotViews()
	if len(views) != 1 || len(views[0].Bots) != 1 {
		t.Fatalf("mirrored views = %#v, want one bot", views)
	}

	view := views[0]
	if got := view.Sessions[0]; got.Phase != model.SessionPlayReady || got.AttemptID != 7 || got.PlayerEntityID != 30011 {
		t.Fatalf("session = %+v, want PlayReady attempt 7 entity 30011", got)
	}
	if got := view.Positions[0]; got != (model.Position{X: 13, Y: 70, Z: 200}) {
		t.Fatalf("position = %+v, want {13,70,200}", got)
	}
	if got := view.Rotations[0]; got != (model.Rotation{Yaw: 105, Pitch: 5}) {
		t.Fatalf("rotation = %+v, want {105,5}", got)
	}
}

func TestNetworkApplySystemUpdatesHealthWithoutDiscardingMaximum(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x02}
	stageMirroredBot(t, world, profileID)

	current := 6.5
	system := NetworkApplySystem{}
	err := system.Run(&scheduler.RunContext{
		World: world,
		Data: &TickData{Network: network.Batch{Events: []network.Event{
			{ProfileID: profileID, AttemptID: 7, Kind: network.EventConnecting},
			{ProfileID: profileID, AttemptID: 7, Kind: network.EventPlayReady},
			{ProfileID: profileID, AttemptID: 7, Kind: network.EventStatePatch, Patch: &network.StatePatch{HealthCurrent: &current}},
		}}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := world.MirroredBotViews()[0].Healths[0]
	if got != (model.Health{Current: 6.5, Max: 20}) {
		t.Fatalf("health = %+v, want {Current: 6.5, Max: 20}", got)
	}
}

func TestBootstrapSystemCreatesMirroredBotAndEmitsStartIntent(t *testing.T) {
	world := enginecore.NewWorld()
	outbox := network.NewOutbox()
	profileID := model.ProfileID{0x03}
	commands := enginecore.NewCommandBuffer(0)

	err := (BootstrapSystem{}).Run(&scheduler.RunContext{
		World:    world,
		Commands: commands,
		Data: &TickData{
			Bootstrap: []model.Bot{{ProfileID: profileID, Username: "king_crimson_bot"}},
			Outbox:    outbox,
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	world.Stage(commands.Envelopes(), []model.Mask{model.MirroredBotMask})
	if err := world.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	views := world.MirroredBotViews()
	if len(views) != 1 || len(views[0].Bots) != 1 {
		t.Fatalf("views = %#v, want one mirrored bot", views)
	}
	if got := views[0].Sessions[0]; got.Phase != model.SessionStopped {
		t.Fatalf("session = %+v, want Stopped", got)
	}
	if intents := outbox.Drain(); len(intents) != 1 || intents[0] != (network.Intent{ProfileID: profileID, Kind: network.IntentStartSession}) {
		t.Fatalf("intents = %#v, want start intent", intents)
	}
}

func stageMirroredBot(t testing.TB, world *enginecore.World, profileID model.ProfileID) {
	t.Helper()
	var bundle enginecore.Bundle
	bundle.Set(model.CBot, model.Bot{ProfileID: profileID, Username: "king_crimson_bot"})
	bundle.Set(model.CSession, model.Session{Phase: model.SessionStopped})
	bundle.Set(model.CPosition, model.Position{X: 10, Y: 64, Z: 200})
	bundle.Set(model.CRotation, model.Rotation{Yaw: 90})
	bundle.Set(model.CVelocity, model.Velocity{})
	bundle.Set(model.CHealth, model.Health{Current: 20, Max: 20})
	bundle.Set(model.CGameMode, model.GameModeSurvival)

	world.Stage([]enginecore.Envelop{{
		Command: enginecore.CreateCommand{Bundle: bundle},
	}}, []model.Mask{model.MirroredBotMask})
	if err := world.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
}
