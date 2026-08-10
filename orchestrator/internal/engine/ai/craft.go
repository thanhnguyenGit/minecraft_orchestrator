package ai

import (
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/mc_protocol/registry"
)

type CraftStep struct {
	ItemName   string
	Count      int
	NeedsTable bool
}

// craftSteps builds a deterministic plan using the static Minecraft registry.
func craftSteps(target string, inv model.Inventory) []CraftStep {
	if _, ok := registry.RecipeFor(target); !ok {
		return nil
	}
	planner := craftPlanner{items: inventoryCounts(inv)}
	if !planner.plan(target, 1) {
		return nil
	}
	return planner.steps
}

type craftPlanner struct {
	items map[string]int
	steps []CraftStep
	stack map[string]bool
}

func (p *craftPlanner) plan(target string, required int) bool {
	if required <= 0 || p.items[target] >= required {
		return true
	}
	if p.stack == nil {
		p.stack = make(map[string]bool)
	}
	if p.stack[target] {
		return false
	}

	recipe, ok := registry.RecipeFor(target)
	if !ok || recipe.OutputCount <= 0 {
		return false
	}
	p.stack[target] = true
	defer delete(p.stack, target)

	crafts := divideRoundUp(required-p.items[target], recipe.OutputCount)
	if recipe.NeedsTable && !p.plan("crafting_table", 1) {
		return false
	}
	
	for _, prerequisite := range recipe.Prerequisites {
		if !p.plan(prerequisite.ItemName, prerequisite.Count) {
			return false
		}
	}
	
	for _, ingredient := range recipe.Ingredients {
		if ingredient.Count <= 0 || !p.plan(ingredient.ItemName, ingredient.Count*crafts) {
			return false
		}
		p.items[ingredient.ItemName] -= ingredient.Count * crafts
	}
	
	p.items[target] += recipe.OutputCount * crafts
	p.steps = append(p.steps, CraftStep{
		ItemName: target, 
		Count: crafts, 
		NeedsTable: recipe.NeedsTable,
	})
	
	return true
}

func divideRoundUp(value, divisor int) int {
	return (value + divisor - 1) / divisor
}

func inventoryCounts(inv model.Inventory) map[string]int {
	items := make(map[string]int, len(inv.Slots))
	for _, slot := range inv.Slots {
		if slot.Item == nil || slot.Item.Name == "" || slot.Item.Count <= 0 {
			continue
		}
		items[slot.Item.Name] += int(slot.Item.Count)
	}
	return items
}

func countItem(inv model.Inventory, name string) int {
	total := 0
	for _, slot := range inv.Slots {
		if slot.Item != nil && slot.Item.Name == name {
			total += int(slot.Item.Count)
		}
	}
	return total
}

func CanCraft(target string, inv model.Inventory) bool {
	return len(craftSteps(target, inv)) > 0
}

func StepsFor(target string, inv model.Inventory) []CraftStep {
	return craftSteps(target, inv)
}
