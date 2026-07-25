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
	CHunger
	CInputState
	CConnection
	CDisconnected
	ComponentCount
)

func ParseComponent(v uint8) (Component, error) {
	if v >= uint8(ComponentCount) {
		return 0, fmt.Errorf("invalid component value: %d", v)
	}

	return Component(v), nil
}

var componentNames = [...]string{
	"Meta",
	"Position",
	"Velocity",
	"Bot",
	"Health",
	"Hunger",
	"InputState",
	"Connection",
	"Disconnected",
}

type Bot struct {
	ID uint64
}

type Position struct {
	X, Y, Z float64
}

type Velocity struct {
	X, Y, Z float64
}

type Connection struct {
	ClientId string
	SessionId string
}

type Disconnected struct {
	SinceTick uint64
}

type Health struct {
	Current float64
	Max float64
}

var (
	ConnecedBotMask = Components(
		CPosition,
		CVelocity,
		CHealth,
		CBot,
		// CInputState,
		CConnection,
	)
	DisconnectedBotMask = Components( 
		CPosition,
		CVelocity,
		CHealth,
		CBot,
		// CInputState,
		CDisconnected,
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

