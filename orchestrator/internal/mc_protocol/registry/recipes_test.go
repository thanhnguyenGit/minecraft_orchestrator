package registry

import "testing"

func TestRecipeRegistryPinsMinecraftVersion(t *testing.T) {
	if Version != "1.21.11" {
		t.Fatalf("registry version = %q, want 1.21.11", Version)
	}
	if err := ValidateVersion(Version); err != nil {
		t.Fatalf("ValidateVersion(%q) error = %v", Version, err)
	}
	if err := ValidateVersion("1.21.10"); err == nil {
		t.Fatal("ValidateVersion accepted an unpinned Minecraft version")
	}
}

func TestRecipeRegistryReturnsRepresentativeRecipes(t *testing.T) {
	tests := []struct {
		output     string
		count      int
		needsTable bool
		ingredient Ingredient
	}{
		{"oak_planks", 4, false, Ingredient{ItemName: "oak_log", Count: 1}},
		{"crafting_table", 1, false, Ingredient{ItemName: "oak_planks", Count: 4}},
		{"stick", 4, false, Ingredient{ItemName: "oak_planks", Count: 2}},
		{"wooden_pickaxe", 1, true, Ingredient{ItemName: "oak_planks", Count: 3}},
		{"stone_pickaxe", 1, true, Ingredient{ItemName: "cobblestone", Count: 3}},
	}

	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			recipe, ok := RecipeFor(tt.output)
			if !ok {
				t.Fatalf("RecipeFor(%q) returned no recipe", tt.output)
			}
			if recipe.OutputCount != tt.count || recipe.NeedsTable != tt.needsTable {
				t.Fatalf("recipe = %#v, want output count %d and needsTable %v", recipe, tt.count, tt.needsTable)
			}
			if len(recipe.Ingredients) == 0 || recipe.Ingredients[0] != tt.ingredient {
				t.Fatalf("recipe ingredients = %#v, want first ingredient %#v", recipe.Ingredients, tt.ingredient)
			}
		})
	}
}

func TestRecipeForReturnsDefensiveIngredients(t *testing.T) {
	first, ok := RecipeFor("stick")
	if !ok {
		t.Fatal("RecipeFor(stick) returned no recipe")
	}
	first.Ingredients[0].Count = 99

	second, ok := RecipeFor("stick")
	if !ok {
		t.Fatal("RecipeFor(stick) returned no recipe on second lookup")
	}
	if second.Ingredients[0].Count != 2 {
		t.Fatalf("second recipe ingredient count = %d, want 2", second.Ingredients[0].Count)
	}
}
