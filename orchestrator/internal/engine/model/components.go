package model

import (
	"fmt"
	"strings"
)

type (
	Component uint8
	Mask      uint64
)

const (
	CPosition Component = iota
	CVelocity
	CHealth
	CBot
	CRotation
	CGameMode
	CSession
	CInventory
	CEffects
	ComponentCount
)

func ParseComponent(v uint8) (Component, error) {
	if v >= uint8(ComponentCount) {
		return 0, fmt.Errorf("invalid component value: %d", v)
	}

	return Component(v), nil
}

var componentNames = [...]string{
	"Position",
	"Velocity",
	"Health",
	"Bot",
	"Rotation",
	"GameMode",
	"Session",
	"Inventory",
	"Effects",
}

type ProfileID [16]byte

type Bot struct {
	ProfileID ProfileID
	Username  string
}

type Position struct {
	X, Y, Z float64
}

type Velocity struct {
	X, Y, Z float64
}

type Rotation struct {
	Yaw, Pitch float32
}

type GameMode uint8

const (
	GameModeSurvival GameMode = iota
	GameModeCreative
	GameModeAdventure
	GameModeSpectator
)

type SessionPhase uint8

const (
	SessionStopped SessionPhase = iota
	SessionConnecting
	SessionPlayReady
	SessionRetryWaiting
	SessionFailed
)

type Session struct {
	Phase          SessionPhase
	AttemptID      uint64
	PlayerEntityID int32
	Failure        string
	RemoteSessionID string
	LastSequence    uint64
}

type ItemStack struct { ID int32; Name string; Metadata int32; Count int32 }
type InventorySlot struct { Slot int32; Item *ItemStack }
type Inventory struct { SelectedHotbarSlot int32; Slots []InventorySlot }
type Effect struct { ID int32; Name string; Amplifier int32; DurationTicks int32 }
type Effects struct { Values []Effect }

type Health struct {
	Current float64
	Max     float64
}

var (
	MirroredBotMask = Components(
		CPosition,
		CVelocity,
		CHealth,
		CBot,
		CRotation,
		CGameMode,
		CSession,
		CInventory,
		CEffects,
	)
)

func (c Component) String() string {
	if c >= ComponentCount {
		return fmt.Sprintf("Component(%d)", c)
	}

	return componentNames[c]
}

func Bit(c Component) Mask {
	if c >= 64 {
		panic("component id exceeds mask width")
	}

	return 1 << c
}

func Components(components ...Component) Mask {
	var m Mask

	for _, c := range components {
		m |= Bit(c)
	}

	return m
}

func (m Mask) Contains(candidate Mask) bool {
	return m&candidate == candidate
}

func (m Mask) Has(c Component) bool {
	return m&Bit(c) != 0
}

func (m Mask) Intersects(other Mask) bool {
	return m&other != 0
}

func (m Mask) Equals(other Mask) bool {
	return m == other
}

func (m Mask) String() string {
	if m == 0 {
		return "{}"
	}

	parts := make([]string, 0, ComponentCount)

	for component := range ComponentCount {
		if m.Has(component) {
			parts = append(parts, component.String())
		}
	}

	return "{" + strings.Join(parts, ",") + "}"
}
