package ai

import (
	"strings"

	"minecraft_orchestrator/internal/engine/model"
)

var resourceBlockNames = map[string]bool{
	"acacia_log":             true,
	"acacia_wood":            true,
	"birch_log":              true,
	"birch_wood":             true,
	"cherry_log":             true,
	"cherry_wood":            true,
	"coal_ore":               true,
	"cobblestone":            true,
	"copper_ore":             true,
	"dark_oak_log":           true,
	"dark_oak_wood":          true,
	"deepslate_coal_ore":     true,
	"deepslate_copper_ore":   true,
	"deepslate_diamond_ore":  true,
	"deepslate_emerald_ore":  true,
	"deepslate_gold_ore":     true,
	"deepslate_iron_ore":     true,
	"deepslate_lapis_ore":    true,
	"deepslate_redstone_ore": true,
	"diamond_ore":            true,
	"emerald_ore":            true,
	"gold_ore":               true,
	"iron_ore":               true,
	"jungle_log":             true,
	"jungle_wood":            true,
	"lapis_ore":              true,
	"mangrove_log":           true,
	"mangrove_wood":          true,
	"oak_log":                true,
	"oak_wood":               true,
	"redstone_ore":           true,
	"spruce_log":             true,
	"spruce_wood":            true,
	"stone":                  true,
}

func IsResource(name string) bool {
	return resourceBlockNames[name]
}

var toolForResource = map[string]string{
	"oak_log": "wooden_axe", "birch_log": "wooden_axe", "spruce_log": "wooden_axe",
	"jungle_log": "wooden_axe", "acacia_log": "wooden_axe", "dark_oak_log": "wooden_axe",
	"mangrove_log": "wooden_axe", "cherry_log": "wooden_axe",
	"oak_wood": "wooden_axe", "birch_wood": "wooden_axe", "spruce_wood": "wooden_axe",
	"jungle_wood": "wooden_axe", "acacia_wood": "wooden_axe", "dark_oak_wood": "wooden_axe",
	"mangrove_wood": "wooden_axe", "cherry_wood": "wooden_axe",
	"stone": "wooden_pickaxe", "cobblestone": "wooden_pickaxe",
	"coal_ore": "wooden_pickaxe", "deepslate_coal_ore": "wooden_pickaxe",
	"iron_ore": "wooden_pickaxe", "deepslate_iron_ore": "wooden_pickaxe",
	"copper_ore": "wooden_pickaxe", "deepslate_copper_ore": "wooden_pickaxe",
	"gold_ore": "wooden_pickaxe", "deepslate_gold_ore": "wooden_pickaxe",
	"diamond_ore": "wooden_pickaxe", "deepslate_diamond_ore": "wooden_pickaxe",
	"redstone_ore": "wooden_pickaxe", "deepslate_redstone_ore": "wooden_pickaxe",
	"lapis_ore": "wooden_pickaxe", "deepslate_lapis_ore": "wooden_pickaxe",
	"emerald_ore": "wooden_pickaxe", "deepslate_emerald_ore": "wooden_pickaxe",
}

func ToolForResource(resourceName string) (string, bool) {
	tool, ok := toolForResource[resourceName]
	return tool, ok
}

func CanMine(name string, inv model.Inventory) bool {
	if strings.HasSuffix(name, "_log") || strings.HasSuffix(name, "_wood") {
		return true
	}
	if name == "stone" || name == "cobblestone" || strings.HasSuffix(name, "_ore") {
		return hasItem(inv, "wooden_pickaxe") || hasItem(inv, "stone_pickaxe")
	}
	return false
}
