package runtime

import (
	"errors"
	"fmt"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/mc_protocol/client"
)

var ErrProfileMismatch = errors.New("Minecraft login profile UUID does not match bot specification")

// Adapter converts typed Minecraft packets into engine inputs. It retains only
// the server entity ID for the current connection attempt; ECS remains the
// owner of all mirrored player state.
type Adapter struct {
	profileID      model.ProfileID
	attemptID      uint64
	playerEntityID int32
}

func NewAdapter(profileID model.ProfileID, attemptID uint64) *Adapter {
	return &Adapter{profileID: profileID, attemptID: attemptID}
}

func (a *Adapter) Handle(event client.Event) ([]network.Event, error) {
	switch message := event.Message.(type) {
	case client.LoginSuccess:
		if message.UUID != [16]byte(a.profileID) {
			return nil, fmt.Errorf("%w: got %x, want %x", ErrProfileMismatch, message.UUID, a.profileID)
		}
		return nil, nil
	case client.PlayLogin:
		mode, err := gameMode(message.SpawnInfo.GameMode)
		if err != nil {
			return nil, err
		}
		a.playerEntityID = message.EntityID
		return []network.Event{
			a.newEvent(network.EventPlayReady),
			{
				ProfileID: a.profileID,
				AttemptID: a.attemptID,
				Kind:      network.EventStatePatch,
				Patch:     &network.StatePatch{GameMode: &mode},
			},
		}, nil
	case client.Respawn:
		mode, err := gameMode(message.SpawnInfo.GameMode)
		if err != nil {
			return nil, err
		}
		return []network.Event{{
			ProfileID: a.profileID,
			AttemptID: a.attemptID,
			Kind:      network.EventStatePatch,
			Patch:     &network.StatePatch{GameMode: &mode},
		}}, nil
	case client.SynchronizePlayerPosition:
		return []network.Event{{
			ProfileID: a.profileID,
			AttemptID: a.attemptID,
			Kind:      network.EventPositionCorrection,
			Correction: &network.PositionCorrection{
				Position: model.Position{X: message.X, Y: message.Y, Z: message.Z},
				Velocity: model.Velocity{X: message.DX, Y: message.DY, Z: message.DZ},
				Rotation: model.Rotation{Yaw: message.Yaw, Pitch: message.Pitch},
				Relative: network.RelativeFlags(message.Flags) & (network.RelativePositionX | network.RelativePositionY | network.RelativePositionZ | network.RelativeYaw | network.RelativePitch | network.RelativeVelocityX | network.RelativeVelocityY | network.RelativeVelocityZ),
			},
		}}, nil
	case client.SetHealth:
		health := float64(message.Health)
		return []network.Event{{
			ProfileID: a.profileID,
			AttemptID: a.attemptID,
			Kind:      network.EventStatePatch,
			Patch:     &network.StatePatch{HealthCurrent: &health},
		}}, nil
	case client.EntityVelocity:
		if message.EntityID != a.playerEntityID {
			return nil, nil
		}
		velocity := model.Velocity{
			X: float64(message.X) / 8000,
			Y: float64(message.Y) / 8000,
			Z: float64(message.Z) / 8000,
		}
		return []network.Event{{
			ProfileID: a.profileID,
			AttemptID: a.attemptID,
			Kind:      network.EventStatePatch,
			Patch:     &network.StatePatch{Velocity: &velocity},
		}}, nil
	default:
		return nil, nil
	}
}

func (a *Adapter) newEvent(kind network.EventKind) network.Event {
	return network.Event{
		ProfileID:      a.profileID,
		AttemptID:      a.attemptID,
		Kind:           kind,
		PlayerEntityID: a.playerEntityID,
	}
}

func gameMode(value int8) (model.GameMode, error) {
	if value < int8(model.GameModeSurvival) || value > int8(model.GameModeSpectator) {
		return 0, fmt.Errorf("unsupported Minecraft game mode: %d", value)
	}
	return model.GameMode(value), nil
}
