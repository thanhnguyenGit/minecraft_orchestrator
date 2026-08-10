package ai

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

var (
	_ func() ControllerState                                       = EmptyState
	_ func(ControllerState) ControllerState                        = ValidateConstraint
	_ func(ControllerState, ControllerState) *ControllerStateDelta = DiffControllerState
	_ func(ControllerState, ControllerState) *ControllerState      = DiffControllerStateLegacy
)

func TestDiffControllerStateClearsRemovedTargets(t *testing.T) {
	last := ControllerState{
		GotoTarget:  &model.BlockPosition{X: 4, Y: 64, Z: -2},
		BreakTarget: &model.BlockPosition{X: 4, Y: 63, Z: -2},
	}

	delta := DiffControllerState(last, EmptyState())
	if delta == nil {
		t.Fatal("DiffControllerState() = nil, want clear-only delta")
	}
	if !delta.Clears(ControllerFieldGotoTarget) {
		t.Fatal("delta does not clear goto target")
	}
	if !delta.Clears(ControllerFieldBreakTarget) {
		t.Fatal("delta does not clear break target")
	}
}

func TestDiffControllerStateLegacyReturnsChangedTargets(t *testing.T) {
	previous := EmptyState()
	target := model.BlockPosition{X: 4, Y: 64, Z: -2}
	current := ControllerState{GotoTarget: &target}

	delta := DiffControllerStateLegacy(previous, current)
	if delta == nil || delta.GotoTarget == nil {
		t.Fatalf("DiffControllerStateLegacy() = %#v, want changed goto target", delta)
	}
	if *delta.GotoTarget != target {
		t.Fatalf("goto target = %#v, want %#v", *delta.GotoTarget, target)
	}
}
