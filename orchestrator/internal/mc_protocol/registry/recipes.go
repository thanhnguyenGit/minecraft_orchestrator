// Code generated from minecraft-data/data/pc/1.21.11/items.json and recipes.json;
// DO NOT EDIT. The compact records below retain only recipes used by the utility AI.

package registry

import "fmt"

// Version is the Minecraft data version from which this registry was generated.
const Version = "1.21.11"

// Ingredient is an item and its required quantity.
type Ingredient struct {
	ItemName string
	Count    int
}

// Recipe is an immutable planning view of a static Minecraft crafting recipe.
// Prerequisites are non-consumed progression requirements used by the utility AI.
type Recipe struct {
	Output        string
	OutputCount   int
	Ingredients   []Ingredient
	Prerequisites []Ingredient
	NeedsTable    bool
}

var recipesByOutput = map[string]Recipe{
	"oak_planks": {
		Output:      "oak_planks",
		OutputCount: 4,
		Ingredients: []Ingredient{{ItemName: "oak_log", Count: 1}},
	},
	"crafting_table": {
		Output:      "crafting_table",
		OutputCount: 1,
		Ingredients: []Ingredient{{ItemName: "oak_planks", Count: 4}},
	},
	"stick": {
		Output:      "stick",
		OutputCount: 4,
		Ingredients: []Ingredient{{ItemName: "oak_planks", Count: 2}},
	},
	"wooden_pickaxe": {
		Output:      "wooden_pickaxe",
		OutputCount: 1,
		Ingredients: []Ingredient{
			{ItemName: "oak_planks", Count: 3},
			{ItemName: "stick", Count: 2},
		},
		NeedsTable: true,
	},
	"wooden_axe": {
		Output:      "wooden_axe",
		OutputCount: 1,
		Ingredients: []Ingredient{
			{ItemName: "oak_planks", Count: 3},
			{ItemName: "stick", Count: 2},
		},
		NeedsTable: true,
	},
	"stone_pickaxe": {
		Output:      "stone_pickaxe",
		OutputCount: 1,
		Ingredients: []Ingredient{
			{ItemName: "cobblestone", Count: 3},
			{ItemName: "stick", Count: 2},
		},
		Prerequisites: []Ingredient{{ItemName: "wooden_pickaxe", Count: 1}},
		NeedsTable:    true,
	},
}

// ValidateVersion rejects data versions other than the statically embedded snapshot.
func ValidateVersion(version string) error {
	if version != Version {
		return fmt.Errorf("minecraft registry version %q does not match pinned version %q", version, Version)
	}
	return nil
}

// RecipeFor returns a defensive copy of the recipe for output.
func RecipeFor(output string) (Recipe, bool) {
	recipe, ok := recipesByOutput[output]
	if !ok {
		return Recipe{}, false
	}
	recipe.Ingredients = append([]Ingredient(nil), recipe.Ingredients...)
	recipe.Prerequisites = append([]Ingredient(nil), recipe.Prerequisites...)
	return recipe, true
}
