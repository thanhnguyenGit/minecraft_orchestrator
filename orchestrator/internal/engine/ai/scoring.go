package ai

import (
	"math"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/mc_protocol/registry"
)

var hostileNames = map[string]bool{
	"blaze":            true,
	"creeper":          true,
	"drowned":          true,
	"elder_guardian":   true,
	"enderman":         true,
	"endermite":        true,
	"evoker":           true,
	"ghast":            true,
	"guardian":         true,
	"hoglin":           true,
	"husk":             true,
	"magma_cube":       true,
	"phantom":          true,
	"piglin":           true,
	"piglin_brute":     true,
	"pillager":         true,
	"ravager":          true,
	"shulker":          true,
	"silverfish":       true,
	"skeleton":         true,
	"slime":            true,
	"spider":           true,
	"stray":            true,
	"vex":              true,
	"vindicator":       true,
	"witch":            true,
	"wither":           true,
	"warden":           true,
	"wither_skeleton":  true,
	"zoglin":           true,
	"zombie":           true,
	"zombified_piglin": true,
}

func IsHostile(name string) bool {
	return hostileNames[name]
}

func HasFood(inv model.Inventory) bool {
	_, ok := FoodInHotbar(inv)
	return ok
}

func FoodInHotbar(inv model.Inventory) (string, bool) {
	for _, slot := range inv.Slots {
		if slot.Item == nil {
			continue
		}
		if slot.Slot < 0 || slot.Slot > 8 {
			continue
		}
		if registry.IsFoodItem(slot.Item.ID) {
			return slot.Item.Name, true
		}
	}
	return "", false
}

type ScoringInput struct {
	HealthPct            float64
	HungerPct            float64
	HasFood              bool
	Hostiles             []model.PerceivedEntity
	Inventory            model.Inventory
	VisibleResourceCount int
}

func CalcThreat(hostiles []model.PerceivedEntity) float64 {
	var threat float64
	for _, h := range hostiles {
		d := h.Distance
		if d < 0.5 {
			d = 0.5
		}

		threat += 1.0 / d
	}

	return threat
}

func ScoreEat(in ScoringInput) float64 {
	need := 1.0 - in.HungerPct
	if need < 0 {
		need = 0
	}

	opportunity := 0.0
	if in.HasFood {
		opportunity = 1.0
	}

	return need * opportunity
}

func ScoreFight(in ScoringInput) float64 {
	threat := CalcThreat(in.Hostiles)
	if threat == 0 {
		return 0
	}

	opportunity := 1.0
	risk := 1.0 - in.HealthPct

	return threat * opportunity * (1.0 - risk)
}

func ScoreFlee(in ScoringInput) float64 {
	threat := CalcThreat(in.Hostiles)
	if threat == 0 {
		return 0
	}

	ratio := threat / math.Max(in.HealthPct, 0.01)
	if ratio <= 1.0 {
		return 0
	}

	return ratio - 1.0
}

func ScoreWander() float64 {
	return 0.05
}

func ScoreGatherResource(in ScoringInput) float64 {
	need := toolNeed(in.Inventory)
	opportunity := 0.0
	if in.VisibleResourceCount > 0 {
		opportunity = 1.0
	}
	return need * opportunity
}

func toolNeed(inv model.Inventory) float64 {
	need := 0.0
	hasWoodPick := hasItem(inv, "wooden_pickaxe")

	if !hasWoodPick {
		need += 0.4
	}
	if hasWoodPick && !hasItem(inv, "stone_pickaxe") {
		need += 0.3
	}
	if !hasItem(inv, "wooden_axe") {
		need += 0.2
	}
	if !hasItem(inv, "wooden_sword") {
		need += 0.1
	}
	if need == 0 {
		return 0
	}

	logCount := countItem(inv, "oak_log")
	plankCount := countItem(inv, "oak_planks")
	hasTable := hasItem(inv, "crafting_table")

	if hasTable && plankCount >= 2 {
		return need * 0.1
	}
	if plankCount >= 4 || hasTable {
		return need * 0.15
	}
	if logCount > 0 || plankCount > 0 {
		return need * 0.25
	}
	return need
}

func ScoreCraftTool(in ScoringInput) float64 {
	targets := []struct {
		name  string
		score float64
	}{
		{"crafting_table", 0.3},
		{"wooden_pickaxe", 0.5},
		{"wooden_axe", 0.3},
		{"stone_pickaxe", 0.4},
		{"wooden_sword", 0.2},
	}

	bestNeed := 0.0
	for _, t := range targets {
		if hasItem(in.Inventory, t.name) {
			continue
		}
		if CanCraft(t.name, in.Inventory) {
			if t.score > bestNeed {
				bestNeed = t.score
			}
		}
	}

	if bestNeed == 0 {
		return 0
	}
	return bestNeed
}

func hasItem(inv model.Inventory, name string) bool {
	for _, slot := range inv.Slots {
		if slot.Item != nil && slot.Item.Name == name {
			return true
		}
	}
	return false
}

func HasItem(inv model.Inventory, name string) bool {
	return hasItem(inv, name)
}

func HasItemEquipped(inv model.Inventory, name string) bool {
	for _, slot := range inv.Slots {
		if slot.Item != nil && slot.Item.Name == name && slot.Slot >= 0 && slot.Slot <= 8 {
			return true
		}
	}
	return false
}

func SelectGoal(scores map[GoalType]float64, _ GoalType) GoalType {
	best := Idle
	bestScore := -1.0

	for _, goal := range []GoalType{Flee, Fight, Eat, CraftTool, GatherResource, Idle} {
		score := scores[goal]
		if score > bestScore {
			bestScore = score
			best = goal
		}
	}

	return best
}

func EscapeDestination(botPos model.Position, hostiles []model.PerceivedEntity) model.BlockPosition {
	var cx, cz float64
	for _, h := range hostiles {
		cx += h.Position.X
		cz += h.Position.Z
	}

	if len(hostiles) > 0 {
		cx /= float64(len(hostiles))
		cz /= float64(len(hostiles))
	}

	dx := botPos.X - cx
	dz := botPos.Z - cz

	dist := math.Sqrt(dx*dx + dz*dz)
	if dist < 0.01 {
		dx = 1
		dz = 0
		dist = 1
	}

	runDistance := 15.0
	x := int32(botPos.X + (dx/dist)*runDistance)
	z := int32(botPos.Z + (dz/dist)*runDistance)
	y := int32(botPos.Y)

	return model.BlockPosition{
		X: x,
		Y: y,
		Z: z,
	}
}
