package hosttransport

import (
	"fmt"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

// ObservationToEvent validates a host observation at the transport boundary
// and converts it to an immutable Inbox event. It has no World access.
func ObservationToEvent(
	observation *orchestratorv1.BotObservation,
	allowed map[model.ProfileID]struct{},
) (network.Event, error) {
	if observation == nil {
		return network.Event{}, fmt.Errorf("host observation is nil")
	}

	if len(observation.GetProfileId()) != len(model.ProfileID{}) {
		return network.Event{}, fmt.Errorf("host profile id has length %d, want 16", len(observation.GetProfileId()))
	}

	var profileID model.ProfileID
	copy(profileID[:], observation.GetProfileId())
	if _, ok := allowed[profileID]; !ok {
		return network.Event{}, fmt.Errorf("host observation has unknown profile id")
	}

	if observation.GetSessionId() == "" {
		return network.Event{}, fmt.Errorf("host observation has empty session id")
	}

	if observation.GetSequence() == 0 {
		return network.Event{}, fmt.Errorf("host observation has zero sequence")
	}

	event := network.Event{
		ProfileID:       profileID,
		RemoteSessionID: observation.GetSessionId(),
		Sequence:        observation.GetSequence(),
	}

	switch payload := observation.GetPayload().(type) {
	case *orchestratorv1.BotObservation_StatusChanged:
		event.Kind = network.EventHostStatus
		event.HostStatus, event.Failure = hostStatus(payload.StatusChanged)
	case *orchestratorv1.BotObservation_Spawned:
		event.Kind = network.EventHostSnapshot
		event.Snapshot = botState(payload.Spawned.GetState())
	case *orchestratorv1.BotObservation_StateSnapshot:
		event.Kind = network.EventHostSnapshot
		event.Snapshot = botState(payload.StateSnapshot.GetState())
	case *orchestratorv1.BotObservation_VitalsChanged:
		event.Kind = network.EventHostVitals
		vitals := vitals(payload.VitalsChanged.GetVitals())
		event.Vitals = &vitals
	case *orchestratorv1.BotObservation_EffectsChanged:
		event.Kind = network.EventHostEffects
		effects := effects(payload.EffectsChanged.GetEffects())
		event.Effects = &effects
	case *orchestratorv1.BotObservation_PositionChanged:
		event.Kind = network.EventHostPosition
		snapshot := network.HostSnapshot{
			Position: position(payload.PositionChanged.GetPosition()),
			Rotation: rotation(payload.PositionChanged.GetPosition()),
			Velocity: velocity(payload.PositionChanged.GetPosition()),
		}
		event.Position = &snapshot
	case *orchestratorv1.BotObservation_InventoryChanged:
		event.Kind = network.EventHostInventory
		inventory := inventory(&orchestratorv1.HostInventory{
			SelectedHotbarSlot: payload.InventoryChanged.GetSelectedHotbarSlot(),
			Slots:               payload.InventoryChanged.GetSlots(),
		})
		event.Inventory = &inventory
	case *orchestratorv1.BotObservation_ChunkLoaded:
		event.Kind = network.EventChunkLoad
		event.ChunkLoad = &network.ChunkLoad{
			Position: model.ChunkPosition{
				X: payload.ChunkLoaded.GetChunkX(),
				Z: payload.ChunkLoaded.GetChunkZ(),
			},
			Data:   payload.ChunkLoaded.GetData(),
			MinY:   payload.ChunkLoaded.GetMinY(),
			Height: payload.ChunkLoaded.GetHeight(),
		}
	case *orchestratorv1.BotObservation_ChunkUnloaded:
		event.Kind = network.EventChunkUnload
		pos := model.ChunkPosition{
			X: payload.ChunkUnloaded.GetChunkX(),
			Z: payload.ChunkUnloaded.GetChunkZ(),
		}

		event.ChunkUnload = &pos
	case *orchestratorv1.BotObservation_BlockUpdated:
		event.Kind = network.EventBlockStateChange
		event.BlockStateChange = &network.BlockStateChange{
			Position: model.BlockPosition{X: payload.BlockUpdated.GetX(), Y: payload.BlockUpdated.GetY(), Z: payload.BlockUpdated.GetZ()},
			StateID:  uint32(payload.BlockUpdated.GetStateId()),
		}
	case *orchestratorv1.BotObservation_MultiBlocksUpdated:
		event.Kind = network.EventMultiBlocksUpdated
		protoRecords := payload.MultiBlocksUpdated.GetRecords()
		records := make([]network.BlockStateChange, len(protoRecords))
		for i, r := range protoRecords {
			records[i] = network.BlockStateChange{
				Position: model.BlockPosition{X: r.GetX(), Y: r.GetY(), Z: r.GetZ()},
				StateID:  uint32(r.GetStateId()),
			}
		}
		event.MultiBlocksUpdated = &network.MultiBlocksUpdated{Records: records}
	case *orchestratorv1.BotObservation_EntitiesChanged:
		event.Kind = network.EventEntityChanges
		event.EntityChanges = entityChanges(payload.EntitiesChanged)
	default:
		return network.Event{}, fmt.Errorf("host observation has no payload")
	}
	return event, nil
}

func hostStatus(status *orchestratorv1.BotStatusChanged) (network.HostStatus, string) {
	if status == nil {
		return network.HostError, "missing status"
	}
	switch status.GetState() {
	case orchestratorv1.BotConnectionState_BOT_CONNECTION_STATE_CONNECTING:
		return network.HostConnecting, status.GetDetail()
	case orchestratorv1.BotConnectionState_BOT_CONNECTION_STATE_CONNECTED:
		return network.HostConnected, status.GetDetail()
	case orchestratorv1.BotConnectionState_BOT_CONNECTION_STATE_DISCONNECTED:
		return network.HostDisconnected, status.GetDetail()
	case orchestratorv1.BotConnectionState_BOT_CONNECTION_STATE_KICKED:
		return network.HostKicked, status.GetDetail()
	default:
		return network.HostError, status.GetDetail()
	}
}

func botState(state *orchestratorv1.HostBotState) *network.HostSnapshot {
	if state == nil {
		return &network.HostSnapshot{}
	}
	p := state.GetPosition()
	return &network.HostSnapshot{
		Vitals:    vitals(state.GetVitals()),
		Position:  position(p),
		Rotation:  rotation(p),
		Velocity:  velocity(p),
		Inventory: inventory(state.GetInventory()),
		Effects:   effects(state.GetEffects()),
		GameMode:  model.GameMode(state.GetGameMode()),
	}
}

func vitals(value *orchestratorv1.HostVitals) network.HostVitals {
	if value == nil {
		return network.HostVitals{}
	}

	return network.HostVitals{
		Health:     value.GetHealth(),
		Food:       value.GetFood(),
		Saturation: value.GetSaturation(),
		Oxygen:     value.GetOxygen(),
	}
}

func position(value *orchestratorv1.HostPosition) model.Position {
	if value == nil {
		return model.Position{}
	}
	return model.Position{X: value.GetX(), Y: value.GetY(), Z: value.GetZ()}
}

func rotation(value *orchestratorv1.HostPosition) model.Rotation {
	if value == nil {
		return model.Rotation{}
	}
	return model.Rotation{Yaw: float32(value.GetYaw()), Pitch: float32(value.GetPitch())}
}

func velocity(value *orchestratorv1.HostPosition) model.Velocity {
	if value == nil {
		return model.Velocity{}
	}
	return model.Velocity{X: value.GetVelocityX(), Y: value.GetVelocityY(), Z: value.GetVelocityZ()}
}

func effects(values []*orchestratorv1.HostPotionEffect) model.Effects {
	result := model.Effects{
		Values: make([]model.Effect, 0, len(values)),
	}

	for _, value := range values {
		if value != nil {
			result.Values = append(
				result.Values,
				model.Effect{
					ID:            value.GetId(),
					Name:          value.GetName(),
					Amplifier:     value.GetAmplifier(),
					DurationTicks: value.GetDurationTicks(),
				},
			)
		}
	}
	return result
}

func inventory(value *orchestratorv1.HostInventory) model.Inventory {
	if value == nil {
		return model.Inventory{}
	}

	result := model.Inventory{
		SelectedHotbarSlot: value.GetSelectedHotbarSlot(),
		Slots:              make([]model.InventorySlot, 0, len(value.GetSlots())),
	}

	for _, slot := range value.GetSlots() {
		if slot == nil {
			continue
		}

		resultSlot := model.InventorySlot{Slot: slot.GetSlot()}
		if item := slot.GetItem(); item != nil {
			resultSlot.Item = &model.ItemStack{
				ID:       item.GetId(),
				Name:     item.GetName(),
				Metadata: item.GetMetadata(),
				Count:    item.GetCount(),
			}
		}

		result.Slots = append(result.Slots, resultSlot)
	}
	return result
}

func entityToGo(e *orchestratorv1.HostEntity) network.Entity {
	return network.Entity{
		ID:       e.GetEntityId(),
		Name:     e.GetName(),
		Position: model.Position{X: e.GetX(), Y: e.GetY(), Z: e.GetZ()},
		Yaw:      e.GetYaw(),
		Pitch:    e.GetPitch(),
	}
}

func entityChanges(payload *orchestratorv1.HostEntitiesChanged) *network.EntityChanges {
	if payload == nil {
		return nil
	}
	added := make([]network.Entity, len(payload.GetAdded()))
	for i, e := range payload.GetAdded() {
		added[i] = entityToGo(e)
	}
	moved := make([]network.Entity, len(payload.GetMoved()))
	for i, e := range payload.GetMoved() {
		moved[i] = entityToGo(e)
	}
	return &network.EntityChanges{
		Added:   added,
		Removed: payload.GetRemoved(),
		Moved:   moved,
	}
}
