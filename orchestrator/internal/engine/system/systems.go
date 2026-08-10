package core

import (
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sort"
	"strings"

	"minecraft_orchestrator/internal/engine/ai"
	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/engine/scheduler"
	"minecraft_orchestrator/internal/mc_protocol/chunk"
	"minecraft_orchestrator/internal/mc_protocol/registry"
)

const (
	SystemBootstrap    scheduler.SystemID = "Bootstrap"
	SystemNetworkApply scheduler.SystemID = "NetworkApply"
	SystemPerception   scheduler.SystemID = "Perception"
	SystemGoalSelector scheduler.SystemID = "GoalSelector"
	SystemNeeds        scheduler.SystemID = "Needs"
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
		bundle.Set(model.CUtilityAI, model.UtilityAIState{})
		bundle.Set(model.CControllerSync, model.ControllerSyncState{})

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
			model.CUtilityAI,
			model.CControllerSync,
		),
	}
}

func (NetworkApplySystem) Run(ctx *scheduler.RunContext) error {
	debugEnabled := ctx.Logger.Enabled(ctx.Context, slog.LevelDebug)

	data, err := tickData(ctx)
	if err != nil {
		return err
	}

	views := ctx.World.MirroredBotViews()
	for _, event := range data.Network.Events {
		if event.Kind != network.EventRealityState {
			for _, view := range views {
				for index, bot := range view.Bots {
					if bot.ProfileID == event.ProfileID && beginsNewRemoteSession(view.Sessions[index], event) {
						ctx.World.Resources().RealityView().Clear(event.ProfileID)
					}
				}
			}
		}

		applyChunkEvent(ctx.World.Resources(), event, ctx.Logger, debugEnabled)
		applyEntityEvent(ctx.World.Resources(), event, ctx.Logger)
		applyRealityEvent(ctx.World.Resources(), views, event, ctx.Logger)

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

	if debugEnabled {
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
					chunks := worldView.ChunksCount()
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
		resetSessionAI(view, index)
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

func beginsNewRemoteSession(session model.Session, event network.Event) bool {
	return event.RemoteSessionID != "" && session.RemoteSessionID != "" && session.RemoteSessionID != event.RemoteSessionID
}

func applyHostStatus(view *enginecore.MirroredBotView, index int, event network.Event) bool {
	session := view.Sessions[index]
	if event.RemoteSessionID != "" && session.RemoteSessionID != event.RemoteSessionID {
		session.RemoteSessionID = event.RemoteSessionID
		session.LastSequence = 0
		resetSessionAI(view, index)
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

func resetSessionAI(view *enginecore.MirroredBotView, index int) {
	view.UtilityAIs[index] = model.UtilityAIState{}
	view.ControllerSyncs[index] = model.ControllerSyncState{}
}

func applyHostSnapshot(view *enginecore.MirroredBotView, index int, snapshot network.HostSnapshot) {
	view.Positions[index], view.Rotations[index], view.Velocitys[index] = snapshot.Position, snapshot.Rotation, snapshot.Velocity

	health := &view.Healths[index]
	health.Current = snapshot.Vitals.Health
	view.Hungers[index] = model.Hunger{Current: float64(snapshot.Vitals.Food), Max: 20}

	view.GameModes[index], view.Inventorys[index], view.Effectss[index] = snapshot.GameMode, snapshot.Inventory, snapshot.Effects
}

func applyChunkEvent(
	resource *enginecore.Resources,
	event network.Event,
	logger *slog.Logger,
	debugEnabled bool,
) {
	views := resource.WorldViews()

	perception, hasPerception := views.Get(event.ProfileID)

	switch event.Kind {
	case network.EventChunkLoad:
		if event.ChunkLoad == nil {
			return
		}

		if debugEnabled {
			logger.Debug(
				"ecs.chunk_load",
				"kind", event.Kind.String(),
				"position", event.ChunkLoad.Position,
				"data_length", len(event.ChunkLoad.Data),
				"min_y", event.ChunkLoad.MinY,
				"height", event.ChunkLoad.Height,
			)
		}

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
			if debugEnabled {
				logger.Debug("ecs.chunk_drop", "reason", "no_active_dimension")
			}
			return
		}

		column, err := chunk.DecodeColumn(event.ChunkLoad.Data, perception.ActiveDimensionType)
		if err != nil {
			if debugEnabled {
				logger.Debug("ecs.chunk_decode_error", "error", err)
			}
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

		logger.Debug("ecs.chunk_load", "stored_chunks", chunks.ChunksCount())
	case network.EventChunkUnload:
		if event.ChunkUnload == nil {
			return
		}

		chunks, ok := resource.WorldViews().Get(event.ProfileID)
		if !ok {
			return
		}

		preChunkCount := chunks.ChunksCount()

		attemptID := uint64(0)
		if hasPerception {
			attemptID = perception.AttemptID
		}

		views.UnloadChunk(event.ProfileID, attemptID, *event.ChunkUnload)

		currentChunkCount := chunks.ChunksCount()

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

// pickDestination is shared by the utility controller's idle and world-fact
// evaluation. It only selects a target; ControllerState is the sole path that
// can send that target to the host.
func pickDestination(profileID model.ProfileID, pos model.Position, wv *model.WorldViews) (model.BlockPosition, bool) {
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

func applyRealityEvent(resource *enginecore.Resources, views []enginecore.MirroredBotView, event network.Event, logger *slog.Logger) {
	if event.Kind != network.EventRealityState || event.RealityState == nil {
		return
	}
	matchedSession := false
	for _, view := range views {
		for index, bot := range view.Bots {
			if bot.ProfileID == event.ProfileID && event.RemoteSessionID != "" && view.Sessions[index].RemoteSessionID == event.RemoteSessionID {
				matchedSession = true
			}
		}
	}
	if !matchedSession {
		logger.Debug("reality.drop", "profile_id", fmt.Sprintf("%x", event.ProfileID), "session_id", event.RemoteSessionID, "reason", "session_mismatch")
		return
	}

	rs := event.RealityState
	logger.Debug("reality.received",
		"profile_id", fmt.Sprintf("%x", event.ProfileID),
		"arrival_distance", fmt.Sprintf("%v", rs.ArrivalDistance),
		"digging_block", fmt.Sprintf("%v", rs.DiggingBlock),
		"attacking_entity", fmt.Sprintf("%v", rs.AttackingEntity),
		"equipped_item", fmt.Sprintf("%v", rs.EquippedItem),
		"goto_target", fmt.Sprintf("%v", rs.GotoTarget),
	)
	state := model.RealityState{
		ArrivalDistance:          rs.ArrivalDistance,
		DiggingBlock:             rs.DiggingBlock,
		AttackingEntity:          rs.AttackingEntity,
		EquippedItem:             rs.EquippedItem,
		GotoTarget:               rs.GotoTarget,
		ActionOutcomes:           append([]model.ActionOutcome(nil), rs.ActionOutcomes...),
		ActionFailed:             rs.ActionFailed,
		Failure:                  rs.Failure,
		ActionFailureCorrelation: rs.ActionFailureCorrelation,
	}
	resource.RealityView().Set(event.ProfileID, state)
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

type PerceptionSystem struct {
	fov ai.FOVStrategy
}

func NewPerceptionSystem(fov ai.FOVStrategy) *PerceptionSystem {
	if fov == nil {
		fov = ai.ConeFOV{}
	}
	return &PerceptionSystem{fov: fov}
}

func (s PerceptionSystem) ID() scheduler.SystemID {
	return SystemPerception
}

func (PerceptionSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{
		Queries: []model.Mask{model.MirroredBotMask},
	}
}

func (s *PerceptionSystem) Run(ctx *scheduler.RunContext) error {
	entityViews := ctx.World.Resources().EntityViews()
	perceptionView := ctx.World.Resources().PerceptionView()
	perceptionBlockView := ctx.World.Resources().PerceptionBlockView()
	worldViews := ctx.World.Resources().WorldViews()
	views := ctx.World.MirroredBotViews()

	for _, view := range views {
		for index, session := range view.Sessions {
			if session.Phase != model.SessionPlayReady {
				continue
			}

			bot := view.Bots[index]
			pos := view.Positions[index]
			rot := view.Rotations[index]

			nearby := entityViews.GetNearby(bot.ProfileID, pos, defaultPerceptionRadius)
			visible := make([]model.PerceivedEntity, 0, len(nearby))

			for _, entity := range nearby {
				if !s.fov.IsInFOV(pos, rot, defaultFOV, entity.Position) {
					continue
				}

				dx := entity.Position.X - pos.X
				dy := entity.Position.Y - pos.Y
				dz := entity.Position.Z - pos.Z
				distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

				yawRad := float64(rot.Yaw)
				entityAngle := math.Atan2(-dx, dz)
				angle := math.Mod(entityAngle-yawRad+3*math.Pi, 2*math.Pi) - math.Pi

				visible = append(visible, model.PerceivedEntity{
					ID:       entity.ID,
					Name:     entity.Name,
					Position: entity.Position,
					Distance: distance,
					Angle:    angle,
				})
			}

			perceptionView.Set(bot.ProfileID, visible)
			visibleHostiles := make([]model.PerceivedEntity, 0, len(visible))
			nearestHostileDistance := -1.0
			for _, entity := range visible {
				if !ai.IsHostile(entity.Name) {
					continue
				}
				visibleHostiles = append(visibleHostiles, entity)
				if nearestHostileDistance < 0 || entity.Distance < nearestHostileDistance {
					nearestHostileDistance = entity.Distance
				}
			}
			ctx.Logger.Info("perception.entities_fov",
				"profile_id", fmt.Sprintf("%x", bot.ProfileID),
				"username", bot.Username,
				"nearby_entities", len(nearby),
				"visible_entities", len(visible),
				"fov_rejected_entities", len(nearby)-len(visible),
				"visible_hostiles", len(visibleHostiles),
				"nearest_hostile_distance", nearestHostileDistance,
				"threat", ai.CalcThreat(visibleHostiles),
			)

			visibleBlocks, ctr := s.scanBlocksInFOV(bot.ProfileID, pos, rot, worldViews, view.Inventorys[index])
			perceptionBlockView.Set(bot.ProfileID, visibleBlocks)

			ctx.Logger.Info("perception.blocks_fov",
				"profile_id", fmt.Sprintf("%x", bot.ProfileID),
				"username", bot.Username,
				"resource_candidates", ctr.resourceCandidates,
				"exposed_resource_candidates", ctr.exposedResourceCandidates,
				"buried_resource_candidates", ctr.buriedResourceCandidates,
				"occluded_resource_candidates", ctr.occludedResourceCandidates,
				"non_mineable_resource_candidates", ctr.nonMineableResourceCandidates,
				"visible_mineable_resources", ctr.visibleMineableResources,
				"nearest_resource_candidate", resourceCandidateName(ctr.nearestResourceCandidate),
				"nearest_resource_candidate_distance", resourceCandidateDistance(ctr.nearestResourceCandidate),
			)
		}
	}

	return nil
}

type fovBlockCounters struct {
	resourceCandidates            int
	exposedResourceCandidates     int
	buriedResourceCandidates      int
	occludedResourceCandidates    int
	nonMineableResourceCandidates int
	visibleMineableResources      int
	nearestResourceCandidate      *model.PerceptionBlock
}

func (s *PerceptionSystem) scanBlocksInFOV(
	profileID model.ProfileID,
	pos model.Position,
	rot model.Rotation,
	wv *model.WorldViews,
	inv model.Inventory,
) ([]model.PerceptionBlock, fovBlockCounters) {
	const radius = int32(15)
	const verticalReach = int32(4)
	originX := int32(pos.X)
	originY := int32(pos.Y)
	originZ := int32(pos.Z)

	neighbors := [6]model.BlockPosition{
		{X: 1, Y: 0, Z: 0},
		{X: -1, Y: 0, Z: 0},
		{X: 0, Y: 1, Z: 0},
		{X: 0, Y: -1, Z: 0},
		{X: 0, Y: 0, Z: 1},
		{X: 0, Y: 0, Z: -1},
	}

	var results []model.PerceptionBlock
	ctr := fovBlockCounters{}

	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			for dy := -verticalReach; dy <= verticalReach; dy++ {
				bx := originX + dx
				by := originY + dy
				bz := originZ + dz

				blockPos := model.BlockPosition{X: bx, Y: by, Z: bz}
				blockCenter := model.Position{
					X: float64(bx) + 0.5,
					Y: float64(by) + 0.5,
					Z: float64(bz) + 0.5,
				}

				if !s.fov.IsInFOV(pos, rot, defaultFOV, blockCenter) {
					continue
				}

				stateID, ok := wv.BlockState(profileID, blockPos)
				if !ok || stateID == 0 {
					continue
				}

				name, ok := registry.BlockName(stateID)
				if !ok || !ai.IsResource(name) {
					continue
				}

				ctr.resourceCandidates++
				candidate := model.PerceptionBlock{Position: blockPos, Name: name}
				fdx := float64(dx)
				fdy := float64(dy)
				fdz := float64(dz)
				candidate.Distance = math.Sqrt(fdx*fdx + fdy*fdy + fdz*fdz)
				if ctr.nearestResourceCandidate == nil || candidate.Distance < ctr.nearestResourceCandidate.Distance {
					ctr.nearestResourceCandidate = &candidate
				}

				hasExposed := false
				visibleFace := model.BlockPosition{}
				eye := model.Position{
					X: pos.X,
					Y: pos.Y + 1.62,
					Z: pos.Z,
				}

				for _, n := range neighbors {
					nx := bx + n.X
					ny := by + n.Y
					nz := bz + n.Z
					sid, ok := wv.BlockState(profileID, model.BlockPosition{
						X: nx,
						Y: ny,
						Z: nz,
					})

					if ok && sid == 0 {
						hasExposed = true
						facePoint := model.Position{
							X: float64(bx) + 0.5 + float64(n.X)*0.501,
							Y: float64(by) + 0.5 + float64(n.Y)*0.501,
							Z: float64(bz) + 0.5 + float64(n.Z)*0.501,
						}

						dirX := facePoint.X - eye.X
						dirY := facePoint.Y - eye.Y
						dirZ := facePoint.Z - eye.Z

						dot := (dirX * float64(n.X)) + (dirY * float64(n.Y)) + (dirZ * float64(n.Z))

						if dot >= 0 {
							continue
						}

						if ai.HasClearLineOfSight(eye, facePoint, blockPos, func(p model.BlockPosition) (uint32, bool) {
							return wv.BlockState(profileID, p)
						}) {
							visibleFace = n
							break
						}
					}
				}
				if !hasExposed {
					ctr.buriedResourceCandidates++
					continue
				}
				ctr.exposedResourceCandidates++
				if visibleFace == (model.BlockPosition{}) {
					ctr.occludedResourceCandidates++
					continue
				}

				if !ai.CanMine(name, inv) {
					ctr.nonMineableResourceCandidates++
					continue
				}

				dist := candidate.Distance

				yawRad := float64(rot.Yaw)
				blockAngle := math.Atan2(float64(-dx), float64(dz))
				angle := math.Mod(blockAngle-yawRad+3*math.Pi, 2*math.Pi) - math.Pi

				results = append(results, model.PerceptionBlock{
					Position:    blockPos,
					Name:        name,
					Distance:    dist,
					Angle:       angle,
					VisibleFace: visibleFace,
				})
			}
		}
	}

	ctr.visibleMineableResources = len(results)

	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	return results, ctr
}

const (
	defaultPerceptionRadius = 32.0
	defaultFOV              = 120.0
)

// GoalSelectorSystem reconciles stateless utility behaviors with the per-bot
// lifecycle and controller delivery state stored in ECS columns.
type GoalSelectorSystem struct{}

func (GoalSelectorSystem) ID() scheduler.SystemID {
	return SystemGoalSelector
}

func (GoalSelectorSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{
		Queries: []model.Mask{model.MirroredBotMask},
		Writes:  model.Components(model.CUtilityAI, model.CControllerSync),
	}
}

func (s *GoalSelectorSystem) Run(ctx *scheduler.RunContext) error {
	data, err := tickData(ctx)
	if err != nil {
		return err
	}
	perceptionView := ctx.World.Resources().PerceptionView()
	realityView := ctx.World.Resources().RealityView()
	perceptionBlockView := ctx.World.Resources().PerceptionBlockView()
	views := ctx.World.MirroredBotViews()
	worldViews := ctx.World.Resources().WorldViews()
	runner := ai.NewBehaviorRunner(ai.NewBehaviorCatalog())

	for _, view := range views {
		for i, bot := range view.Bots {
			perceived := perceptionView.Get(bot.ProfileID)
			perceivedBlocks := perceptionBlockView.Get(bot.ProfileID)
			utility := view.UtilityAIs[i]
			controller := view.ControllerSyncs[i]
			reality := realityForProfile(realityView, bot.ProfileID)
			feedbackReason := reconcileOneShotFeedback(&utility, &controller, reality)
			completedOneShot := feedbackReason == model.GoalExitFailed || feedbackReason == model.GoalExitCompleted
			tickContext := model.NewTickContext(model.TickContextInput{
				Tick:      ctx.Tick,
				Bot:       bot,
				Session:   view.Sessions[i],
				Position:  view.Positions[i],
				Rotation:  view.Rotations[i],
				Health:    view.Healths[i],
				Hunger:    view.Hungers[i],
				Inventory: view.Inventorys[i],
				Entities:  perceived,
				Blocks:    perceivedBlocks,
				Reality:   reality,
				World:     goalWorldFacts(bot.ProfileID, view.Positions[i], worldViews),
			})
			trace := ai.EvaluateUtilityTrace(tickContext, utility)
			if view.Sessions[i].Phase != model.SessionPlayReady {
				emitUtilityTick(ctx.Logger, ctx.Tick, bot, view.Sessions[i], view.Positions[i], view.Rotations[i], view.Healths[i], view.Hungers[i], view.Inventorys[i], trace, ai.BehaviorDecision{}, controller, nil, 0, "session_not_play_ready", utility.CurrentGoal, utility.CurrentGoal)
				continue
			}
			if data.Outbox == nil {
				emitUtilityTick(ctx.Logger, ctx.Tick, bot, view.Sessions[i], view.Positions[i], view.Rotations[i], view.Healths[i], view.Hungers[i], view.Inventorys[i], trace, ai.BehaviorDecision{}, controller, nil, 0, "no_outbox", utility.CurrentGoal, utility.CurrentGoal)
				continue
			}

			previousGoal := utility.CurrentGoal
			result := runner.ReconcileWithFeedback(tickContext, utility, feedbackReason)
			trace = result.Trace

			desired := controllerStateForDecision(result.Decision)
			if completedOneShot && isOneShotControllerAction(result.Decision.Action) {
				desired = model.EmptyControllerState()
			} else if isOneShotControllerAction(result.Decision.Action) && controller.InFlightOneShot.Action != model.ControllerActionNone {
				// One-shot delivery is correlated one at a time; keep its desired
				// field while utility selection and sustained controls continue.
				desired = controller.Desired.Clone()
			}
			desired = model.ValidateControllerState(desired)
			controller.Desired = desired.Clone()

			delta := model.DiffControllerState(controller.LastSent, controller.Desired)
			publishedSequence := uint64(0)
			if delta != nil {
				controller.ControllerSequence++
				publishedSequence = controller.ControllerSequence
				if !completedOneShot && isOneShotControllerAction(result.Decision.Action) && controller.InFlightOneShot.Action == model.ControllerActionNone {
					action := result.Decision.Action
					controller.ActionSequences[action] = controller.ControllerSequence
					controller.InFlightOneShot = model.InFlightOneShot{Action: action, Correlation: controller.ControllerSequence, Goal: result.State.CurrentGoal, Target: result.Decision.Target, PreconditionsHash: ai.DecisionPreconditionsHash(tickContext, result.Decision), CraftCount: result.Decision.CraftCount, PlaceItem: result.Decision.PlaceItem}
				}
				data.Outbox.Publish(network.Intent{
					ProfileID:       bot.ProfileID,
					Kind:            network.IntentControllerState,
					ControllerState: convertControllerState(delta, controller.ControllerSequence),
				})
				controller.LastSent = controller.Desired.Clone()
			}
			noEmitReason := ""
			if delta == nil {
				noEmitReason = noControllerEmissionReason(trace, result)
			}
			emitUtilityTick(ctx.Logger, ctx.Tick, bot, view.Sessions[i], view.Positions[i], view.Rotations[i], view.Healths[i], view.Hungers[i], view.Inventorys[i], trace, result.Decision, controller, delta, publishedSequence, noEmitReason, previousGoal, result.State.CurrentGoal)

			view.UtilityAIs[i] = result.State
			view.ControllerSyncs[i] = controller
		}
	}

	return nil
}

func emitUtilityTick(logger *slog.Logger, tick uint64, bot model.Bot, session model.Session, position model.Position, rotation model.Rotation, health model.Health, hunger model.Hunger, inventory model.Inventory, trace ai.UtilityTraceSnapshot, decision ai.BehaviorDecision, controller model.ControllerSyncState, delta *model.ControllerStateDelta, publishedSequence uint64, noEmitReason string, previousGoal, selectedGoal model.GoalType) {
	attrs := []any{
		"tick", tick,
		"profile_id", fmt.Sprintf("%x", bot.ProfileID),
		"session_id", session.RemoteSessionID,
		"position", slog.GroupValue(slog.Float64("x", position.X), slog.Float64("y", position.Y), slog.Float64("z", position.Z)),
		"rotation", slog.GroupValue(slog.Float64("yaw", float64(rotation.Yaw)), slog.Float64("pitch", float64(rotation.Pitch))),
		"vitals", slog.GroupValue(slog.Float64("health", health.Current), slog.Float64("health_max", health.Max), slog.Float64("hunger", hunger.Current), slog.Float64("hunger_max", hunger.Max)),
		"inventory_progress", inventoryProgressSummary(inventory),
		"lifecycle", lifecycleName(trace.Lifecycle),
		"reconcile_gate", string(trace.ReconcileGate),
		"winner_goal", goalName(trace.WinnerGoal),
		"winner_score", trace.WinnerScore,
		"selected_goal", goalName(selectedGoal),
		"previous_goal", goalName(previousGoal),
		"preempted", previousGoal != selectedGoal,
		"retained_goal", goalName(trace.RetainedGoal),
		"retained_score", trace.RetainedScore,
		"retained_eligible", trace.RetainedEligible,
		"scores", utilityScoreGroup(trace.Goals),
		"eligibility", utilityEligibilityGroup(trace.Goals),
		"hostile_count", trace.HostileCount,
		"nearest_hostile_distance", trace.NearestHostileDistance,
		"threat", trace.Threat,
		"recent_hostile_count", trace.RecentHostileCount,
		"resource_candidate", resourceCandidateName(trace.ResourceCandidate),
		"resource_candidate_distance", resourceCandidateDistance(trace.ResourceCandidate),
		"mineable_resource_count", trace.MineableResourceCount,
		"decision_action", controllerActionName(decision.Action),
		"decision_target", decisionTargetSummary(decision.Target),
		"desired_controller", controllerStateSummary(controller.Desired),
		"changed_controller_fields", deltaChangedFields(delta),
		"cleared_controller_fields", deltaClearedFields(delta),
		"published_sequence", publishedSequence,
	}
	if noEmitReason != "" {
		attrs = append(attrs, "no_emit_reason", noEmitReason)
	}
	logger.Debug("utility_ai.tick", attrs...)
}

func noControllerEmissionReason(trace ai.UtilityTraceSnapshot, result ai.ReconcileResult) string {
	if result.Decision.Action == model.ControllerActionNone {
		return "no_decision_action"
	}
	return "desired_state_unchanged"
}

func utilityScoreGroup(goals []ai.UtilityGoalTrace) slog.Value {
	attrs := make([]slog.Attr, 0, len(goals))
	for _, goal := range goals {
		attrs = append(attrs, slog.Float64(goalName(goal.Goal), goal.Score))
	}
	return slog.GroupValue(attrs...)
}

func utilityEligibilityGroup(goals []ai.UtilityGoalTrace) slog.Value {
	attrs := make([]slog.Attr, 0, len(goals))
	for _, goal := range goals {
		attrs = append(attrs, slog.Bool(goalName(goal.Goal), goal.Eligible))
	}
	return slog.GroupValue(attrs...)
}

func resourceCandidateName(candidate *model.PerceptionBlock) string {
	if candidate == nil {
		return ""
	}
	return candidate.Name
}

func resourceCandidateDistance(candidate *model.PerceptionBlock) float64 {
	if candidate == nil {
		return -1
	}
	return candidate.Distance
}

func inventoryProgressSummary(inventory model.Inventory) string {
	tracked := []string{"oak_log", "oak_planks", "crafting_table", "wooden_pickaxe", "stone_pickaxe", "wooden_axe", "wooden_sword"}
	counts := make(map[string]int32, len(tracked))
	for _, slot := range inventory.Slots {
		if slot.Item == nil {
			continue
		}
		for _, name := range tracked {
			if slot.Item.Name == name {
				counts[name] += slot.Item.Count
			}
		}
	}
	parts := make([]string, 0, len(tracked))
	for _, name := range tracked {
		parts = append(parts, fmt.Sprintf("%s=%d", name, counts[name]))
	}
	return strings.Join(parts, ",")
}

func controllerStateSummary(state model.ControllerState) string {
	parts := make([]string, 0, 7)
	if state.GotoTarget != nil {
		parts = append(parts, "goto")
	}
	if state.BreakTarget != nil {
		parts = append(parts, "break")
	}
	if state.AttackTarget != nil {
		parts = append(parts, "attack")
	}
	if state.CraftTarget != nil {
		parts = append(parts, "craft")
	}
	if state.EquipTarget != nil {
		parts = append(parts, "equip")
	}
	if state.ConsumeTarget != nil {
		parts = append(parts, "consume")
	}
	if state.PlaceTarget != nil {
		parts = append(parts, "place")
	}
	return strings.Join(parts, ",")
}

func deltaChangedFields(delta *model.ControllerStateDelta) string {
	if delta == nil {
		return ""
	}
	return controllerStateSummary(delta.State)
}

func deltaClearedFields(delta *model.ControllerStateDelta) string {
	if delta == nil {
		return ""
	}
	parts := make([]string, 0, len(delta.ClearFields))
	for _, field := range delta.ClearFields {
		parts = append(parts, controllerFieldName(field))
	}
	return strings.Join(parts, ",")
}

func decisionTargetSummary(target model.GoalTarget) string {
	switch target.Kind {
	case model.GoalTargetEntity:
		return fmt.Sprintf("entity:%d", target.EntityID)
	case model.GoalTargetItem:
		return "item:" + target.Item
	case model.GoalTargetBlock:
		return fmt.Sprintf("block:%d,%d,%d", target.Block.X, target.Block.Y, target.Block.Z)
	case model.GoalTargetDestination:
		return fmt.Sprintf("destination:%.1f,%.1f,%.1f", target.Destination.X, target.Destination.Y, target.Destination.Z)
	default:
		return ""
	}
}

func goalName(goal model.GoalType) string {
	switch goal {
	case model.Idle:
		return "idle"
	case model.Eat:
		return "eat"
	case model.CraftTool:
		return "craft_tool"
	case model.Flee:
		return "flee"
	case model.Fight:
		return "fight"
	case model.GatherResource:
		return "gather_resource"
	default:
		return "unknown"
	}
}

func lifecycleName(phase model.GoalLifecyclePhase) string {
	switch phase {
	case model.GoalPhaseEntering:
		return "entering"
	case model.GoalPhaseExecuting:
		return "executing"
	case model.GoalPhaseExiting:
		return "exiting"
	case model.GoalPhaseBlocked:
		return "blocked"
	default:
		return "inactive"
	}
}

func controllerActionName(action model.ControllerAction) string {
	switch action {
	case model.ControllerActionGoto:
		return "goto"
	case model.ControllerActionBreak:
		return "break"
	case model.ControllerActionAttack:
		return "attack"
	case model.ControllerActionCraft:
		return "craft"
	case model.ControllerActionEquip:
		return "equip"
	case model.ControllerActionPlace:
		return "place"
	case model.ControllerActionConsume:
		return "consume"
	default:
		return "none"
	}
}

func controllerFieldName(field model.ControllerField) string {
	switch field {
	case model.ControllerFieldGotoTarget:
		return "goto"
	case model.ControllerFieldBreakTarget:
		return "break"
	case model.ControllerFieldAttackTarget:
		return "attack"
	case model.ControllerFieldCraftTarget:
		return "craft"
	case model.ControllerFieldEquipTarget:
		return "equip"
	case model.ControllerFieldPlaceTarget:
		return "place"
	case model.ControllerFieldConsumeTarget:
		return "consume"
	default:
		return "unknown"
	}
}

func realityForProfile(view *model.RealityView, profileID model.ProfileID) *model.RealityState {
	state, ok := view.Get(profileID)
	if !ok {
		return nil
	}
	return &state
}

func goalWorldFacts(profileID model.ProfileID, position model.Position, views *model.WorldViews) model.TickWorldFacts {
	world, ok := views.Get(profileID)
	facts := model.TickWorldFacts{HasNavigableWorld: ok && world.HasActiveDimensionType}
	if facts.HasNavigableWorld {
		facts.WanderDestination, facts.HasWanderTarget = pickDestination(profileID, position, views)
		facts.PreconditionsHash = world.Perception.Revision
	}
	return facts
}

func controllerStateForDecision(decision ai.BehaviorDecision) model.ControllerState {
	state := model.EmptyControllerState()
	if decision.MovementTarget.Kind != model.GoalTargetNone {
		target := blockTarget(decision.MovementTarget)
		state.GotoTarget = &target
	}
	switch decision.Action {
	case model.ControllerActionGoto:
		target := blockTarget(decision.Target)
		state.GotoTarget = &target
		return state
	case model.ControllerActionBreak:
		target := blockTarget(decision.Target)
		state.BreakTarget = &target
		return state
	case model.ControllerActionAttack:
		target := decision.Target.EntityID
		state.AttackTarget = &target
		return state
	case model.ControllerActionCraft:
		target := model.CraftTarget{ItemName: decision.Target.Item, Count: decision.CraftCount}
		return model.ControllerState{CraftTarget: &target}
	case model.ControllerActionEquip:
		target := decision.Target.Item
		return model.ControllerState{EquipTarget: &target}
	case model.ControllerActionPlace:
		target := decision.Target.Block
		return model.ControllerState{PlaceTarget: &model.PlaceTarget{X: target.X, Y: target.Y, Z: target.Z}}
	case model.ControllerActionConsume:
		target := decision.Target.Item
		return model.ControllerState{ConsumeTarget: &target}
	default:
		return model.EmptyControllerState()
	}
}

func blockTarget(target model.GoalTarget) model.BlockPosition {
	if target.Kind == model.GoalTargetDestination {
		return model.BlockPosition{X: int32(target.Destination.X), Y: int32(target.Destination.Y), Z: int32(target.Destination.Z)}
	}
	return target.Block
}

func isOneShotControllerAction(action model.ControllerAction) bool {
	switch action {
	case model.ControllerActionCraft, model.ControllerActionEquip, model.ControllerActionConsume, model.ControllerActionPlace:
		return true
	default:
		return false
	}
}

// reconcileOneShotFeedback processes only the currently selected one-shot.
// Matching feedback for a superseded goal is delivery metadata only and cannot
// clear or fail a newer selection.
func reconcileOneShotFeedback(utility *model.UtilityAIState, controller *model.ControllerSyncState, reality *model.RealityState) model.GoalExitReason {
	inFlight := controller.InFlightOneShot
	if reality == nil || inFlight.Action == model.ControllerActionNone || inFlight.Correlation == 0 {
		return model.GoalExitNone
	}
	for _, outcome := range reality.ActionOutcomes {
		if outcome.ControllerSequence != inFlight.Correlation || outcome.Action != inFlight.Action {
			continue
		}
		controller.InFlightOneShot = model.InFlightOneShot{}
		if utility.CurrentGoal != inFlight.Goal || utility.Target != inFlight.Target {
			return model.GoalExitNone
		}
		if outcome.Status == model.ActionOutcomeFailed {
			utility.FailedPlans.Add(model.FailedPlan{Goal: inFlight.Goal, Action: inFlight.Action, Target: inFlight.Target, Reason: model.GoalExitFailed, Correlation: inFlight.Correlation, PreconditionsHash: inFlight.PreconditionsHash, CraftCount: inFlight.CraftCount, PlaceItem: inFlight.PlaceItem})
			return model.GoalExitFailed
		} else if outcome.Status == model.ActionOutcomeCompleted {
			utility.CompletedPlans.Add(model.FailedPlan{Goal: inFlight.Goal, Action: inFlight.Action, Target: inFlight.Target, Reason: model.GoalExitCompleted, Correlation: inFlight.Correlation, PreconditionsHash: inFlight.PreconditionsHash, CraftCount: inFlight.CraftCount, PlaceItem: inFlight.PlaceItem})
			return model.GoalExitCompleted
		}
		return model.GoalExitNone
	}
	return model.GoalExitNone
}

type NeedState struct {
	hungerUrgency  float32
	healthUrgency  float32
	safetyUrgency  float32
	foodSupplyNeed float32
}

func buildControllerState(
	goal ai.GoalType,
	pos model.Position,
	view enginecore.MirroredBotView,
	i int,
	bot model.Bot,
	hostiles []model.PerceivedEntity,
	perceivedBlocks []model.PerceptionBlock,
	worldViews *model.WorldViews,
) ai.ControllerState {
	switch goal {
	case ai.Eat:
		food, ok := ai.FoodInHotbar(view.Inventorys[i])
		if !ok {
			return ai.EmptyState()
		}
		return ai.ControllerState{ConsumeTarget: &food}
	case ai.Idle:
		wv, ok := worldViews.Get(bot.ProfileID)
		if !ok || !wv.HasActiveDimensionType {
			return ai.EmptyState()
		}
		dest, ok := pickDestination(bot.ProfileID, pos, worldViews)
		if !ok {
			return ai.EmptyState()
		}
		return ai.ControllerState{GotoTarget: &dest}
	case ai.Flee:
		dest := ai.EscapeDestination(pos, hostiles)
		return ai.ControllerState{GotoTarget: &dest}
	case ai.Fight:
		if len(hostiles) == 0 {
			return ai.EmptyState()
		}
		closest := hostiles[0]
		for _, h := range hostiles[1:] {
			if h.Distance < closest.Distance {
				closest = h
			}
		}
		id := closest.ID
		return ai.ControllerState{AttackTarget: &id}
	case ai.GatherResource:
		if len(perceivedBlocks) == 0 {
			return ai.EmptyState()
		}
		target := perceivedBlocks[0]
		cs := ai.ControllerState{
			GotoTarget:  &target.Position,
			BreakTarget: &target.Position,
		}
		if tool, ok := ai.ToolForResource(target.Name); ok {
			if !ai.HasItemEquipped(view.Inventorys[i], tool) {
				cs.EquipTarget = &tool
			}
		}
		return cs
	case ai.CraftTool:
		targets := []string{"crafting_table", "wooden_pickaxe", "wooden_axe", "stone_pickaxe", "wooden_sword"}
		for _, t := range targets {
			if ai.HasItem(view.Inventorys[i], t) {
				continue
			}
			steps := ai.StepsFor(t, view.Inventorys[i])
			if len(steps) > 0 {
				ct := ai.ControllerState{}
				ct.CraftTarget = &ai.CraftTarget{
					ItemName: steps[0].ItemName,
					Count:    int32(steps[0].Count),
				}
				return ct
			}
		}
		return ai.EmptyState()
	default:
		return ai.EmptyState()
	}
}

func applyRealityFeedback(cs ai.ControllerState, profileID model.ProfileID, rv *model.RealityView, logger *slog.Logger) ai.ControllerState {
	rs, ok := rv.Get(profileID)
	if !ok {
		return cs
	}

	logger.Info("reality.feedback",
		"profile_id", fmt.Sprintf("%x", profileID),
		"arrival_dist", fmt.Sprintf("%v", rs.ArrivalDistance),
		"reality_goto", fmt.Sprintf("%v", rs.GotoTarget),
		"cs_goto", fmt.Sprintf("%v", cs.GotoTarget),
		"reality_dig", fmt.Sprintf("%v", rs.DiggingBlock),
		"cs_break", fmt.Sprintf("%v", cs.BreakTarget),
	)

	if rs.GotoTarget != nil && cs.GotoTarget != nil && *rs.GotoTarget == *cs.GotoTarget {
		if rs.ArrivalDistance != nil && *rs.ArrivalDistance < 2.0 {
			logger.Info("goal.arrived",
				"profile_id", fmt.Sprintf("%x", profileID),
				"distance", *rs.ArrivalDistance,
			)
			cs.GotoTarget = nil
		}
	}
	return cs
}

func convertControllerState(delta *ai.ControllerStateDelta, seq uint64) *network.ControllerState {
	out := &network.ControllerState{Sequence: seq}
	for _, field := range delta.ClearFields {
		out.ClearFields = append(out.ClearFields, network.ControllerField(field))
	}
	if delta.State.GotoTarget != nil {
		bp := *delta.State.GotoTarget
		out.GoToTarget = &bp
	}
	if delta.State.BreakTarget != nil {
		bp := *delta.State.BreakTarget
		out.BreakTarget = &bp
	}
	if delta.State.AttackTarget != nil {
		v := *delta.State.AttackTarget
		out.AttackTarget = &v
	}
	if delta.State.CraftTarget != nil {
		out.CraftTarget = &network.CraftSpec{
			ItemName: delta.State.CraftTarget.ItemName,
			Count:    delta.State.CraftTarget.Count,
		}
	}
	if delta.State.EquipTarget != nil {
		v := *delta.State.EquipTarget
		out.EquipTarget = &v
	}
	if delta.State.PlaceTarget != nil {
		out.PlaceTarget = &network.PlaceSpec{
			X:     delta.State.PlaceTarget.X,
			Y:     delta.State.PlaceTarget.Y,
			Z:     delta.State.PlaceTarget.Z,
			FaceX: delta.State.PlaceTarget.FaceX,
			FaceY: delta.State.PlaceTarget.FaceY,
			FaceZ: delta.State.PlaceTarget.FaceZ,
		}
	}
	if delta.State.ConsumeTarget != nil {
		v := *delta.State.ConsumeTarget
		out.ConsumeTarget = &v
	}
	return out
}

type NeedSystem struct {
	states map[model.ProfileID]*NeedState
}

func (NeedSystem) ID() scheduler.SystemID {
	return SystemNeeds
}
