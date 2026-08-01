package hosttransport

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

func TestObservationToEventMapsSnapshot(t *testing.T) {
	profile := model.ProfileID{1}
	observation := &orchestratorv1.BotObservation{
		ProfileId: profile[:], SessionId: "session-a", Sequence: 4,
		Payload: &orchestratorv1.BotObservation_StateSnapshot{StateSnapshot: &orchestratorv1.HostStateSnapshot{State: &orchestratorv1.HostBotState{
			Vitals:    &orchestratorv1.HostVitals{Health: 17.5, Food: 18, Saturation: 4},
			Position:  &orchestratorv1.HostPosition{Dimension: "minecraft:overworld", X: 1, Y: 64, Z: -2, Yaw: 90, Pitch: 15, VelocityX: .1},
			Inventory: &orchestratorv1.HostInventory{SelectedHotbarSlot: 2, Slots: []*orchestratorv1.HostInventorySlot{{Slot: 36, Item: &orchestratorv1.HostItemStack{Id: 5, Name: "minecraft:oak_planks", Count: 3}}}},
			Effects:   []*orchestratorv1.HostPotionEffect{{Id: 1, Name: "speed", Amplifier: 2, DurationTicks: 60}},
			GameMode:  0,
		}}},
	}

	event, err := ObservationToEvent(observation, map[model.ProfileID]struct{}{profile: {}})
	if err != nil {
		t.Fatalf("ObservationToEvent() error = %v", err)
	}
	if event.Kind != network.EventHostSnapshot || event.RemoteSessionID != "session-a" || event.Sequence != 4 {
		t.Fatalf("event = %#v", event)
	}
	if event.Snapshot == nil || event.Snapshot.Position.Y != 64 || event.Snapshot.Inventory.Slots[0].Item.Name != "minecraft:oak_planks" {
		t.Fatalf("snapshot = %#v", event.Snapshot)
	}
}

func TestObservationToEventRejectsUnknownAndMalformedProfiles(t *testing.T) {
	for _, observation := range []*orchestratorv1.BotObservation{
		{ProfileId: []byte{1}, SessionId: "session", Sequence: 1},
		{ProfileId: make([]byte, 16), SessionId: "session", Sequence: 1},
	} {
		if _, err := ObservationToEvent(observation, map[model.ProfileID]struct{}{}); err == nil {
			t.Fatalf("ObservationToEvent(%#v) unexpectedly succeeded", observation)
		}
	}
}
