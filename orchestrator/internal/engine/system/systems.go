package core

import (
	"fmt"
	"log/slog"

	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/engine/scheduler"
)

const (
	SystemBootstrap    scheduler.SystemID = "Bootstrap"
	SystemNetworkApply scheduler.SystemID = "NetworkApply"
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
		for _, view := range views {
			for index, bot := range view.Bots {
				if bot.ProfileID != event.ProfileID {
					continue
				}

				applyNetworkEvent(&view, index, event, ctx.Logger)
			}
		}
	}

	for _, view := range views {
		for i, bot := range view.Bots {
			ctx.Logger.Info("ecs.state",
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

	view.GameModes[index], view.Inventorys[index], view.Effectss[index] = snapshot.GameMode, snapshot.Inventory, snapshot.Effects
}
