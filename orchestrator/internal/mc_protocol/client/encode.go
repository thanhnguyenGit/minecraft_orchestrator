package client

import (
	"fmt"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

// EncodeServerbound maps a typed message to its protocol-774 packet for phase.
func EncodeServerbound(phase Phase, message ServerboundMessage) (wire.Packet, error) {
	switch phase {
	case PhaseLogin:
		return encodeLogin(message)
	case PhaseConfiguration:
		return encodeConfiguration(message)
	case PhasePlay:
		return encodePlay(message)
	default:
		return wire.Packet{}, fmt.Errorf("unsupported Minecraft phase: %d", phase)
	}
}
