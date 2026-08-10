package model

import "testing"

func TestMirroredBotComponentsIncludeUtilityAIState(t *testing.T) {
	const wantComponentCount = 12
	if ComponentCount != wantComponentCount {
		t.Fatalf("ComponentCount = %d, want %d mirrored bot components", ComponentCount, wantComponentCount)
	}

	for _, component := range []Component{CUtilityAI, CControllerSync} {
		parsed, err := ParseComponent(uint8(component))
		if err != nil {
			t.Fatalf("ParseComponent(%d) error = %v", component, err)
		}
		if parsed != component {
			t.Fatalf("ParseComponent(%d) = %v, want %v", component, parsed, component)
		}
	}

	if got := CUtilityAI.String(); got != "UtilityAI" {
		t.Fatalf("CUtilityAI.String() = %q, want UtilityAI", got)
	}
	if got := CControllerSync.String(); got != "ControllerSync" {
		t.Fatalf("CControllerSync.String() = %q, want ControllerSync", got)
	}

	want := Components(CBot, CSession, CPosition, CRotation, CVelocity, CHealth, CHunger, CGameMode, CInventory, CEffects, CUtilityAI, CControllerSync)
	if !MirroredBotMask.Equals(want) {
		t.Fatalf("MirroredBotMask = %s, want %s", MirroredBotMask, want)
	}
}

func TestUtilityAIStateRepresentsLifecycleAndConcreteTarget(t *testing.T) {
	state := UtilityAIState{
		CurrentGoal: CraftTool,
		Phase:       GoalPhaseBlocked,
		Target: GoalTarget{
			Kind: GoalTargetItem,
			Item: "wooden_pickaxe",
		},
		RecentHostiles:     [RecentHostileMemoryCapacity]HostileMemory{{EntityID: 42, SeenTick: 99}},
		RecentHostileCount: 1,
		LastExitReason:     GoalExitBlocked,
		FailedPlans: FailedPlanCache{
			Entries: [FailedPlanCacheCapacity]FailedPlan{{Goal: CraftTool, Reason: GoalExitBlocked, PreconditionsHash: 123}},
			Count:   1,
		},
	}

	if state.Target.Kind != GoalTargetItem || state.Target.Item != "wooden_pickaxe" {
		t.Fatalf("Target = %#v, want item target", state.Target)
	}
	if state.RecentHostiles[0].SeenTick != 99 || state.FailedPlans.Entries[0].PreconditionsHash != 123 {
		t.Fatalf("UtilityAIState = %#v, want hostile memory tick and failed-plan preconditions hash", state)
	}
	if state.FailedPlans.Count != 1 || len(state.FailedPlans.Entries) != 16 {
		t.Fatalf("FailedPlans = %#v, want bounded cache with one entry", state.FailedPlans)
	}
}

func TestFailedPlanCacheAddEvictsOldestAtCapacity(t *testing.T) {
	var cache FailedPlanCache
	for i := range FailedPlanCacheCapacity + 2 {
		cache.Add(FailedPlan{Correlation: uint64(i)})
	}

	if got := cache.Len(); got != FailedPlanCacheCapacity {
		t.Fatalf("Len() = %d, want %d", got, FailedPlanCacheCapacity)
	}
	if cache.Count > FailedPlanCacheCapacity {
		t.Fatalf("Count = %d, exceeds capacity %d", cache.Count, FailedPlanCacheCapacity)
	}
	if got := cache.Entries[0].Correlation; got != 2 {
		t.Fatalf("oldest entry correlation = %d, want 2 after FIFO eviction", got)
	}
	if got := cache.Entries[FailedPlanCacheCapacity-1].Correlation; got != FailedPlanCacheCapacity+1 {
		t.Fatalf("newest entry correlation = %d, want %d", got, FailedPlanCacheCapacity+1)
	}
}

func TestFailedPlanCacheAddRepairsInvalidCount(t *testing.T) {
	cache := FailedPlanCache{Count: FailedPlanCacheCapacity + 4}
	cache.Add(FailedPlan{Correlation: 1})

	if cache.Count > FailedPlanCacheCapacity {
		t.Fatalf("Count = %d, exceeds capacity %d", cache.Count, FailedPlanCacheCapacity)
	}
}
