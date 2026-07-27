package core

import (
	"fmt"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/scheduler"
)

const (
	SystemConnection          scheduler.SystemID = "Connection"
	SystemApplyInput          scheduler.SystemID = "ApplyInput"
	SystemMovement            scheduler.SystemID = "Movement"
	SystemHealth              scheduler.SystemID = "Health"
	SystemDisconnectedCleanUp scheduler.SystemID = "DisconnectedCleanup"
)

type TickData struct {
	
}

func tickData(ctx *scheduler.RunContext) (*TickData, error) {
	data, ok := ctx.Data.(*TickData)
	if !ok || data == nil {
		return nil, fmt.Errorf("unexpected tick data %T", ctx.Data)
	}

	return data, nil
}

type ConnectionSystem struct {}

func (ConnectionSystem) ID() scheduler.SystemID {
	return SystemConnection
}

func (ConnectionSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{
		Structural: []model.Mask {
			model.ConnectedBotMask,
			model.DisconnectedBotMask,
		},
	}
}

func (ConnectionSystem) Run(ctx *scheduler.RunContext) error {
	return nil
}

type ApplyInputSystem struct {
	Speed float64
}

func (ApplyInputSystem) ID() scheduler.SystemID {
	return SystemApplyInput
}

func (ApplyInputSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{
		Queries: []model.Mask {
			model.ConnectedBotMask,
		},
		Writes: model.Components(model.CInputState, model.CVelocity),
	}
}

func (ApplyInputSystem)  Run(ctx *scheduler.RunContext) error {
	return nil
}

type MovementSystem struct {
	Grain int
}

func (MovementSystem) ID() scheduler.SystemID{
	return SystemMovement
}

func (MovementSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{}
}

func (MovementSystem) Run(ctx *scheduler.RunContext) error {
	return nil
}

type DisconnectedCleanUpSystem struct {}

func (DisconnectedCleanUpSystem) ID() scheduler.SystemID {
	return SystemDisconnectedCleanUp
}

func (DisconnectedCleanUpSystem) Access() scheduler.AccessSpec {
	return scheduler.AccessSpec{}
}

func (DisconnectedCleanUpSystem) Run(ctx *scheduler.RunContext) error {
	return nil
}

