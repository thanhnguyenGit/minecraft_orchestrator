package client

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

var traceSessionCounter atomic.Uint64

func nextTraceSessionID(_ string) string {
	return fmt.Sprintf("session-%d", traceSessionCounter.Add(1))
}

func (s *Session) handleInbound(packet wire.Packet, message ClientboundMessage) error {
	sequence := s.tracePacket("inbound", packet, message, 0)
	previousCause := s.traceCause.Swap(sequence)
	defer s.traceCause.Store(previousCause)
	return s.handlePacket(message)
}

func (s *Session) traceOutbound(packet wire.Packet, message ServerboundMessage) {
	s.tracePacket("outbound", packet, message, s.traceCause.Load())
}

func (s *Session) tracePacket(direction string, packet wire.Packet, message any, causedBy uint64) uint64 {
	logger := s.cfg.Logger
	ctx := context.Background()
	if logger == nil || !logger.Enabled(ctx, slog.LevelDebug) {
		return 0
	}
	sequence := s.traceSequence.Add(1)
	attrs := []slog.Attr{
		slog.String("session_id", s.traceSessionID),
		slog.Uint64("sequence", sequence),
		slog.String("direction", direction),
		slog.String("phase", tracePhaseName(s.phase)),
		slog.String("packet_id", fmt.Sprintf("0x%x", packet.ID)),
		slog.Int("body_bytes", len(packet.Body)),
		slog.String("message_type", fmt.Sprintf("%T", message)),
	}
	if causedBy != 0 {
		attrs = append(attrs, slog.Uint64("caused_by", causedBy))
	}
	attrs = append(attrs, traceMessageAttrs(message)...)
	logger.LogAttrs(ctx, slog.LevelDebug, "minecraft.packet", attrs...)
	return sequence
}

func tracePhaseName(phase Phase) string {
	switch phase {
	case PhaseLogin:
		return "login"
	case PhaseConfiguration:
		return "configuration"
	case PhasePlay:
		return "play"
	default:
		return "unknown"
	}
}

func traceMessageAttrs(message any) []slog.Attr {
	switch message := message.(type) {
	case SynchronizePlayerPosition:
		return []slog.Attr{slog.Int64("teleport_id", int64(message.TeleportID)), slog.Float64("x", message.X), slog.Float64("y", message.Y), slog.Float64("z", message.Z)}
	case TeleportConfirm:
		return []slog.Attr{slog.Int64("teleport_id", int64(message.ID))}
	case SetHealth:
		return []slog.Attr{slog.Float64("health", float64(message.Health)), slog.Int64("food", int64(message.Food)), slog.Float64("saturation", float64(message.Saturation))}
	case EntityVelocity:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Int64("velocity_x", int64(message.X)), slog.Int64("velocity_y", int64(message.Y)), slog.Int64("velocity_z", int64(message.Z))}
	case DamageEvent:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Int64("source_type_id", int64(message.SourceTypeID)), slog.Int64("source_cause_id", int64(message.SourceCauseID)), slog.Int64("source_direct_id", int64(message.SourceDirectID))}
	case EntityStatus:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Int64("status", int64(message.Status))}
	case HurtAnimation:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Float64("yaw", float64(message.Yaw))}
	case SynchronizeEntityPosition:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Float64("x", message.X), slog.Float64("y", message.Y), slog.Float64("z", message.Z)}
	case EntityRelativeMove:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Int64("delta_x", int64(message.DX)), slog.Int64("delta_y", int64(message.DY)), slog.Int64("delta_z", int64(message.DZ))}
	case EntityMoveAndLook:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Int64("delta_x", int64(message.DX)), slog.Int64("delta_y", int64(message.DY)), slog.Int64("delta_z", int64(message.DZ))}
	case EntityTeleport:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Float64("x", message.X), slog.Float64("y", message.Y), slog.Float64("z", message.Z)}
	case EntityUpdateAttributes:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Int("attribute_count", len(message.Attributes))}
	case PlayLogin:
		return []slog.Attr{slog.Int64("entity_id", int64(message.EntityID)), slog.Int64("view_distance", int64(message.ViewDistance)), slog.Int64("simulation_distance", int64(message.SimulationDistance)), slog.Int64("game_mode", int64(message.SpawnInfo.GameMode))}
	case Respawn:
		return []slog.Attr{slog.Int64("dimension", int64(message.SpawnInfo.Dimension)), slog.Int64("game_mode", int64(message.SpawnInfo.GameMode))}
	case ChunkBatchReceived:
		return []slog.Attr{slog.Float64("desired_chunks_per_tick", float64(message.DesiredChunksPerTick))}
	case Pong:
		return []slog.Attr{slog.Int64("ping_id", int64(message.ID))}
	case KeepAlive:
		return []slog.Attr{slog.Int64("keep_alive_id", message.ID)}
	default:
		return nil
	}
}
