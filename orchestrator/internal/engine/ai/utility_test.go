package ai

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestGoalTypeAliasesModelWithExactValuesInOrder(t *testing.T) {
	modelGoals := []model.GoalType{
		model.Idle,
		model.Eat,
		model.CraftTool,
		model.FindFood,
		model.Flee,
		model.Fight,
		model.GatherResource,
		model.Hunt,
		model.ReturnToShelter,
	}
	aiGoals := []GoalType{
		Idle,
		Eat,
		CraftTool,
		FindFood,
		Flee,
		Fight,
		GatherResource,
		Hunt,
		ReturnToShelter,
	}

	for index := range modelGoals {
		if got, want := uint8(modelGoals[index]), uint8(index); got != want {
			t.Fatalf("model GoalType at index %d = %d, want %d", index, got, want)
		}
		var modelGoal model.GoalType = aiGoals[index]
		if modelGoal != modelGoals[index] {
			t.Fatalf("AI GoalType at index %d = %d, want model value %d", index, modelGoal, modelGoals[index])
		}
	}
}
