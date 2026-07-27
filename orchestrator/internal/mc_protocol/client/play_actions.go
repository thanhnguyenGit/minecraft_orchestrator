package client

import (
	"bytes"
	"fmt"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

// MovementFlags are the flags carried by serverbound player movement packets.
type MovementFlags uint8

const (
	MovementOnGround MovementFlags = 1 << iota
	MovementHorizontalCollision
)

type PlayerPosition struct {
	X, Y, Z float64
	Flags   MovementFlags
}
type PlayerPositionAndLook struct {
	X, Y, Z    float64
	Yaw, Pitch float32
	Flags      MovementFlags
}
type PlayerLook struct {
	Yaw, Pitch float32
	Flags      MovementFlags
}
type PlayerFlying struct{ Flags MovementFlags }
type VehicleMove struct {
	X, Y, Z    float64
	Yaw, Pitch float32
	OnGround   bool
}
type UseEntity struct {
	TargetID int32
	Action   int32 // 0 interact, 1 attack, 2 interact-at
	TargetX  float32
	TargetY  float32
	TargetZ  float32
	Hand     int32
	Sneaking bool
}
type AttackEntity struct {
	TargetID int32
	Sneaking bool
}
type PlayerDigging struct {
	Status   int32
	Position wire.BlockPosition
	Face     int8
	Sequence int32
}
type UseItemOnBlock struct {
	Hand                        int32
	Position                    wire.BlockPosition
	Face                        int32
	CursorX, CursorY, CursorZ   float32
	InsideBlock, WorldBorderHit bool
	Sequence                    int32
}
type UseItem struct {
	Hand       int32
	Sequence   int32
	Yaw, Pitch float32
}
type PlayerInput struct{ Flags uint8 }
type PerformRespawn struct{}
type PlayerLoaded struct{}
type ArmSwing struct{ Hand int32 }
type HeldItemSlot struct{ Slot int16 }
type EntityAction struct{ EntityID, ActionID, JumpBoost int32 }
type CraftRecipeRequest struct {
	WindowID int8
	RecipeID int32
	MakeAll  bool
}

func (PlayerPosition) serverboundMessage()        {}
func (PlayerPositionAndLook) serverboundMessage() {}
func (PlayerLook) serverboundMessage()            {}
func (PlayerFlying) serverboundMessage()          {}
func (VehicleMove) serverboundMessage()           {}
func (UseEntity) serverboundMessage()             {}
func (AttackEntity) serverboundMessage()          {}
func (PlayerDigging) serverboundMessage()         {}
func (UseItemOnBlock) serverboundMessage()        {}
func (UseItem) serverboundMessage()               {}
func (PlayerInput) serverboundMessage()           {}
func (PerformRespawn) serverboundMessage()        {}
func (PlayerLoaded) serverboundMessage()          {}
func (ArmSwing) serverboundMessage()              {}
func (HeldItemSlot) serverboundMessage()          {}
func (EntityAction) serverboundMessage()          {}
func (CraftRecipeRequest) serverboundMessage()    {}

func encodePlayAction(message ServerboundMessage) (wire.Packet, bool, error) {
	var b bytes.Buffer
	writeMovement := func(x, y, z float64, flags MovementFlags) error {
		for _, value := range []float64{x, y, z} {
			if err := wire.WriteFloat64(&b, value); err != nil {
				return err
			}
		}
		return b.WriteByte(byte(flags))
	}
	switch m := message.(type) {
	case PlayerPosition:
		if err := writeMovement(m.X, m.Y, m.Z, m.Flags); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x1d, Body: b.Bytes()}, true, nil
	case PlayerPositionAndLook:
		// Position and look places rotation before movement flags.
		for _, value := range []float64{m.X, m.Y, m.Z} {
			if err := wire.WriteFloat64(&b, value); err != nil {
				return wire.Packet{}, true, err
			}
		}
		for _, value := range []float32{m.Yaw, m.Pitch} {
			if err := wire.WriteFloat32(&b, value); err != nil {
				return wire.Packet{}, true, err
			}
		}
		if err := b.WriteByte(byte(m.Flags)); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x1e, Body: b.Bytes()}, true, nil
	case PlayerLook:
		for _, value := range []float32{m.Yaw, m.Pitch} {
			if err := wire.WriteFloat32(&b, value); err != nil {
				return wire.Packet{}, true, err
			}
		}
		if err := b.WriteByte(byte(m.Flags)); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x1f, Body: b.Bytes()}, true, nil
	case PlayerFlying:
		if err := b.WriteByte(byte(m.Flags)); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x20, Body: b.Bytes()}, true, nil
	case VehicleMove:
		for _, value := range []float64{m.X, m.Y, m.Z} {
			if err := wire.WriteFloat64(&b, value); err != nil {
				return wire.Packet{}, true, err
			}
		}
		for _, value := range []float32{m.Yaw, m.Pitch} {
			if err := wire.WriteFloat32(&b, value); err != nil {
				return wire.Packet{}, true, err
			}
		}
		if err := wire.WriteBool(&b, m.OnGround); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x21, Body: b.Bytes()}, true, nil
	case UseEntity:
		if m.Action < 0 || m.Action > 2 {
			return wire.Packet{}, true, fmt.Errorf("invalid use entity action: %d", m.Action)
		}
		if err := wire.WriteVarInt(&b, m.TargetID); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteVarInt(&b, m.Action); err != nil {
			return wire.Packet{}, true, err
		}
		if m.Action == 2 {
			for _, v := range []float32{m.TargetX, m.TargetY, m.TargetZ} {
				if err := wire.WriteFloat32(&b, v); err != nil {
					return wire.Packet{}, true, err
				}
			}
		}
		if m.Action == 0 || m.Action == 2 {
			if err := wire.WriteVarInt(&b, m.Hand); err != nil {
				return wire.Packet{}, true, err
			}
		}
		if err := wire.WriteBool(&b, m.Sneaking); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x19, Body: b.Bytes()}, true, nil
	case AttackEntity:
		return encodePlayAction(UseEntity{TargetID: m.TargetID, Action: 1, Sneaking: m.Sneaking})
	case PlayerDigging:
		if err := wire.WriteVarInt(&b, m.Status); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteBlockPosition(&b, m.Position); err != nil {
			return wire.Packet{}, true, err
		}
		if err := b.WriteByte(byte(m.Face)); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteVarInt(&b, m.Sequence); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x28, Body: b.Bytes()}, true, nil
	case UseItemOnBlock:
		if err := wire.WriteVarInt(&b, m.Hand); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteBlockPosition(&b, m.Position); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteVarInt(&b, m.Face); err != nil {
			return wire.Packet{}, true, err
		}
		for _, v := range []float32{m.CursorX, m.CursorY, m.CursorZ} {
			if err := wire.WriteFloat32(&b, v); err != nil {
				return wire.Packet{}, true, err
			}
		}
		if err := wire.WriteBool(&b, m.InsideBlock); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteBool(&b, m.WorldBorderHit); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteVarInt(&b, m.Sequence); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x3f, Body: b.Bytes()}, true, nil
	case UseItem:
		if err := wire.WriteVarInt(&b, m.Hand); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteVarInt(&b, m.Sequence); err != nil {
			return wire.Packet{}, true, err
		}
		for _, v := range []float32{m.Yaw, m.Pitch} {
			if err := wire.WriteFloat32(&b, v); err != nil {
				return wire.Packet{}, true, err
			}
		}
		return wire.Packet{ID: 0x40, Body: b.Bytes()}, true, nil
	case PlayerInput:
		if err := b.WriteByte(m.Flags); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x2a, Body: b.Bytes()}, true, nil
	case PerformRespawn:
		if err := wire.WriteVarInt(&b, 0); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x0b, Body: b.Bytes()}, true, nil
	case PlayerLoaded:
		return wire.Packet{ID: 0x2b}, true, nil
	case ArmSwing:
		if err := wire.WriteVarInt(&b, m.Hand); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x3c, Body: b.Bytes()}, true, nil
	case HeldItemSlot:
		if err := wire.WriteInt16(&b, m.Slot); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x34, Body: b.Bytes()}, true, nil
	case EntityAction:
		for _, v := range []int32{m.EntityID, m.ActionID, m.JumpBoost} {
			if err := wire.WriteVarInt(&b, v); err != nil {
				return wire.Packet{}, true, err
			}
		}
		return wire.Packet{ID: 0x29, Body: b.Bytes()}, true, nil
	case CraftRecipeRequest:
		if err := b.WriteByte(byte(m.WindowID)); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteVarInt(&b, m.RecipeID); err != nil {
			return wire.Packet{}, true, err
		}
		if err := wire.WriteBool(&b, m.MakeAll); err != nil {
			return wire.Packet{}, true, err
		}
		return wire.Packet{ID: 0x26, Body: b.Bytes()}, true, nil
	default:
		return wire.Packet{}, false, nil
	}
}
