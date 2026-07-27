package client

import "minecraft_orchestrator/internal/mc_protocol/wire"

// ClientboundMessage is a typed packet received from the server.
type ClientboundMessage interface{ clientboundMessage() }

// ServerboundMessage is a typed packet sent to the server.
type ServerboundMessage interface{ serverboundMessage() }

// KeepAlive uses a phase-specific packet ID in both directions.
type KeepAlive struct{ ID int64 }

// Ping is a clientbound request that requires a Pong response.
type Ping struct{ ID int32 }

// Pong is the serverbound response to Ping.
type Pong struct{ ID int32 }

// UnknownClientbound retains packets that do not yet have a typed decoder.
type UnknownClientbound struct{ Raw wire.Packet }

func (KeepAlive) clientboundMessage()          {}
func (KeepAlive) serverboundMessage()          {}
func (Ping) clientboundMessage()               {}
func (Pong) serverboundMessage()               {}
func (UnknownClientbound) clientboundMessage() {}
