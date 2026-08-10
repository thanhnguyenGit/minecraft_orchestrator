package hosttransport

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
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

func TestRealityStateToEventPreservesSuccessfulAndFailedActionOutcomes(t *testing.T) {
	profile := model.ProfileID{2}
	reality := &orchestratorv1.RealityState{
		ProfileId: profile[:],
		Sequence:  8,
		SessionId: "session-a",
		ActionOutcomes: []*orchestratorv1.ActionOutcome{
			{
				ControllerSequence: 40,
				Kind:               orchestratorv1.ControllerActionKind_CONTROLLER_ACTION_KIND_CRAFT,
				Status:             orchestratorv1.CommandStatus_COMMAND_STATUS_COMPLETED,
				Detail:             "craft_completed",
			},
			{
				ControllerSequence: 41,
				Kind:               orchestratorv1.ControllerActionKind_CONTROLLER_ACTION_KIND_EQUIP,
				Status:             orchestratorv1.CommandStatus_COMMAND_STATUS_FAILED,
				Detail:             "item_unavailable",
			},
		},
	}
	payload, err := proto.Marshal(reality)
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	var decoded orchestratorv1.RealityState
	if err := proto.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("proto.Unmarshal() error = %v", err)
	}

	event, err := RealityStateToEvent(&decoded, map[model.ProfileID]struct{}{profile: {}})
	if err != nil {
		t.Fatalf("RealityStateToEvent() error = %v", err)
	}
	if event.RealityState == nil || event.RemoteSessionID != "session-a" {
		t.Fatal("reality event is nil")
	}
	want := []model.ActionOutcome{
		{ControllerSequence: 40, Action: model.ControllerActionCraft, Status: model.ActionOutcomeCompleted, Detail: "craft_completed"},
		{ControllerSequence: 41, Action: model.ControllerActionEquip, Status: model.ActionOutcomeFailed, Detail: "item_unavailable"},
	}
	if got := event.RealityState.ActionOutcomes; !reflect.DeepEqual(got, want) {
		t.Fatalf("action outcomes = %#v, want %#v", got, want)
	}
	if !event.RealityState.ActionFailed || event.RealityState.Failure != "item_unavailable" || event.RealityState.ActionFailureCorrelation != 41 {
		t.Fatalf("legacy failure = %#v, want failed equip outcome with controller correlation", event.RealityState)
	}
}
