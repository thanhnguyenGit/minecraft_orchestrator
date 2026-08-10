package ai

import (
	"reflect"
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func behaviorTestContext(tick uint64) model.TickContext {
	return model.NewTickContext(model.TickContextInput{
		Tick: tick,
		Bot:  model.Bot{ProfileID: model.ProfileID{1}, Username: "utility-bot"},
		Session: model.Session{
			Phase:          model.SessionPlayReady,
			AttemptID:      7,
			PlayerEntityID: 99,
		},
		Position: model.Position{X: 10, Y: 64, Z: 10},
		Health:   model.Health{Current: 20, Max: 20},
		Hunger:   model.Hunger{Current: 20, Max: 20},
		World: model.TickWorldFacts{
			HasNavigableWorld: true,
			PreconditionsHash: 123,
		},
	})
}

func withInventory(ctx model.TickContext, slots ...model.InventorySlot) model.TickContext {
	return model.NewTickContext(model.TickContextInput{
		Tick:      ctx.Tick,
		Bot:       model.Bot{ProfileID: ctx.ProfileID, Username: ctx.Username},
		Session:   ctx.Session,
		Position:  ctx.Position,
		Rotation:  ctx.Rotation,
		Health:    ctx.Health,
		Hunger:    ctx.Hunger,
		Inventory: model.Inventory{Slots: slots},
		World:     ctx.World,
	})
}

func withEntities(ctx model.TickContext, entities ...model.PerceivedEntity) model.TickContext {
	return model.NewTickContext(model.TickContextInput{
		Tick:     ctx.Tick,
		Bot:      model.Bot{ProfileID: ctx.ProfileID, Username: ctx.Username},
		Session:  ctx.Session,
		Position: ctx.Position,
		Rotation: ctx.Rotation,
		Health:   ctx.Health,
		Hunger:   ctx.Hunger,
		Entities: entities,
		World:    ctx.World,
	})
}

func withReality(ctx model.TickContext, reality model.RealityState) model.TickContext {
	return model.NewTickContext(model.TickContextInput{
		Tick:      ctx.Tick,
		Bot:       model.Bot{ProfileID: ctx.ProfileID, Username: ctx.Username},
		Session:   ctx.Session,
		Position:  ctx.Position,
		Rotation:  ctx.Rotation,
		Health:    ctx.Health,
		Hunger:    ctx.Hunger,
		Inventory: ctx.Inventory.ToInventory(),
		World:     ctx.World,
		Reality:   &reality,
	})
}

func TestTickContextSnapshotsMutableInputs(t *testing.T) {
	item := &model.ItemStack{ID: 800, Name: "apple", Count: 1}
	entities := []model.PerceivedEntity{{ID: 42, Name: "zombie", Position: model.Position{X: 2}}}
	blocks := []model.PerceptionBlock{{Position: model.BlockPosition{X: 3}, Name: "oak_log"}}
	gotoTarget := model.BlockPosition{X: 4}

	ctx := model.NewTickContext(model.TickContextInput{
		Tick:      11,
		Bot:       model.Bot{ProfileID: model.ProfileID{9}, Username: "snapshot-bot"},
		Session:   model.Session{Phase: model.SessionPlayReady, AttemptID: 3, PlayerEntityID: 12},
		Position:  model.Position{X: 1},
		Health:    model.Health{Current: 10, Max: 20},
		Hunger:    model.Hunger{Current: 7, Max: 20},
		Inventory: model.Inventory{Slots: []model.InventorySlot{{Slot: 1, Item: item}}},
		Entities:  entities,
		Blocks:    blocks,
		Reality: &model.RealityState{
			GotoTarget:   &gotoTarget,
			ActionFailed: true,
			Failure:      "goto rejected",
		},
	})

	item.Name = "stone"
	entities[0].Name = "skeleton"
	blocks[0].Name = "diamond_ore"
	gotoTarget.X = 99

	if got := ctx.Inventory.Slots[0].Item.Name; got != "apple" {
		t.Fatalf("inventory snapshot item = %q, want apple", got)
	}
	if got := ctx.Entities[0].Name; got != "zombie" {
		t.Fatalf("entity snapshot = %q, want zombie", got)
	}
	if got := ctx.Blocks[0].Name; got != "oak_log" {
		t.Fatalf("block snapshot = %q, want oak_log", got)
	}
	if got := ctx.Reality.GotoTarget.X; got != 4 {
		t.Fatalf("reality snapshot target X = %d, want 4", got)
	}
	if !ctx.Reality.ActionFailed || ctx.Reality.Failure != "goto rejected" {
		t.Fatalf("reality snapshot failure = %#v, want action failure feedback", ctx.Reality)
	}

	inventoryCopy := ctx.Inventory.ToInventory()
	inventoryCopy.Slots[0].Item.Name = "changed"
	if got := ctx.Inventory.Slots[0].Item.Name; got != "apple" {
		t.Fatalf("context inventory changed through derived inventory: %q", got)
	}
}

func TestBehaviorCatalogMapsOnlyExecutableGoalsInDeterministicOrder(t *testing.T) {
	catalog := NewBehaviorCatalog()
	want := []model.GoalType{model.Flee, model.Fight, model.Eat, model.CraftTool, model.GatherResource, model.Idle}

	if got := catalog.Goals(); !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog goals = %v, want %v", got, want)
	}
	for _, goal := range want {
		behavior, ok := catalog.Behavior(goal)
		if !ok || behavior.Goal() != goal {
			t.Fatalf("catalog behavior for %v = (%T, %t), want matching executable behavior", goal, behavior, ok)
		}
		if reflect.TypeOf(behavior).Elem().NumField() != 0 {
			t.Fatalf("behavior %T has fields; per-bot lifecycle belongs in UtilityAIState", behavior)
		}
	}
	for _, placeholder := range []model.GoalType{model.FindFood, model.Hunt, model.ReturnToShelter} {
		if behavior, ok := catalog.Behavior(placeholder); ok || behavior != nil {
			t.Fatalf("placeholder %v unexpectedly has executable behavior %T", placeholder, behavior)
		}
	}
}

func TestRunnerReplacesInvalidGoalWithBestEligibleGoal(t *testing.T) {
	ctx := behaviorTestContext(40)
	initial := model.UtilityAIState{CurrentGoal: model.Fight, Phase: model.GoalPhaseExecuting}
	runner := NewBehaviorRunner(NewBehaviorCatalog())

	result := runner.Reconcile(ctx, initial)

	if result.State.CurrentGoal != model.Idle || result.State.Phase != model.GoalPhaseExecuting {
		t.Fatalf("state = %#v, want idle executing when Fight is invalid", result.State)
	}
	if result.State.LastExitReason != model.GoalExitCancelled {
		t.Fatalf("last exit = %v, want cancelled", result.State.LastExitReason)
	}
	if initial.Phase != model.GoalPhaseExecuting {
		t.Fatalf("runner mutated input UtilityAIState: %#v", initial)
	}

}

func TestRunnerPreemptsIdleGotoForHigherScoringGatherAndFight(t *testing.T) {
	runner := NewBehaviorRunner(NewBehaviorCatalog())
	idleTarget := model.GoalTarget{Kind: model.GoalTargetDestination, Destination: model.Position{X: 32, Y: 64, Z: 32}}
	initial := model.UtilityAIState{
		CurrentGoal: model.Idle,
		Phase:       model.GoalPhaseExecuting,
		Target:      idleTarget,
	}

	gatherCtx := behaviorTestContext(50)
	gatherCtx.World.HasWanderTarget = true
	gatherCtx.World.WanderDestination = model.BlockPosition{X: 32, Y: 64, Z: 32}
	gatherCtx.Blocks[0] = model.PerceptionBlock{Position: model.BlockPosition{X: 4, Y: 64, Z: 4}, Name: "oak_log", Distance: 2}
	gatherCtx.BlockCount = 1
	gather := runner.Reconcile(gatherCtx, initial)
	if gather.State.CurrentGoal != model.GatherResource || gather.Decision.Action != model.ControllerActionBreak {
		t.Fatalf("gather result = %#v, want selected gather break", gather)
	}
	if gather.Decision.MovementTarget.Kind != model.GoalTargetBlock || gather.Decision.MovementTarget.Block != gather.Decision.Target.Block {
		t.Fatalf("gather movement = %#v, want movement to gathered block", gather.Decision)
	}

	fightCtx := withEntities(behaviorTestContext(51), model.PerceivedEntity{ID: 77, Name: "zombie", Position: model.Position{X: 13, Y: 64, Z: 10}, Distance: 3})
	fight := runner.Reconcile(fightCtx, initial)
	if fight.State.CurrentGoal != model.Fight || fight.Decision.Action != model.ControllerActionAttack {
		t.Fatalf("fight result = %#v, want selected fight attack", fight)
	}
	if fight.Decision.MovementTarget.Kind != model.GoalTargetDestination {
		t.Fatalf("fight movement = %#v, want goto near hostile", fight.Decision)
	}
}

func TestRunnerRetainsIdleDestinationUntilArrivalThenSelectsWinner(t *testing.T) {
	runner := NewBehaviorRunner(NewBehaviorCatalog())
	stable := model.GoalTarget{Kind: model.GoalTargetDestination, Destination: model.Position{X: 20, Y: 64, Z: 20}}
	state := model.UtilityAIState{CurrentGoal: model.Idle, Phase: model.GoalPhaseExecuting, Target: stable}
	ctx := behaviorTestContext(60)
	ctx.World.HasWanderTarget = true
	ctx.World.WanderDestination = model.BlockPosition{X: 90, Y: 64, Z: 90}

	retained := runner.Reconcile(ctx, state)
	if retained.Decision.Target != stable {
		t.Fatalf("idle target = %#v, want retained %#v", retained.Decision.Target, stable)
	}

	arrival := 1.0
	ctx = withReality(ctx, model.RealityState{GotoTarget: &model.BlockPosition{X: 20, Y: 64, Z: 20}, ArrivalDistance: &arrival})
	ctx.Blocks[0] = model.PerceptionBlock{Position: model.BlockPosition{X: 3, Y: 64, Z: 3}, Name: "oak_log", Distance: 2}
	ctx.BlockCount = 1
	next := runner.Reconcile(ctx, state)
	if next.State.CurrentGoal != model.GatherResource || next.Decision.Action != model.ControllerActionBreak {
		t.Fatalf("arrival result = %#v, want same-tick gather selection", next)
	}
}

func TestRunnerArrivalReleasesIdleDestinationForNewWanderTarget(t *testing.T) {
	runner := NewBehaviorRunner(NewBehaviorCatalog())
	old := model.GoalTarget{Kind: model.GoalTargetDestination, Destination: model.Position{X: 20, Y: 64, Z: 20}}
	state := model.UtilityAIState{CurrentGoal: model.Idle, Phase: model.GoalPhaseExecuting, Target: old}
	ctx := behaviorTestContext(61)
	ctx.World.HasWanderTarget = true
	ctx.World.WanderDestination = model.BlockPosition{X: 90, Y: 64, Z: 90}

	withoutArrival := runner.Reconcile(ctx, state)
	if withoutArrival.Decision.Target != old {
		t.Fatalf("no-arrival target = %#v, want retained %#v", withoutArrival.Decision.Target, old)
	}

	distance := 1.0
	oldBlock := model.BlockPosition{X: 20, Y: 64, Z: 20}
	withArrival := runner.Reconcile(withReality(ctx, model.RealityState{GotoTarget: &oldBlock, ArrivalDistance: &distance}), state)
	want := model.GoalTarget{Kind: model.GoalTargetDestination, Destination: model.Position{X: 90, Y: 64, Z: 90}}
	if withArrival.Decision.Target != want {
		t.Fatalf("arrival target = %#v, want fresh wander %#v", withArrival.Decision.Target, want)
	}
}

func TestRunnerFailedCraftPlanIgnoresWorldRevisionButRetriesAfterInventoryChange(t *testing.T) {
	runner := NewBehaviorRunner(NewBehaviorCatalog())
	ctx := withInventory(behaviorTestContext(62), model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 1}})
	decision := BehaviorDecision{Goal: model.CraftTool, Action: model.ControllerActionCraft, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "oak_planks"}, CraftCount: 1}
	hash := DecisionPreconditionsHash(ctx, decision)
	state := model.UtilityAIState{FailedPlans: model.FailedPlanCache{Entries: [model.FailedPlanCacheCapacity]model.FailedPlan{{Goal: model.CraftTool, Action: model.ControllerActionCraft, Target: decision.Target, PreconditionsHash: hash, CraftCount: 1}}, Count: 1}}

	ctx.World.PreconditionsHash++
	blocked := runner.Reconcile(ctx, state)
	if blocked.Decision.Action == model.ControllerActionCraft {
		t.Fatalf("world revision retried failed craft: %#v", blocked)
	}

	ctx = withInventory(ctx, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 2}})
	retried := runner.Reconcile(ctx, state)
	if retried.Decision.Action != model.ControllerActionCraft {
		t.Fatalf("relevant inventory change did not retry craft: %#v", retried)
	}
}

func TestDecisionPreconditionsHashUsesOnlyActionRelevantInputs(t *testing.T) {
	ctx := withInventory(behaviorTestContext(63),
		model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "apple", Count: 1}},
		model.InventorySlot{Slot: 1, Item: &model.ItemStack{Name: "dirt", Count: 1}},
	)
	craft := BehaviorDecision{Goal: model.CraftTool, Action: model.ControllerActionCraft, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "oak_planks"}}
	craftHungry := ctx
	craftHungry.Hunger.Current = 1
	if DecisionPreconditionsHash(ctx, craft) != DecisionPreconditionsHash(craftHungry, craft) {
		t.Fatal("unrelated hunger revived craft")
	}
	craftDirt := withInventory(ctx, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "apple", Count: 1}}, model.InventorySlot{Slot: 1, Item: &model.ItemStack{Name: "dirt", Count: 2}})
	if DecisionPreconditionsHash(ctx, craft) != DecisionPreconditionsHash(craftDirt, craft) {
		t.Fatal("unrelated inventory revived craft")
	}
	consume := BehaviorDecision{Goal: model.Eat, Action: model.ControllerActionConsume, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "apple"}}
	changedOther := withInventory(ctx, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "apple", Count: 1}}, model.InventorySlot{Slot: 1, Item: &model.ItemStack{Name: "dirt", Count: 2}})
	if DecisionPreconditionsHash(ctx, consume) != DecisionPreconditionsHash(changedOther, consume) {
		t.Fatal("unrelated inventory revived consume")
	}
	changedFood := withInventory(ctx, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "apple", Count: 2}}, model.InventorySlot{Slot: 1, Item: &model.ItemStack{Name: "dirt", Count: 1}})
	if DecisionPreconditionsHash(ctx, consume) == DecisionPreconditionsHash(changedFood, consume) {
		t.Fatal("relevant consumable change did not revive consume")
	}
}

func TestRunnerCompletedOneShotDoesNotRepeatUntilPreconditionsChange(t *testing.T) {
	ctx := withInventory(behaviorTestContext(64), model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 1}})
	decision := BehaviorDecision{Goal: model.CraftTool, Action: model.ControllerActionCraft, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "oak_planks"}, CraftCount: 1}
	hash := DecisionPreconditionsHash(ctx, decision)
	state := model.UtilityAIState{CompletedPlans: model.FailedPlanCache{Entries: [model.FailedPlanCacheCapacity]model.FailedPlan{{Goal: model.CraftTool, Action: model.ControllerActionCraft, Target: decision.Target, PreconditionsHash: hash, CraftCount: 1}}, Count: 1}}
	if got := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(ctx, state); got.Decision.Action == model.ControllerActionCraft {
		t.Fatalf("completed craft repeated: %#v", got)
	}
	changed := withInventory(ctx, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 2}})
	if got := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(changed, state); got.Decision.Action != model.ControllerActionCraft {
		t.Fatalf("changed craft inputs did not re-enable action: %#v", got)
	}
}

func TestRunnerCompletedPlanForgetsHistoricalFingerprintAfterChange(t *testing.T) {
	ctx := withInventory(behaviorTestContext(66), model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 1}})
	d := BehaviorDecision{Goal: model.CraftTool, Action: model.ControllerActionCraft, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "oak_planks"}}
	a := DecisionPreconditionsHash(ctx, d)
	state := model.UtilityAIState{CompletedPlans: model.FailedPlanCache{Entries: [model.FailedPlanCacheCapacity]model.FailedPlan{{Goal: d.Goal, Action: d.Action, Target: d.Target, PreconditionsHash: a}}, Count: 1}}
	bctx := withInventory(ctx, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 2}})
	NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(bctx, state)
	if state.CompletedPlans.Len() != 1 {
		t.Fatal("runner mutated input")
	}
	result := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(bctx, state)
	if result.State.CompletedPlans.Len() != 0 {
		t.Fatalf("changed fingerprint did not invalidate completion: %#v", result.State.CompletedPlans)
	}
	if got := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(ctx, result.State); got.Decision.Action != model.ControllerActionCraft {
		t.Fatalf("A fingerprint remained suppressed after A→B→A: %#v", got)
	}
}

func TestCraftFingerprintUsesRecursiveRecipeInputsAndNotSlots(t *testing.T) {
	d := BehaviorDecision{Goal: model.CraftTool, Action: model.ControllerActionCraft, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "stone_pickaxe"}}
	a := withInventory(behaviorTestContext(67), model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "cobblestone", Count: 3}}, model.InventorySlot{Slot: 1, Item: &model.ItemStack{Name: "stick", Count: 2}}, model.InventorySlot{Slot: 2, Item: &model.ItemStack{Name: "dirt", Count: 1}})
	moved := withInventory(a, model.InventorySlot{Slot: 7, Item: &model.ItemStack{Name: "cobblestone", Count: 3}}, model.InventorySlot{Slot: 8, Item: &model.ItemStack{Name: "stick", Count: 2}}, model.InventorySlot{Slot: 2, Item: &model.ItemStack{Name: "dirt", Count: 9}})
	if DecisionPreconditionsHash(a, d) != DecisionPreconditionsHash(moved, d) {
		t.Fatal("slot move or dirt changed craft fingerprint")
	}
	changed := withInventory(a, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "cobblestone", Count: 2}}, model.InventorySlot{Slot: 1, Item: &model.ItemStack{Name: "stick", Count: 2}})
	if DecisionPreconditionsHash(a, d) == DecisionPreconditionsHash(changed, d) {
		t.Fatal("cobblestone change did not change stone pickaxe fingerprint")
	}
}

func TestPlaceFingerprintUsesPlaceItemAvailability(t *testing.T) {
	d := BehaviorDecision{Goal: model.Idle, Action: model.ControllerActionPlace, Target: model.GoalTarget{Kind: model.GoalTargetBlock, Block: model.BlockPosition{X: 1}}, PlaceItem: "oak_planks"}
	a := withInventory(behaviorTestContext(68), model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_planks", Count: 1}})
	b := withInventory(a, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_planks", Count: 2}})
	if DecisionPreconditionsHash(a, d) == DecisionPreconditionsHash(b, d) {
		t.Fatal("place item count did not change fingerprint")
	}
}

func TestRunnerInvalidatesCompletedConsumeWhileItIsIneligible(t *testing.T) {
	ctx := withInventory(behaviorTestContext(69), model.InventorySlot{Slot: 0, Item: &model.ItemStack{ID: 800, Name: "apple", Count: 1}})
	ctx.Hunger = model.Hunger{Current: 1, Max: 20}
	d := BehaviorDecision{Goal: model.Eat, Action: model.ControllerActionConsume, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "apple"}}
	state := model.UtilityAIState{CompletedPlans: model.FailedPlanCache{Entries: [model.FailedPlanCacheCapacity]model.FailedPlan{{Goal: d.Goal, Action: d.Action, Target: d.Target, PreconditionsHash: DecisionPreconditionsHash(ctx, d)}}, Count: 1}}
	full := ctx
	full.Hunger.Current = full.Hunger.Max
	changed := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(full, state)
	if changed.State.CompletedPlans.Len() != 0 {
		t.Fatalf("ineligible consume did not invalidate old completion: %#v", changed.State.CompletedPlans)
	}
	if got := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(ctx, changed.State); got.Decision.Action != model.ControllerActionConsume {
		t.Fatalf("A→B→A consume remained suppressed: %#v", got)
	}
}

func TestRunnerInvalidatesCachedConsumeWhenOriginalItemIsUnavailable(t *testing.T) {
	ctx := withInventory(behaviorTestContext(71), model.InventorySlot{Slot: 0, Item: &model.ItemStack{ID: 800, Name: "apple", Count: 1}})
	ctx.Hunger.Current = 1
	d := BehaviorDecision{Goal: model.Eat, Action: model.ControllerActionConsume, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "apple"}}
	state := model.UtilityAIState{CompletedPlans: model.FailedPlanCache{Entries: [model.FailedPlanCacheCapacity]model.FailedPlan{{Goal: d.Goal, Action: d.Action, Target: d.Target, PreconditionsHash: DecisionPreconditionsHash(ctx, d)}}, Count: 1}}
	bread := withInventory(ctx, model.InventorySlot{Slot: 0, Item: &model.ItemStack{ID: 800, Name: "bread", Count: 1}})
	changed := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(bread, state)
	if changed.State.CompletedPlans.Len() != 0 {
		t.Fatalf("unavailable apple did not invalidate cache: %#v", changed.State.CompletedPlans)
	}
	if got := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(ctx, changed.State); got.Decision.Action != model.ControllerActionConsume {
		t.Fatalf("restored apple stayed suppressed: %#v", got)
	}
}

func TestCraftFingerprintIncludesCraftCount(t *testing.T) {
	ctx := withInventory(behaviorTestContext(70), model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 1}})
	a := BehaviorDecision{Goal: model.CraftTool, Action: model.ControllerActionCraft, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "oak_planks"}, CraftCount: 4}
	b := a
	b.CraftCount = 8
	if DecisionPreconditionsHash(ctx, a) == DecisionPreconditionsHash(ctx, b) {
		t.Fatal("craft count did not affect decision fingerprint")
	}
}

func TestRunnerFeedbackExitReasonIsOnlyForThatTick(t *testing.T) {
	for _, reason := range []model.GoalExitReason{model.GoalExitFailed, model.GoalExitCompleted} {
		ctx := withEntities(withInventory(behaviorTestContext(65), model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 1}}), model.PerceivedEntity{ID: 7, Name: "zombie", Position: model.Position{X: 1}, Distance: 1})
		state := model.UtilityAIState{CurrentGoal: model.CraftTool, Phase: model.GoalPhaseExecuting}
		first := NewBehaviorRunner(NewBehaviorCatalog()).ReconcileWithFeedback(ctx, state, reason)
		if first.State.LastExitReason != reason {
			t.Fatalf("same tick reason = %v, want %v", first.State.LastExitReason, reason)
		}
		ctx.Health = model.Health{Current: 1, Max: 20}
		later := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(ctx, first.State)
		if later.State.LastExitReason != model.GoalExitCancelled {
			t.Fatalf("later transition reason = %v, want cancelled", later.State.LastExitReason)
		}
	}
}

func TestRunnerSelectsEmergencyFleeForRecentHostileAtThirtyPercentHealth(t *testing.T) {
	ctx := behaviorTestContext(100)
	ctx.Health = model.Health{Current: 6, Max: 20}
	state := model.UtilityAIState{
		RecentHostiles:     [model.RecentHostileMemoryCapacity]model.HostileMemory{{EntityID: 7, Position: model.Position{X: 4}, SeenTick: 80}},
		RecentHostileCount: 1,
	}

	result := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(ctx, state)

	if result.State.CurrentGoal != model.Flee || result.State.Phase != model.GoalPhaseExecuting {
		t.Fatalf("state = %#v, want Flee executing for an emergency threat", result.State)
	}
	if result.Decision.Goal != model.Flee || result.Decision.Action != model.ControllerActionGoto {
		t.Fatalf("decision = %#v, want flee goto intent", result.Decision)
	}
}

func TestRunnerEmergencyFleePreemptsExecutingGoals(t *testing.T) {
	tests := []struct {
		name  string
		goal  model.GoalType
		setup func(model.TickContext) model.TickContext
	}{
		{
			name: "eat",
			goal: model.Eat,
			setup: func(ctx model.TickContext) model.TickContext {
				ctx = withInventory(ctx, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "apple", Count: 1}})
				ctx.Hunger = model.Hunger{Current: 1, Max: 20}
				return ctx
			},
		},
		{
			name: "craft",
			goal: model.CraftTool,
			setup: func(ctx model.TickContext) model.TickContext {
				return withInventory(ctx, model.InventorySlot{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 1}})
			},
		},
		{
			name: "fight",
			goal: model.Fight,
			setup: func(ctx model.TickContext) model.TickContext {
				return withEntities(ctx, model.PerceivedEntity{ID: 7, Name: "zombie", Position: model.Position{X: 4}, Distance: 6})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup(behaviorTestContext(100))
			ctx.Health = model.Health{Current: 6, Max: 20}
			state := model.UtilityAIState{
				CurrentGoal:        tt.goal,
				Phase:              model.GoalPhaseExecuting,
				RecentHostiles:     [model.RecentHostileMemoryCapacity]model.HostileMemory{{EntityID: 7, Position: model.Position{X: 4}, SeenTick: 95}},
				RecentHostileCount: 1,
			}

			result := NewBehaviorRunner(NewBehaviorCatalog()).Reconcile(ctx, state)

			if result.State.CurrentGoal != model.Flee || result.State.Phase != model.GoalPhaseExecuting {
				t.Fatalf("state = %#v, want Flee executing", result.State)
			}
			if result.Decision.Goal != model.Flee || result.Decision.Action != model.ControllerActionGoto {
				t.Fatalf("decision = %#v, want fresh flee goto decision", result.Decision)
			}
		})
	}
}
