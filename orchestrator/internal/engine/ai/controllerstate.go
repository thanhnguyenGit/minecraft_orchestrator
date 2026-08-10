package ai

import "minecraft_orchestrator/internal/engine/model"

type ControllerField = model.ControllerField

const (
	ControllerFieldGotoTarget    = model.ControllerFieldGotoTarget
	ControllerFieldBreakTarget   = model.ControllerFieldBreakTarget
	ControllerFieldAttackTarget  = model.ControllerFieldAttackTarget
	ControllerFieldCraftTarget   = model.ControllerFieldCraftTarget
	ControllerFieldEquipTarget   = model.ControllerFieldEquipTarget
	ControllerFieldPlaceTarget   = model.ControllerFieldPlaceTarget
	ControllerFieldConsumeTarget = model.ControllerFieldConsumeTarget
)

type CraftTarget = model.CraftTarget
type PlaceTarget = model.PlaceTarget
type ControllerState = model.ControllerState
type ControllerStateDelta = model.ControllerStateDelta

// EmptyState preserves the legacy AI API while controller state is model-owned.
func EmptyState() ControllerState {
	return model.EmptyControllerState()
}

// ValidateConstraint preserves the legacy AI API and its delta semantics.
func ValidateConstraint(state ControllerState) ControllerState {
	return model.ValidateControllerState(state)
}

// DiffControllerState preserves the legacy AI API and its delta semantics.
func DiffControllerState(last, current ControllerState) *ControllerStateDelta {
	return model.DiffControllerState(last, current)
}

// DiffControllerStateLegacy preserves the pre-delta return shape for callers
// that only need changed targets. Removed targets are intentionally omitted:
// use DiffControllerState when explicit controller clears must be delivered.
func DiffControllerStateLegacy(last, current ControllerState) *ControllerState {
	delta := model.DiffControllerState(last, current)
	if delta == nil || !delta.State.HasAny() {
		return nil
	}

	state := delta.State.Clone()
	return &state
}
