package core

import (
	"fmt"

	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/engine/scheduler"
)

const (
	SystemBootstrap           scheduler.SystemID = "Bootstrap"
	SystemConnection          scheduler.SystemID = "Connection"
	SystemNetworkApply        scheduler.SystemID = "NetworkApply"
	SystemApplyInput          scheduler.SystemID = "ApplyInput"
	SystemMovement            scheduler.SystemID = "Movement"
	SystemHealth              scheduler.SystemID = "Health"
	SystemDisconnectedCleanUp scheduler.SystemID = "DisconnectedCleanup"
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

type ConnectionSystem struct{}

func (ConnectionSystem) ID() scheduler.SystemID {
	return SystemConnection
}

func (ConnectionSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{
		Structural: []model.Mask{
			model.ConnectedBotMask,
			model.DisconnectedBotMask,
		},
	}
}

func (ConnectionSystem) Run(ctx *scheduler.RunContext) error {
	return nil
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

// NetworkApplySystem is the only system that turns Minecraft session events
// into ECS component mutations. The Inbox and session workers never receive a
// World pointer.
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
				applyNetworkEvent(&view, index, event)
			}
		}
	}

	return nil
}

func applyNetworkEvent(view *enginecore.MirroredBotView, index int, event network.Event) {
	session := view.Sessions[index]

	switch event.Kind {
	case network.EventConnecting:
		if session.Phase == model.SessionStopped || session.Phase == model.SessionRetryWaiting {
			view.Sessions[index] = model.Session{
				Phase:     model.SessionConnecting,
				AttemptID: event.AttemptID,
			}
		}
	case network.EventPlayReady:
		if session.Phase == model.SessionConnecting && session.AttemptID == event.AttemptID {
			session.Phase = model.SessionPlayReady
			session.PlayerEntityID = event.PlayerEntityID
			view.Sessions[index] = session
		}
	case network.EventSessionClosed:
		if session.AttemptID == event.AttemptID && session.Phase != model.SessionStopped {
			view.Sessions[index] = model.Session{
				Phase: model.SessionRetryWaiting,
			}
		}
	case network.EventSessionFailed:
		if session.AttemptID == event.AttemptID {
			view.Sessions[index] = model.Session{
				Phase:   model.SessionFailed,
				Failure: event.Failure,
			}
		}
	case network.EventPositionCorrection:
		if session.Phase == model.SessionPlayReady && session.AttemptID == event.AttemptID && event.Correction != nil {
			applyPositionCorrection(view, index, *event.Correction)
		}
	case network.EventStatePatch:
		if session.Phase == model.SessionPlayReady && session.AttemptID == event.AttemptID && event.Patch != nil {
			applyStatePatch(view, index, *event.Patch)
		}
	case network.EventHostStatus:
		applyHostStatus(view, index, event)
	case network.EventHostSnapshot:
		if acceptHostObservation(view, index, event) && event.Snapshot != nil {
			applyHostSnapshot(view, index, *event.Snapshot)
		}
	case network.EventHostPosition:
		if acceptHostObservation(view, index, event) && event.Position != nil {
			applyHostSnapshot(view, index, *event.Position)
		}
	case network.EventHostVitals:
		if acceptHostObservation(view, index, event) && event.Vitals != nil {
			health := view.Healths[index]
			health.Current = event.Vitals.Health
			view.Healths[index] = health
		}
	case network.EventHostEffects:
		if acceptHostObservation(view, index, event) && event.Effects != nil {
			view.Effectss[index] = *event.Effects
		}
	case network.EventHostInventory:
		if acceptHostObservation(view, index, event) && event.Inventory != nil {
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

func applyHostStatus(view *enginecore.MirroredBotView, index int, event network.Event) {
	session := view.Sessions[index]
	if event.RemoteSessionID != "" && session.RemoteSessionID != event.RemoteSessionID {
		session.RemoteSessionID = event.RemoteSessionID
		session.LastSequence = 0
	}
	if event.Sequence <= session.LastSequence {
		return
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
}

func applyHostSnapshot(view *enginecore.MirroredBotView, index int, snapshot network.HostSnapshot) {
	view.Positions[index], view.Rotations[index], view.Velocitys[index] = snapshot.Position, snapshot.Rotation, snapshot.Velocity
	health := view.Healths[index]
	health.Current = snapshot.Vitals.Health
	view.Healths[index] = health
	view.GameModes[index], view.Inventorys[index], view.Effectss[index] = snapshot.GameMode, snapshot.Inventory, snapshot.Effects
}

func applyPositionCorrection(view *enginecore.MirroredBotView, index int, correction network.PositionCorrection) {
	position := correction.Position
	velocity := correction.Velocity
	rotation := correction.Rotation
	currentPosition := view.Positions[index]
	currentVelocity := view.Velocitys[index]
	currentRotation := view.Rotations[index]

	if correction.Relative&network.RelativePositionX != 0 {
		position.X += currentPosition.X
	}
	if correction.Relative&network.RelativePositionY != 0 {
		position.Y += currentPosition.Y
	}
	if correction.Relative&network.RelativePositionZ != 0 {
		position.Z += currentPosition.Z
	}
	if correction.Relative&network.RelativeYaw != 0 {
		rotation.Yaw += currentRotation.Yaw
	}
	if correction.Relative&network.RelativePitch != 0 {
		rotation.Pitch += currentRotation.Pitch
	}
	if correction.Relative&network.RelativeVelocityX != 0 {
		velocity.X += currentVelocity.X
	}
	if correction.Relative&network.RelativeVelocityY != 0 {
		velocity.Y += currentVelocity.Y
	}
	if correction.Relative&network.RelativeVelocityZ != 0 {
		velocity.Z += currentVelocity.Z
	}

	view.Positions[index] = position
	view.Velocitys[index] = velocity
	view.Rotations[index] = rotation
}

func applyStatePatch(view *enginecore.MirroredBotView, index int, patch network.StatePatch) {
	if patch.HealthCurrent != nil {
		health := view.Healths[index]
		health.Current = *patch.HealthCurrent
		view.Healths[index] = health
	}
	if patch.Velocity != nil {
		view.Velocitys[index] = *patch.Velocity
	}
	if patch.GameMode != nil {
		view.GameModes[index] = *patch.GameMode
	}
}

type ApplyInputSystem struct {
	Speed float64
}

func (ApplyInputSystem) ID() scheduler.SystemID {
	return SystemApplyInput
}

func (ApplyInputSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{
		Queries: []model.Mask{
			model.ConnectedBotMask,
		},
		Writes: model.Components(model.CInputState, model.CVelocity),
	}
}

func (ApplyInputSystem) Run(ctx *scheduler.RunContext) error {
	return nil
}

type MovementSystem struct {
	Grain int
}

func (MovementSystem) ID() scheduler.SystemID {
	return SystemMovement
}

func (MovementSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{}
}

func (MovementSystem) Run(ctx *scheduler.RunContext) error {
	return nil
}

type DisconnectedCleanUpSystem struct{}

func (DisconnectedCleanUpSystem) ID() scheduler.SystemID {
	return SystemDisconnectedCleanUp
}

func (DisconnectedCleanUpSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{}
}

func (DisconnectedCleanUpSystem) Run(ctx *scheduler.RunContext) error {
	return nil
}
