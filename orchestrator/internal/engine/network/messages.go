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
	EventHostStatus
	EventHostSnapshot
	EventHostPosition
	EventHostVitals
	EventHostEffects
	EventHostInventory
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

type HostStatus uint8
const (
	HostConnecting HostStatus = iota
	HostConnected
	HostDisconnected
	HostKicked
	HostError
)
type HostVitals struct { Health float64; Food int32; Saturation float64; Oxygen int32 }
type HostSnapshot struct { Vitals HostVitals; Position model.Position; Rotation model.Rotation; Velocity model.Velocity; Inventory model.Inventory; Effects model.Effects; GameMode model.GameMode }

// Event is an immutable observation from one Minecraft session attempt. It
// deliberately carries no ECS Entity or World reference: ECS resolves and
// validates the profile/attempt identity inside its Input phase.
type Event struct {
	ProfileID model.ProfileID
	AttemptID uint64
	Kind      EventKind

	PlayerEntityID int32
	Failure        string
	RemoteSessionID string
	Sequence        uint64
	HostStatus      HostStatus
	Snapshot        *HostSnapshot
	Vitals          *HostVitals
	Position        *HostSnapshot
	Effects         *model.Effects
	Inventory       *model.Inventory
	Correction     *PositionCorrection
	Patch          *StatePatch
}

type Batch struct {
	Events []Event
}
