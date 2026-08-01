package core

import (
	"testing"

	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/engine/scheduler"
)

func TestNetworkApplySystemRejectsStaleHostObservations(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x04}
	stageMirroredBot(t, world, profileID)
	snapshot := func(x float64) *network.HostSnapshot {
		return &network.HostSnapshot{Vitals: network.HostVitals{Health: 12}, Position: model.Position{X: x}, Inventory: model.Inventory{SelectedHotbarSlot: 1}}
	}
	err := (NetworkApplySystem{}).Run(&scheduler.RunContext{World: world, Data: &TickData{Network: network.Batch{Events: []network.Event{
		{ProfileID: profileID, Kind: network.EventHostSnapshot, RemoteSessionID: "host-a", Sequence: 2, Snapshot: snapshot(2)},
		{ProfileID: profileID, Kind: network.EventHostSnapshot, RemoteSessionID: "host-a", Sequence: 1, Snapshot: snapshot(1)},
		{ProfileID: profileID, Kind: network.EventHostSnapshot, RemoteSessionID: "host-b", Sequence: 1, Snapshot: snapshot(3)},
	}}}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	view := world.MirroredBotViews()[0]
	if view.Positions[0].X != 3 || view.Sessions[0].RemoteSessionID != "host-b" || view.Sessions[0].LastSequence != 1 {
		t.Fatalf("host state = position=%+v session=%+v", view.Positions[0], view.Sessions[0])
	}
}

func TestBootstrapSystemCreatesMirroredBotAndEmitsStartIntent(t *testing.T) {
	world := enginecore.NewWorld()
	outbox := network.NewOutbox()
	profileID := model.ProfileID{0x03}
	commands := enginecore.NewCommandBuffer(0)
	if err := (BootstrapSystem{}).Run(&scheduler.RunContext{World: world, Commands: commands, Data: &TickData{Bootstrap: []model.Bot{{ProfileID: profileID, Username: "bot"}}, Outbox: outbox}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	world.Stage(commands.Envelopes(), []model.Mask{model.MirroredBotMask})
	if err := world.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if intents := outbox.Drain(); len(intents) != 1 || intents[0].Kind != network.IntentStartHost {
		t.Fatalf("intents = %#v", intents)
	}
}

func stageMirroredBot(t testing.TB, world *enginecore.World, profileID model.ProfileID) {
	t.Helper()
	var bundle enginecore.Bundle
	bundle.Set(model.CBot, model.Bot{ProfileID: profileID, Username: "bot"})
	bundle.Set(model.CSession, model.Session{})
	bundle.Set(model.CPosition, model.Position{})
	bundle.Set(model.CRotation, model.Rotation{})
	bundle.Set(model.CVelocity, model.Velocity{})
	bundle.Set(model.CHealth, model.Health{Current: 20, Max: 20})
	bundle.Set(model.CGameMode, model.GameModeSurvival)
	bundle.Set(model.CInventory, model.Inventory{})
	bundle.Set(model.CEffects, model.Effects{})
	commands := enginecore.NewCommandBuffer(0)
	commands.Stage(enginecore.CreateCommand{Bundle: bundle})
	world.Stage(commands.Envelopes(), []model.Mask{model.MirroredBotMask})
	if err := world.Sync(); err != nil {
		t.Fatalf("Sync(): %v", err)
	}
}
