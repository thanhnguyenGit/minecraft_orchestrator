package core

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/engine/scheduler"
	"minecraft_orchestrator/internal/mc_protocol/chunk"
)

const (
	SystemBootstrap    scheduler.SystemID = "Bootstrap"
	SystemNetworkApply scheduler.SystemID = "NetworkApply"
	SystemRandomWander scheduler.SystemID = "RandomWander"
)

type TickData struct {
	Bootstrap []model.Bot
	Network   network.Batch
	Outbox    *network.Outbox
}

func tickData(ctx *scheduler.RunContext) (*TickData, error) {
	data, ok := ctx.Data.(*TickData)
	if !ok || data == nil {
		return nil, fmt.Errorf("unexpected tick data %T", ctx.Data)
	}

	return data, nil
}

type BootstrapSystem struct{}

func (BootstrapSystem) ID() scheduler.SystemID {
	return SystemBootstrap
}

func (BootstrapSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{Structural: []model.Mask{model.MirroredBotMask}}
}

func (BootstrapSystem) Run(ctx *scheduler.RunContext) error {
	data, err := tickData(ctx)
	if err != nil {
		return err
	}
	if len(data.Bootstrap) == 0 {
		return nil
	}
	if data.Outbox == nil {
		return fmt.Errorf("bootstrap requires an outbox")
	}

	for _, bot := range data.Bootstrap {
		var bundle enginecore.Bundle
		bundle.Set(model.CBot, bot)
		bundle.Set(model.CSession, model.Session{Phase: model.SessionStopped})
		bundle.Set(model.CPosition, model.Position{})
		bundle.Set(model.CRotation, model.Rotation{})
		bundle.Set(model.CVelocity, model.Velocity{})
		bundle.Set(model.CHealth, model.Health{Current: 20, Max: 20})
		bundle.Set(model.CHunger, model.Hunger{Current: 20, Max: 20})
		bundle.Set(model.CGameMode, model.GameModeSurvival)
		bundle.Set(model.CInventory, model.Inventory{})
		bundle.Set(model.CEffects, model.Effects{})

		ctx.Commands.Stage(enginecore.CreateCommand{Bundle: bundle})
	}
	data.Outbox.Publish(network.Intent{Kind: network.IntentStartHost})
	return nil
}

// NetworkApplySystem is the only system that turns Mineflayer host observations
// into ECS component mutations. The Inbox and host never receive a World pointer.
type NetworkApplySystem struct{}

func (NetworkApplySystem) ID() scheduler.SystemID {
	return SystemNetworkApply
}

func (NetworkApplySystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{
		Queries: []model.Mask{model.MirroredBotMask},
		Writes: model.Components(
			model.CSession,
			model.CPosition,
			model.CRotation,
			model.CVelocity,
			model.CHealth,
			model.CHunger,
			model.CGameMode,
			model.CInventory,
			model.CEffects,
		),
	}
}

func (NetworkApplySystem) Run(ctx *scheduler.RunContext) error {
	data, err := tickData(ctx)
	if err != nil {
		return err
	}

	views := ctx.World.MirroredBotViews()
	for _, event := range data.Network.Events {
		
		applyChunkEvent(ctx.World.Resources(), event, ctx.Logger)
		applyEntityEvent(ctx.World.Resources(), event, ctx.Logger)

		for _, view := range views {
			for index, bot := range view.Bots {
				if bot.ProfileID != event.ProfileID {
					continue
				}

				applyNetworkEvent(&view, index, event, ctx.Logger)
			}
		}
	}

	worldViews := ctx.World.Resources().WorldViews()

	for _, view := range views {
		for i, bot := range view.Bots {
			ctx.Logger.Debug("ecs.state",
				"username", bot.Username,
				"profile_id", fmt.Sprintf("%x", bot.ProfileID),
				"phase", view.Sessions[i].Phase,
				"remote_session", view.Sessions[i].RemoteSessionID,
				"last_seq", view.Sessions[i].LastSequence,
				"health", fmt.Sprintf("%.1f/%.1f", view.Healths[i].Current, view.Healths[i].Max),
				"pos", fmt.Sprintf("(%.1f, %.1f, %.1f)", view.Positions[i].X, view.Positions[i].Y, view.Positions[i].Z),
				"yaw", fmt.Sprintf("%.1f", view.Rotations[i].Yaw),
				"pitch", fmt.Sprintf("%.1f", view.Rotations[i].Pitch),
				"vel", fmt.Sprintf("(%.3f, %.3f, %.3f)", view.Velocitys[i].X, view.Velocitys[i].Y, view.Velocitys[i].Z),
				"game_mode", view.GameModes[i],
				"effects", len(view.Effectss[i].Values),
				"inv_slots", len(view.Inventorys[i].Slots),
				"inv_hotbar", view.Inventorys[i].SelectedHotbarSlot,
			)

			if worldView, ok := worldViews.Get(bot.ProfileID); ok {
				chunks := len(worldView.GetChunks())
				for cpos, ccol := range worldView.GetChunks() {
					ctx.Logger.Debug("ecs.world",
						"username", bot.Username,
						"profile_id", fmt.Sprintf("%x", bot.ProfileID),
						"chunk_position", cpos,
						"chunk", ccol,
						"chunk_counts", chunks,
					)
				}
			}
		}
	}

	return nil
}

func applyNetworkEvent(
	view *enginecore.MirroredBotView,
	index int,
	event network.Event,
	logger *slog.Logger,
) {
	switch event.Kind {
	case network.EventHostStatus:
		oldPhase := view.Sessions[index].Phase
		if !applyHostStatus(view, index, event) {
			logger.Debug("ecs.status_drop")
			break
		}
		newPhase := view.Sessions[index].Phase
		if newPhase != oldPhase {
			logger.Debug("ecs.status",
				"phase_from", oldPhase,
				"phase_to", newPhase,
			)
		}
	case network.EventHostSnapshot:
		if event.RemoteSessionID == "" {
			logger.Debug(
				"ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "empty_session",
			)
			break
		}
		lastSeq := view.Sessions[index].LastSequence
		if !acceptHostObservation(view, index, event) {
			logger.Debug(
				"ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "stale_sequence",
				"last_seq", lastSeq,
			)
			break
		}
		if event.Snapshot != nil {
			logger.Debug(
				"ecs.apply",
				"kind", event.Kind,
				"event", event,
			)
			applyHostSnapshot(view, index, *event.Snapshot)
		}
	case network.EventHostPosition:
		if event.RemoteSessionID == "" {
			logger.Debug(
				"ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "empty_session",
			)
			break
		}
		lastSeq := view.Sessions[index].LastSequence
		if !acceptHostObservation(view, index, event) {
			logger.Debug("ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "stale_sequence",
				"last_seq", lastSeq,
			)
			break
		}
		if event.Position != nil {
			logger.Debug(
				"ecs.apply",
				"kind", event.Kind,
				"event", event,
			)
			view.Positions[index] = event.Position.Position
			view.Rotations[index] = event.Position.Rotation
			view.Velocitys[index] = event.Position.Velocity
		}
	case network.EventHostVitals:
		if event.RemoteSessionID == "" {
			logger.Debug(
				"ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "empty_session",
			)
			break
		}
		lastSeq := view.Sessions[index].LastSequence
		if !acceptHostObservation(view, index, event) {
			logger.Debug("ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "stale_sequence",
				"last_seq", lastSeq,
			)
			break
		}
		if event.Vitals != nil {
			logger.Debug(
				"ecs.apply",
				"kind", event.Kind,
				"event", event,
			)
			health := view.Healths[index]
			health.Current = event.Vitals.Health
			view.Healths[index] = health
			view.Hungers[index] = model.Hunger{Current: float64(event.Vitals.Food), Max: 20}
		}
	case network.EventHostEffects:
		if event.RemoteSessionID == "" {
			logger.Debug(
				"ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "empty_session",
			)
			break
		}
		lastSeq := view.Sessions[index].LastSequence
		if !acceptHostObservation(view, index, event) {
			logger.Debug("ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "stale_sequence",
				"last_seq", lastSeq,
			)
			break
		}
		if event.Effects != nil {
			logger.Debug(
				"ecs.apply",
				"kind", event.Kind,
				"event", event,
			)
			view.Effectss[index] = *event.Effects
		}
	case network.EventHostInventory:
		if event.RemoteSessionID == "" {
			logger.Debug(
				"ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "empty_session",
			)
			break
		}
		lastSeq := view.Sessions[index].LastSequence
		if !acceptHostObservation(view, index, event) {
			logger.Debug("ecs.apply_drop",
				"kind", event.Kind,
				"event", event,
				"reason", "stale_sequence",
				"last_seq", lastSeq,
			)
			break
		}
		if event.Inventory != nil {
			logger.Debug(
				"ecs.apply",
				"kind", event.Kind,
				"event", event,
			)
			view.Inventorys[index] = *event.Inventory
		}
	}
}

func acceptHostObservation(view *enginecore.MirroredBotView, index int, event network.Event) bool {
	if event.RemoteSessionID == "" {
		return false
	}

	session := view.Sessions[index]
	if session.RemoteSessionID != event.RemoteSessionID {
		session.RemoteSessionID = event.RemoteSessionID
		session.LastSequence = 0
	}

	if event.Sequence <= session.LastSequence {
		return false
	}

	session.LastSequence = event.Sequence
	if session.Phase != model.SessionPlayReady {
		session.Phase = model.SessionPlayReady
	}

	view.Sessions[index] = session
	return true
}

func applyHostStatus(view *enginecore.MirroredBotView, index int, event network.Event) bool {
	session := view.Sessions[index]
	if event.RemoteSessionID != "" && session.RemoteSessionID != event.RemoteSessionID {
		session.RemoteSessionID = event.RemoteSessionID
		session.LastSequence = 0
	}

	if event.Sequence <= session.LastSequence {
		return false
	}

	session.LastSequence = event.Sequence
	switch event.HostStatus {
	case network.HostConnecting:
		session.Phase = model.SessionConnecting
	case network.HostConnected:
		session.Phase = model.SessionPlayReady
	case network.HostDisconnected:
		session.Phase = model.SessionRetryWaiting
	default:
		session.Phase = model.SessionRetryWaiting
		session.Failure = event.Failure
	}

	view.Sessions[index] = session
	return true
}

func applyHostSnapshot(view *enginecore.MirroredBotView, index int, snapshot network.HostSnapshot) {
	view.Positions[index], view.Rotations[index], view.Velocitys[index] = snapshot.Position, snapshot.Rotation, snapshot.Velocity

	health := &view.Healths[index]
	health.Current = snapshot.Vitals.Health
	view.Hungers[index] = model.Hunger{Current: float64(snapshot.Vitals.Food), Max: 20}

	view.GameModes[index], view.Inventorys[index], view.Effectss[index] = snapshot.GameMode, snapshot.Inventory, snapshot.Effects
}

func applyChunkEvent(resource *enginecore.Resources, event network.Event, logger *slog.Logger) {
	views := resource.WorldViews()

	perception, hasPerception := views.Get(event.ProfileID)

	switch event.Kind {
	case network.EventChunkLoad:
		if event.ChunkLoad == nil {
			return
		}

		logger.Debug(
			"ecs.chunk_load",
			"kind", event.Kind.String(),
			"position", event.ChunkLoad.Position,
			"data_length", len(event.ChunkLoad.Data),
			"min_y", event.ChunkLoad.MinY,
			"height", event.ChunkLoad.Height,
		)

		if !hasPerception || perception.ActiveDimensionType.MinY != event.ChunkLoad.MinY || perception.ActiveDimensionType.Height != event.ChunkLoad.Height {
			logger.Debug(
				"ecs.chunk_perception_init",
				"has_perception", hasPerception,
				"current_min_y", perception.ActiveDimensionType.MinY,
				"event_min_y", event.ChunkLoad.MinY,
				"current_height", perception.ActiveDimensionType.Height,
				"event_height", event.ChunkLoad.Height,
			)
			attemptID := perception.AttemptID + 1
			views.BeginAttempt(event.ProfileID, attemptID)
			dimType := model.DimensionType{
				RegistryID: 0,
				Key:        "minecraft:overworld",
				MinY:       event.ChunkLoad.MinY,
				Height:     event.ChunkLoad.Height,
			}
			views.SetDimensionTypes(event.ProfileID, attemptID, []model.DimensionType{dimType})
			views.BindDimension(event.ProfileID, attemptID, 0)

			perception, hasPerception = views.Get(event.ProfileID)
		}

		if !hasPerception || !perception.HasActiveDimensionType {
			logger.Debug("ecs.chunk_drop", "reason", "no_active_dimension")
			return
		}

		column, err := chunk.DecodeColumn(event.ChunkLoad.Data, perception.ActiveDimensionType)
		if err != nil {
			logger.Debug("ecs.chunk_decode_error", "error", err)
			return
		}

		views.ReplaceChunk(
			event.ProfileID,
			perception.AttemptID,
			event.ChunkLoad.Position,
			column,
		)

		chunks, ok := views.Get(event.ProfileID)
		if !ok {
			return
		}

		logger.Debug("ecs.chunk_load", "stored_chunks", len(chunks.GetChunks()))
	case network.EventChunkUnload:
		if event.ChunkUnload == nil {
			return
		}

		chunks, ok := resource.WorldViews().Get(event.ProfileID)
		if !ok {
			return
		}

		preChunkCount := len(chunks.GetChunks())

		attemptID := uint64(0)
		if hasPerception {
			attemptID = perception.AttemptID
		}

		views.UnloadChunk(event.ProfileID, attemptID, *event.ChunkUnload)

		currentChunkCount := len(chunks.GetChunks())

		if currentChunkCount != preChunkCount {
			logger.Debug(
				"ecs.chunk_unload",
				"kind", event.Kind.String(),
				"chunk", event.ChunkUnload,
				"previous_chunk_count", preChunkCount,
				"current_chunk_count", currentChunkCount,
			)
		}
	case network.EventBlockStateChange:
		if event.BlockStateChange == nil {
			return
		}
		logger.Debug(
			"ecs.chunk_block_change",
			"kind", event.Kind.String(),
			"position", event.BlockStateChange.Position,
			"state_id", event.BlockStateChange.StateID,
		)

		attemptID := uint64(0)
		if hasPerception {
			attemptID = perception.AttemptID
		}

		views.SetBlockState(
			event.ProfileID,
			attemptID, event.BlockStateChange.Position,
			event.BlockStateChange.StateID,
		)
	case network.EventMultiBlocksUpdated:
		if event.MultiBlocksUpdated == nil {
			return
		}

		logger.Debug(
			"ecs.chunk_multi_block_change",
			"kind", event.Kind.String(),
			"records", len(event.MultiBlocksUpdated.Records),
		)

		attemptID := uint64(0)
		if hasPerception {
			attemptID = perception.AttemptID
		}

		updates := make([]model.BlockUpdate, len(event.MultiBlocksUpdated.Records))
		for i, r := range event.MultiBlocksUpdated.Records {
			updates[i] = model.BlockUpdate{
				Position: r.Position,
				StateID:  r.StateID,
			}
		}

		views.SetBlockStates(event.ProfileID, attemptID, updates)
	}
}

// TODO: delete RandomWanderSystem after movement tests.
// RandomWanderSystem generates random control sequences for bots, using chunk
// data to avoid walking into walls, lava, fire, water, or off cliffs. It is
// temporary test scaffolding and will be replaced by a real decision system.
type RandomWanderSystem struct {
	states map[model.ProfileID]*wanderState
}

type wanderState struct {
	nextActionAt time.Time
	sequence     uint64
}

func (RandomWanderSystem) ID() scheduler.SystemID {
	return SystemRandomWander
}

func (RandomWanderSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{
		Queries: []model.Mask{model.MirroredBotMask},
	}
}

func (s *RandomWanderSystem) Run(ctx *scheduler.RunContext) error {
	data, err := tickData(ctx)
	if err != nil {
		return err
	}
	if data.Outbox == nil {
		return nil
	}

	if s.states == nil {
		s.states = make(map[model.ProfileID]*wanderState)
	}

	now := time.Now()
	worldViews := ctx.World.Resources().WorldViews()
	views := ctx.World.MirroredBotViews()

	for _, view := range views {
		for index, bot := range view.Bots {
			if view.Sessions[index].Phase != model.SessionPlayReady {
				continue
			}

			ws := s.states[bot.ProfileID]
			if ws == nil {
				ws = &wanderState{}
				s.states[bot.ProfileID] = ws
			}

			if now.Before(ws.nextActionAt) {
				continue
			}

			pos := view.Positions[index]

			wv, ok := worldViews.Get(bot.ProfileID)
			if !ok || !wv.HasActiveDimensionType {
				continue
			}

			dest, ok := s.pickDestination(bot.ProfileID, pos, worldViews)
			if !ok {
				continue
			}

			ws.sequence++

			data.Outbox.Publish(network.Intent{
				ProfileID: bot.ProfileID,
				Kind:      network.IntentCommand,
				Command: &network.GotoCommand{
					Sequence: ws.sequence,
					X:        dest.X,
					Y:        dest.Y,
					Z:        dest.Z,
				},
			})
			
			// jitter := time.Duration(5000+rand.IntN(10000)) * time.Millisecond
			ws.nextActionAt = now.Add(2*time.Second)
		}
	}

	return nil
}

func (s *RandomWanderSystem) pickDestination(profileID model.ProfileID, pos model.Position, wv *model.WorldViews) (model.BlockPosition, bool) {
	radius := int32(5 + rand.IntN(16))
	originX := int32(pos.X)
	originY := int32(pos.Y)
	originZ := int32(pos.Z)

	for range 30 {
		dx := int32(rand.IntN(int(radius*2+1))) - radius
		dz := int32(rand.IntN(int(radius*2+1))) - radius

		for dy := int32(-3); dy <= 3; dy++ {
			bx := originX + dx
			by := originY + dy
			bz := originZ + dz

			if canStandAt(profileID, bx, by, bz, wv) {
				return model.BlockPosition{X: bx, Y: by, Z: bz}, true
			}
		}
	}

	return model.BlockPosition{}, false
}

func canStandAt(profileID model.ProfileID, bx, by, bz int32, wv *model.WorldViews) bool {
	feetID, ok := wv.BlockState(profileID, model.BlockPosition{X: bx, Y: by, Z: bz})
	if !ok || feetID != 0 {
		return false
	}

	headID, ok := wv.BlockState(profileID, model.BlockPosition{X: bx, Y: by + 1, Z: bz})
	if !ok || headID != 0 {
		return false
	}

	groundID, ok := wv.BlockState(profileID, model.BlockPosition{X: bx, Y: by - 1, Z: bz})
	if !ok || groundID == 0 || isHazard(groundID) {
		return false
	}
	return true
}

func isHazard(stateID uint32) bool {
	return (stateID >= 86 && stateID <= 117) || // water + lava
		(stateID >= 3174 && stateID <= 3685) || // fire
		(stateID >= 6728 && stateID <= 6743) || // cactus
		stateID == 14643 || // magma_block
		(stateID >= 20675 && stateID <= 20742) || // campfires + sweet_berry_bush
		stateID == 24487 // powder_snow
}

func applyEntityEvent(resource *enginecore.Resources, event network.Event, logger *slog.Logger) {
	if event.Kind != network.EventEntityChanges || event.EntityChanges == nil {
		return
	}

	
	entities := resource.EntityViews()
	changes := event.EntityChanges
	
	logger.Debug(
		"ecs.entity",
		"entities", entities,
		"raw", changes,
	)

	if len(changes.Added) > 0 {
		entities.AddEntities(event.ProfileID, toModelEntities(changes.Added))
	}
	
	if len(changes.Removed) > 0 {
		entities.RemoveEntities(event.ProfileID, changes.Removed)
	}
	
	if len(changes.Moved) > 0 {
		entities.MoveEntities(event.ProfileID, toModelEntities(changes.Moved))
	}

	logger.Debug("ecs.entity_changes",
		"added", len(changes.Added),
		"removed", len(changes.Removed),
		"moved", len(changes.Moved),
	)
}

func toModelEntities(ents []network.Entity) []model.Entity {
	out := make([]model.Entity, len(ents))
	for i, e := range ents {
		out[i] = model.Entity{
			ID:       e.ID,
			Name:     e.Name,
			Position: e.Position,
			Yaw:      e.Yaw,
			Pitch:    e.Pitch,
		}
	}
	return out
}
