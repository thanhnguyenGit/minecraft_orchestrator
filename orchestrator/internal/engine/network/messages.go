package network

import "minecraft_orchestrator/internal/engine/model"

type EventKind uint8

const (
	EventConnecting EventKind = iota
	EventPlayReady
	EventSessionClosed
	EventSessionFailed
	EventPositionCorrection
	EventStatePatch
)

type RelativeFlags uint32

const (
	RelativePositionX RelativeFlags = 1 << iota
	RelativePositionY
	RelativePositionZ
	RelativeYaw
	RelativePitch
	RelativeVelocityX
	RelativeVelocityY
	RelativeVelocityZ
)

type PositionCorrection struct {
	Position model.Position
	Velocity model.Velocity
	Rotation model.Rotation
	Relative RelativeFlags
}

type StatePatch struct {
	HealthCurrent *float64
	Velocity      *model.Velocity
	GameMode      *model.GameMode
}

// Event is an immutable observation from one Minecraft session attempt. It
// deliberately carries no ECS Entity or World reference: ECS resolves and
// validates the profile/attempt identity inside its Input phase.
type Event struct {
	ProfileID model.ProfileID
	AttemptID uint64
	Kind      EventKind

	PlayerEntityID int32
	Failure        string
	Correction     *PositionCorrection
	Patch          *StatePatch
}

type Batch struct {
	Events []Event
}
