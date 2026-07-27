package client

import "minecraft_orchestrator/internal/mc_protocol/wire"

// DecodeClientbound decodes the lifecycle messages understood by this client.
func DecodeClientbound(phase Phase, packet wire.Packet) (ClientboundMessage, error) {
	switch phase {
	case PhaseLogin:
		return decodeLogin(packet)
	case PhaseConfiguration:
		return decodeConfiguration(packet)
	case PhasePlay:
		return decodePlay(packet)
	default:
		return UnknownClientbound{Raw: packet}, nil
	}
}
