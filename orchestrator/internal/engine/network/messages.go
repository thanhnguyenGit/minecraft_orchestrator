package network

import "minecraft_orchestrator/internal/engine/model"

type EventKind uint8

const (
	EventHostStatus EventKind = iota
	EventHostSnapshot
	EventHostPosition
	EventHostVitals
	EventHostEffects
	EventHostInventory
)

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
	Kind      EventKind

	Failure         string
	RemoteSessionID string
	Sequence        uint64
	HostStatus      HostStatus
	Snapshot        *HostSnapshot
	Vitals          *HostVitals
	Position        *HostSnapshot
	Effects         *model.Effects
	Inventory       *model.Inventory
}

type Batch struct {
	Events []Event
}
