package client

import (
	"bytes"
	"fmt"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

const (
	playServerboundTeleportConfirmID         int32 = 0x00
	playServerboundChunkBatchReceivedID      int32 = 0x0a
	playServerboundClientInformationID       int32 = 0x0d
	playServerboundConfigurationAcknowledged int32 = 0x0f
	playServerboundKeepAliveID               int32 = 0x1b
	playServerboundPongID                    int32 = 0x2c

	playClientboundChunkBatchFinishedID  int32 = 0x0b
	playClientboundDisconnectID          int32 = 0x20
	playClientboundKeepAliveID           int32 = 0x2b
	playClientboundPingID                int32 = 0x3b
	playClientboundSynchronizePositionID int32 = 0x46
	playClientboundStartConfigurationID  int32 = 0x74
)

type TeleportConfirm struct{ ID int32 }
type ChunkBatchReceived struct{ DesiredChunksPerTick float32 }
type ConfigurationAcknowledged struct{}
type PlayDisconnect struct{ Raw wire.Packet }
type SynchronizePlayerPosition struct {
	TeleportID int32
	X, Y, Z    float64
	DX, DY, DZ float64
	Yaw, Pitch float32
	Flags      uint32
}
type ChunkBatchFinished struct{ BatchSize int32 }
type StartConfiguration struct{}
type GlobalPosition struct {
	DimensionName string
	Position      wire.BlockPosition
}
type SpawnInfo struct {
	Dimension, PortalCooldown, SeaLevel int32
	Name                                string
	HashedSeed                          int64
	GameMode                            int8
	PreviousGameMode                    uint8
	IsDebug, IsFlat                     bool
	DeathPosition                       *GlobalPosition
}
type PlayLogin struct {
	EntityID                                               int32
	IsHardcore                                             bool
	WorldNames                                             []string
	MaxPlayers, ViewDistance, SimulationDistance           int32
	ReducedDebugInfo, EnableRespawnScreen, LimitedCrafting bool
	SpawnInfo                                              SpawnInfo
	EnforcesSecureChat                                     bool
}
type Respawn struct {
	SpawnInfo    SpawnInfo
	CopyMetadata uint8
}
type BundleDelimiter struct{}
type Vector3d struct{ X, Y, Z float64 }
type DamageEvent struct {
	EntityID, SourceTypeID, SourceCauseID, SourceDirectID int32
	SourcePosition                                        *Vector3d
}
type EntityStatus struct {
	EntityID int32
	Status   int8
}
type HurtAnimation struct {
	EntityID int32
	Yaw      float32
}
type SynchronizeEntityPosition struct {
	EntityID   int32
	X, Y, Z    float64
	DX, DY, DZ float64
	Yaw, Pitch float32
	OnGround   bool
}
type EntityRelativeMove struct {
	EntityID   int32
	DX, DY, DZ int16
	OnGround   bool
}
type EntityMoveAndLook struct {
	EntityID   int32
	DX, DY, DZ int16
	Yaw, Pitch int8
	OnGround   bool
}
type EntityTeleport struct {
	EntityID   int32
	X, Y, Z    float64
	Yaw, Pitch int8
	OnGround   bool
}
type AttributeModifier struct {
	ID        string
	Amount    float64
	Operation int8
}
type EntityAttribute struct {
	Key       int32
	Value     float64
	Modifiers []AttributeModifier
}
type EntityUpdateAttributes struct {
	EntityID   int32
	Attributes []EntityAttribute
}
type BlockChange struct {
	Position     wire.BlockPosition
	BlockStateID int32
}
type SetHealth struct {
	Health     float32
	Food       int32
	Saturation float32
}
type SetPassengers struct {
	VehicleEntityID    int32
	PassengerEntityIDs []int32
}
type AcknowledgeBlockChange struct{ Sequence int32 }
type EntityVelocity struct {
	EntityID int32
	X, Y, Z  int16
}
type EntityRemoved struct{ EntityIDs []int32 }
type RemoveEntityEffect struct{ EntityID, EffectID int32 }
type EntityEffect struct {
	EntityID, EffectID, Amplifier, Duration int32
	Flags                                   uint8
}
type Experience struct {
	Bar          float32
	Level, Total int32
}
type SetCenterChunk struct{ X, Z int32 }
type SetViewDistance struct{ Distance int32 }

func (TeleportConfirm) serverboundMessage()           {}
func (ChunkBatchReceived) serverboundMessage()        {}
func (ConfigurationAcknowledged) serverboundMessage() {}
func (PlayDisconnect) clientboundMessage()            {}
func (SynchronizePlayerPosition) clientboundMessage() {}
func (ChunkBatchFinished) clientboundMessage()        {}
func (StartConfiguration) clientboundMessage()        {}
func (PlayLogin) clientboundMessage()                 {}
func (Respawn) clientboundMessage()                   {}
func (BundleDelimiter) clientboundMessage()           {}
func (DamageEvent) clientboundMessage()               {}
func (EntityStatus) clientboundMessage()              {}
func (HurtAnimation) clientboundMessage()             {}
func (SynchronizeEntityPosition) clientboundMessage() {}
func (EntityRelativeMove) clientboundMessage()        {}
func (EntityMoveAndLook) clientboundMessage()         {}
func (EntityTeleport) clientboundMessage()            {}
func (EntityUpdateAttributes) clientboundMessage()    {}
func (BlockChange) clientboundMessage()               {}
func (SetHealth) clientboundMessage()                 {}
func (SetPassengers) clientboundMessage()             {}
func (AcknowledgeBlockChange) clientboundMessage()    {}
func (EntityVelocity) clientboundMessage()            {}
func (EntityRemoved) clientboundMessage()             {}
func (RemoveEntityEffect) clientboundMessage()        {}
func (EntityEffect) clientboundMessage()              {}
func (Experience) clientboundMessage()                {}
func (SetCenterChunk) clientboundMessage()            {}
func (SetViewDistance) clientboundMessage()           {}

func encodePlay(message ServerboundMessage) (wire.Packet, error) {
	if packet, handled, err := encodePlayAction(message); handled {
		return packet, err
	}
	var body bytes.Buffer
	switch message := message.(type) {
	case ClientInformation:
		if err := writeClientInformation(&body, message); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: playServerboundClientInformationID, Body: body.Bytes()}, nil
	case KeepAlive:
		if err := wire.WriteInt64(&body, message.ID); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: playServerboundKeepAliveID, Body: body.Bytes()}, nil
	case Pong:
		if err := wire.WriteInt32(&body, message.ID); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: playServerboundPongID, Body: body.Bytes()}, nil
	case TeleportConfirm:
		if err := wire.WriteVarInt(&body, message.ID); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: playServerboundTeleportConfirmID, Body: body.Bytes()}, nil
	case ChunkBatchReceived:
		if err := wire.WriteFloat32(&body, message.DesiredChunksPerTick); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: playServerboundChunkBatchReceivedID, Body: body.Bytes()}, nil
	case ConfigurationAcknowledged:
		return wire.Packet{ID: playServerboundConfigurationAcknowledged}, nil
	default:
		return wire.Packet{}, fmt.Errorf("unsupported play message %T", message)
	}
}

func decodePlay(packet wire.Packet) (ClientboundMessage, error) {
	r := bytes.NewReader(packet.Body)
	switch packet.ID {
	case 0x00:
		return BundleDelimiter{}, wire.RequireEmpty(r)
	case 0x19:
		entityID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		sourceTypeID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		sourceCauseID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		sourceDirectID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		hasPosition, err := wire.ReadBool(r)
		if err != nil {
			return nil, err
		}
		message := DamageEvent{EntityID: entityID, SourceTypeID: sourceTypeID, SourceCauseID: sourceCauseID, SourceDirectID: sourceDirectID}
		if hasPosition {
			position := new(Vector3d)
			position.X, err = wire.ReadFloat64(r)
			if err != nil {
				return nil, err
			}
			position.Y, err = wire.ReadFloat64(r)
			if err != nil {
				return nil, err
			}
			position.Z, err = wire.ReadFloat64(r)
			if err != nil {
				return nil, err
			}
			message.SourcePosition = position
		}
		return message, wire.RequireEmpty(r)
	case 0x22:
		entityID, err := wire.ReadInt32(r)
		if err != nil {
			return nil, err
		}
		status, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		return EntityStatus{EntityID: entityID, Status: int8(status)}, wire.RequireEmpty(r)
	case 0x23:
		entityID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		message := SynchronizeEntityPosition{EntityID: entityID}
		for _, value := range []*float64{&message.X, &message.Y, &message.Z, &message.DX, &message.DY, &message.DZ} {
			if *value, err = wire.ReadFloat64(r); err != nil {
				return nil, err
			}
		}
		if message.Yaw, err = wire.ReadFloat32(r); err != nil {
			return nil, err
		}
		if message.Pitch, err = wire.ReadFloat32(r); err != nil {
			return nil, err
		}
		if message.OnGround, err = wire.ReadBool(r); err != nil {
			return nil, err
		}
		return message, wire.RequireEmpty(r)
	case 0x29:
		entityID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		yaw, err := wire.ReadFloat32(r)
		if err != nil {
			return nil, err
		}
		return HurtAnimation{EntityID: entityID, Yaw: yaw}, wire.RequireEmpty(r)
	case 0x30:
		return readPlayLogin(r)
	case 0x33:
		entityID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		message := EntityRelativeMove{EntityID: entityID}
		for _, value := range []*int16{&message.DX, &message.DY, &message.DZ} {
			if *value, err = wire.ReadInt16(r); err != nil {
				return nil, err
			}
		}
		if message.OnGround, err = wire.ReadBool(r); err != nil {
			return nil, err
		}
		return message, wire.RequireEmpty(r)
	case 0x34:
		entityID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		message := EntityMoveAndLook{EntityID: entityID}
		for _, value := range []*int16{&message.DX, &message.DY, &message.DZ} {
			if *value, err = wire.ReadInt16(r); err != nil {
				return nil, err
			}
		}
		yaw, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		pitch, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		message.Yaw, message.Pitch = int8(yaw), int8(pitch)
		if message.OnGround, err = wire.ReadBool(r); err != nil {
			return nil, err
		}
		return message, wire.RequireEmpty(r)
	case 0x04:
		sequence, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		return AcknowledgeBlockChange{Sequence: sequence}, wire.RequireEmpty(r)
	case 0x08:
		position, err := wire.ReadBlockPosition(r)
		if err != nil {
			return nil, err
		}
		state, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		return BlockChange{Position: position, BlockStateID: state}, wire.RequireEmpty(r)
	case playClientboundDisconnectID:
		return PlayDisconnect{Raw: packet}, nil
	case playClientboundKeepAliveID:
		id, err := wire.ReadInt64(r)
		if err != nil {
			return nil, err
		}
		return KeepAlive{ID: id}, wire.RequireEmpty(r)
	case playClientboundPingID:
		id, err := wire.ReadInt32(r)
		if err != nil {
			return nil, err
		}
		return Ping{ID: id}, wire.RequireEmpty(r)
	case playClientboundSynchronizePositionID:
		return readSynchronizePlayerPosition(r)
	case 0x4b:
		ids, err := readVarIntArray(r, 1<<20)
		if err != nil {
			return nil, err
		}
		return EntityRemoved{EntityIDs: ids}, wire.RequireEmpty(r)
	case 0x4c:
		entityID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		effectID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		return RemoveEntityEffect{EntityID: entityID, EffectID: effectID}, wire.RequireEmpty(r)
	case 0x50:
		spawnInfo, err := readSpawnInfo(r)
		if err != nil {
			return nil, err
		}
		copyMetadata, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		return Respawn{SpawnInfo: spawnInfo, CopyMetadata: copyMetadata}, wire.RequireEmpty(r)
	case 0x5c:
		x, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		z, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		return SetCenterChunk{X: x, Z: z}, wire.RequireEmpty(r)
	case 0x5d:
		distance, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		return SetViewDistance{Distance: distance}, wire.RequireEmpty(r)
	case 0x63:
		entityID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		x, err := wire.ReadInt16(r)
		if err != nil {
			return nil, err
		}
		y, err := wire.ReadInt16(r)
		if err != nil {
			return nil, err
		}
		z, err := wire.ReadInt16(r)
		if err != nil {
			return nil, err
		}
		return EntityVelocity{EntityID: entityID, X: x, Y: y, Z: z}, wire.RequireEmpty(r)
	case 0x65:
		bar, err := wire.ReadFloat32(r)
		if err != nil {
			return nil, err
		}
		level, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		total, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		return Experience{Bar: bar, Level: level, Total: total}, wire.RequireEmpty(r)
	case 0x66:
		health, err := wire.ReadFloat32(r)
		if err != nil {
			return nil, err
		}
		food, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		saturation, err := wire.ReadFloat32(r)
		if err != nil {
			return nil, err
		}
		return SetHealth{Health: health, Food: food, Saturation: saturation}, wire.RequireEmpty(r)
	case 0x69:
		vehicleID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		ids, err := readVarIntArray(r, 1<<20)
		if err != nil {
			return nil, err
		}
		return SetPassengers{VehicleEntityID: vehicleID, PassengerEntityIDs: ids}, wire.RequireEmpty(r)
	case 0x7b:
		entityID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		message := EntityTeleport{EntityID: entityID}
		for _, value := range []*float64{&message.X, &message.Y, &message.Z} {
			if *value, err = wire.ReadFloat64(r); err != nil {
				return nil, err
			}
		}
		yaw, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		pitch, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		message.Yaw, message.Pitch = int8(yaw), int8(pitch)
		if message.OnGround, err = wire.ReadBool(r); err != nil {
			return nil, err
		}
		return message, wire.RequireEmpty(r)
	case 0x81:
		return readEntityUpdateAttributes(r)
	case 0x82:
		entityID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		effectID, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		amplifier, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		duration, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		flags, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		return EntityEffect{EntityID: entityID, EffectID: effectID, Amplifier: amplifier, Duration: duration, Flags: flags}, wire.RequireEmpty(r)
	case playClientboundChunkBatchFinishedID:
		size, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		return ChunkBatchFinished{BatchSize: size}, wire.RequireEmpty(r)
	case playClientboundStartConfigurationID:
		return StartConfiguration{}, wire.RequireEmpty(r)
	default:
		return UnknownClientbound{Raw: packet}, nil
	}
}

func readPlayLogin(r *bytes.Reader) (ClientboundMessage, error) {
	message := PlayLogin{}
	var err error
	if message.EntityID, err = wire.ReadInt32(r); err != nil {
		return nil, err
	}
	if message.IsHardcore, err = wire.ReadBool(r); err != nil {
		return nil, err
	}
	worldCount, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if worldCount < 0 || worldCount > 1<<20 {
		return nil, fmt.Errorf("invalid world name count: %d", worldCount)
	}
	message.WorldNames = make([]string, worldCount)
	for i := range message.WorldNames {
		if message.WorldNames[i], err = wire.ReadString(r); err != nil {
			return nil, err
		}
	}
	for _, value := range []*int32{&message.MaxPlayers, &message.ViewDistance, &message.SimulationDistance} {
		if *value, err = wire.ReadVarInt(r); err != nil {
			return nil, err
		}
	}
	for _, value := range []*bool{&message.ReducedDebugInfo, &message.EnableRespawnScreen, &message.LimitedCrafting} {
		if *value, err = wire.ReadBool(r); err != nil {
			return nil, err
		}
	}
	if message.SpawnInfo, err = readSpawnInfo(r); err != nil {
		return nil, err
	}
	if message.EnforcesSecureChat, err = wire.ReadBool(r); err != nil {
		return nil, err
	}
	return message, wire.RequireEmpty(r)
}

func readSpawnInfo(r *bytes.Reader) (SpawnInfo, error) {
	message := SpawnInfo{}
	var err error
	if message.Dimension, err = wire.ReadVarInt(r); err != nil {
		return SpawnInfo{}, err
	}
	if message.Name, err = wire.ReadString(r); err != nil {
		return SpawnInfo{}, err
	}
	if message.HashedSeed, err = wire.ReadInt64(r); err != nil {
		return SpawnInfo{}, err
	}
	gameMode, err := r.ReadByte()
	if err != nil {
		return SpawnInfo{}, err
	}
	message.GameMode = int8(gameMode)
	if message.PreviousGameMode, err = r.ReadByte(); err != nil {
		return SpawnInfo{}, err
	}
	if message.IsDebug, err = wire.ReadBool(r); err != nil {
		return SpawnInfo{}, err
	}
	if message.IsFlat, err = wire.ReadBool(r); err != nil {
		return SpawnInfo{}, err
	}
	hasDeathPosition, err := wire.ReadBool(r)
	if err != nil {
		return SpawnInfo{}, err
	}
	if hasDeathPosition {
		deathPosition := new(GlobalPosition)
		if deathPosition.DimensionName, err = wire.ReadString(r); err != nil {
			return SpawnInfo{}, err
		}
		if deathPosition.Position, err = wire.ReadBlockPosition(r); err != nil {
			return SpawnInfo{}, err
		}
		message.DeathPosition = deathPosition
	}
	if message.PortalCooldown, err = wire.ReadVarInt(r); err != nil {
		return SpawnInfo{}, err
	}
	if message.SeaLevel, err = wire.ReadVarInt(r); err != nil {
		return SpawnInfo{}, err
	}
	return message, nil
}

func readEntityUpdateAttributes(r *bytes.Reader) (ClientboundMessage, error) {
	entityID, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	count, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if count < 0 || count > 1<<20 {
		return nil, fmt.Errorf("invalid attribute count: %d", count)
	}
	message := EntityUpdateAttributes{EntityID: entityID, Attributes: make([]EntityAttribute, count)}
	for i := range message.Attributes {
		attribute := &message.Attributes[i]
		if attribute.Key, err = wire.ReadVarInt(r); err != nil {
			return nil, err
		}
		if attribute.Value, err = wire.ReadFloat64(r); err != nil {
			return nil, err
		}
		modifierCount, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		if modifierCount < 0 || modifierCount > 1<<20 {
			return nil, fmt.Errorf("invalid attribute modifier count: %d", modifierCount)
		}
		attribute.Modifiers = make([]AttributeModifier, modifierCount)
		for j := range attribute.Modifiers {
			modifier := &attribute.Modifiers[j]
			if modifier.ID, err = wire.ReadString(r); err != nil {
				return nil, err
			}
			if modifier.Amount, err = wire.ReadFloat64(r); err != nil {
				return nil, err
			}
			operation, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			modifier.Operation = int8(operation)
		}
	}
	return message, wire.RequireEmpty(r)
}

func readVarIntArray(r wire.ByteReader, max int32) ([]int32, error) {
	count, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if count < 0 || count > max {
		return nil, fmt.Errorf("invalid VarInt array length: %d", count)
	}
	values := make([]int32, count)
	for i := range values {
		if values[i], err = wire.ReadVarInt(r); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func readSynchronizePlayerPosition(r *bytes.Reader) (ClientboundMessage, error) {
	teleportID, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	x, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	y, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	z, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	dx, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	dy, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	dz, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	yaw, err := wire.ReadFloat32(r)
	if err != nil {
		return nil, err
	}
	pitch, err := wire.ReadFloat32(r)
	if err != nil {
		return nil, err
	}
	flags, err := wire.ReadInt32(r)
	if err != nil {
		return nil, err
	}
	return SynchronizePlayerPosition{TeleportID: teleportID, X: x, Y: y, Z: z, DX: dx, DY: dy, DZ: dz, Yaw: yaw, Pitch: pitch, Flags: uint32(flags)}, wire.RequireEmpty(r)
}
