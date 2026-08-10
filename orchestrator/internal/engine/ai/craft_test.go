package ai

import (
	"reflect"
	"testing"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/mc_protocol/registry"
)

func TestStepsForBuildsWoodenPickaxeCraftChainFromStaticRegistry(t *testing.T) {
	inv := inventory("oak_log", 3)

	got := StepsFor("wooden_pickaxe", inv)
	want := []CraftStep{
		{ItemName: "oak_planks", Count: 1},
		{ItemName: "crafting_table", Count: 1},
		{ItemName: "oak_planks", Count: 1},
		{ItemName: "oak_planks", Count: 1},
		{ItemName: "stick", Count: 1},
		{ItemName: "wooden_pickaxe", Count: 1, NeedsTable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StepsFor(wooden_pickaxe) = %#v, want %#v", got, want)
	}
	if !CanCraft("wooden_pickaxe", inv) {
		t.Fatal("CanCraft(wooden_pickaxe) = false, want true")
	}
}

func TestStepsForRequiresMaterialsAndReturnsNoPlanForUnsupportedTargets(t *testing.T) {
	if got := StepsFor("wooden_pickaxe", inventory("oak_log", 2)); got != nil {
		t.Fatalf("StepsFor with insufficient logs = %#v, want nil", got)
	}
	if CanCraft("wooden_pickaxe", inventory("oak_log", 2)) {
		t.Fatal("CanCraft accepted insufficient materials")
	}
	if got := StepsFor("diamond_pickaxe", inventory("diamond", 3)); got != nil {
		t.Fatalf("StepsFor unsupported target = %#v, want nil", got)
	}
}

func TestStepsForStonePickaxeRetainsProgressiveToolChain(t *testing.T) {
	inv := inventory("oak_log", 5, "cobblestone", 3)

	got := StepsFor("stone_pickaxe", inv)
	want := []CraftStep{
		{ItemName: "oak_planks", Count: 1},
		{ItemName: "crafting_table", Count: 1},
		{ItemName: "oak_planks", Count: 1},
		{ItemName: "oak_planks", Count: 1},
		{ItemName: "stick", Count: 1},
		{ItemName: "wooden_pickaxe", Count: 1, NeedsTable: true},
		{ItemName: "stone_pickaxe", Count: 1, NeedsTable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StepsFor(stone_pickaxe) = %#v, want %#v", got, want)
	}
}

func TestStepsForDoesNotMutateInventoryAndUsesStaticRegistry(t *testing.T) {
	inv := inventory("oak_log", 1)
	before := inv
	got := StepsFor("oak_planks", inv)
	if !reflect.DeepEqual(inv, before) {
		t.Fatalf("StepsFor mutated inventory: got %#v, want %#v", inv, before)
	}
	if len(got) != 1 || got[0].ItemName != "oak_planks" {
		t.Fatalf("StepsFor(oak_planks) = %#v", got)
	}
	if registry.Version != "1.21.11" {
		t.Fatalf("planner static registry version = %q, want 1.21.11", registry.Version)
	}
}

func inventory(items ...any) model.Inventory {
	inv := model.Inventory{}
	for i := 0; i < len(items); i += 2 {
		inv.Slots = append(inv.Slots, model.InventorySlot{
			Slot: int32(i / 2),
			Item: &model.ItemStack{Name: items[i].(string), Count: int32(items[i+1].(int))},
		})
	}
	return inv
}
