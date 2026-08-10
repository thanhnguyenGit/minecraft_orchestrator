package ai

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestFoodInHotbarReturnsConsumableItem(t *testing.T) {
	food, ok := FoodInHotbar(model.Inventory{Slots: []model.InventorySlot{
		{Slot: 9, Item: &model.ItemStack{ID: 1, Name: "stone"}},
		{Slot: 2, Item: &model.ItemStack{ID: 800, Name: "apple"}},
	}})
	if !ok || food != "apple" {
		t.Fatalf("FoodInHotbar() = (%q, %t), want (apple, true)", food, ok)
	}
}

func TestSelectGoalDoesNotApplyCommitmentBonus(t *testing.T) {
	scores := map[GoalType]float64{Idle: ScoreWander(), Fight: ScoreWander() + 0.001}
	if got := SelectGoal(scores, Idle); got != Fight {
		t.Fatalf("SelectGoal() = %v, want higher-scoring Fight without Idle commitment", got)
	}
}

func TestGoalTypesAliasModelValuesInDeclaredOrder(t *testing.T) {
	want := []model.GoalType{
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
	got := []GoalType{Idle, Eat, CraftTool, FindFood, Flee, Fight, GatherResource, Hunt, ReturnToShelter}

	for index, goal := range got {
		var modelGoal model.GoalType = goal
		if modelGoal != want[index] {
			t.Fatalf("goal at index %d = %d, want %d", index, modelGoal, want[index])
		}
	}
}

func TestControllerDomainTypesAliasModel(t *testing.T) {
	var state model.ControllerState = ControllerState{}
	var craft model.CraftTarget = CraftTarget{ItemName: "stick", Count: 2}
	var place model.PlaceTarget = PlaceTarget{X: 1}
	var field model.ControllerField = ControllerFieldGotoTarget

	if state.HasAny() || craft.ItemName != "stick" || place.X != 1 || field != model.ControllerFieldGotoTarget {
		t.Fatalf("controller aliases did not preserve model values")
	}
}
