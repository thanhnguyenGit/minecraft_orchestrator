package client

import (
	"bytes"
	"fmt"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

const (
	playServerboundTeleportConfirmID         int32 = 0x00
	playServerboundChunkBatchReceivedID      int32 = 0x0a
	playServerboundClientInformationID       int32 = 0x0e
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

func (TeleportConfirm) serverboundMessage()           {}
func (ChunkBatchReceived) serverboundMessage()        {}
func (ConfigurationAcknowledged) serverboundMessage() {}
func (PlayDisconnect) clientboundMessage()            {}
func (SynchronizePlayerPosition) clientboundMessage() {}
func (ChunkBatchFinished) clientboundMessage()        {}
func (StartConfiguration) clientboundMessage()        {}

func encodePlay(message ServerboundMessage) (wire.Packet, error) {
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
