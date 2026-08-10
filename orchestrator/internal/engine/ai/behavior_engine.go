package ai

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/mc_protocol/registry"
)

const recentHostileWindowTicks = 20

// GoalBehavior is a stateless implementation of one executable utility goal.
// Per-bot lifecycle and pending-action state always remain in UtilityAIState.
type GoalBehavior interface {
	Goal() model.GoalType
	Score(model.TickContext) float64
	CanEnter(model.TickContext) bool
	CanContinue(model.TickContext) bool
	Enter(model.TickContext) BehaviorDecision
	Step(model.TickContext) BehaviorDecision
}

// BehaviorDecision is an engine-local intent. Controller synchronization is
// deliberately outside this package's runner and can translate this value in a
// later system integration step.
type BehaviorDecision struct {
	Goal           model.GoalType
	Target         model.GoalTarget
	MovementTarget model.GoalTarget
	Action         model.ControllerAction
	CraftCount     int32
	PlaceItem      string
}

// BehaviorCatalog is an ordered registry of only executable behaviors. Its
// order is deterministic and matches utility goal selection priority.
type BehaviorCatalog struct {
	behaviors [6]GoalBehavior
}

// NewBehaviorCatalog returns fresh stateless behavior values. FindFood, Hunt,
// and ReturnToShelter intentionally have no catalog entry yet.
func NewBehaviorCatalog() BehaviorCatalog {
	return BehaviorCatalog{
		behaviors: [6]GoalBehavior{
			&fleeBehavior{},
			&fightBehavior{},
			&eatBehavior{},
			&craftToolBehavior{},
			&gatherResourceBehavior{},
			&idleBehavior{},
		},
	}
}

func (c BehaviorCatalog) Goals() []model.GoalType {
	goals := make([]model.GoalType, 0, len(c.behaviors))
	for _, behavior := range c.behaviors {
		if behavior != nil {
			goals = append(goals, behavior.Goal())
		}
	}
	return goals
}

func (c BehaviorCatalog) Behavior(goal model.GoalType) (GoalBehavior, bool) {
	for _, behavior := range c.behaviors {
		if behavior != nil && behavior.Goal() == goal {
			return behavior, true
		}
	}

	return nil, false
}

// Behaviour engine, it reconciles a TickContext with one ECS-owned UtilityAIState.
// It retains no bot-indexed state and is safe to use independently per entity.
type BehaviorRunner struct {
	catalog BehaviorCatalog
}

func NewBehaviorRunner(catalog BehaviorCatalog) BehaviorRunner {
	return BehaviorRunner{catalog: catalog}
}

// ReconcileResult returns a copied next state plus the intent generated for the
// current lifecycle step.
type ReconcileResult struct {
	State    model.UtilityAIState
	Decision BehaviorDecision
	Trace    UtilityTraceSnapshot
}

// Reconcile arbitrates all executable goals on every tick. Controller delivery
// is intentionally outside this selector: Goto, Attack, and Break are desired
// state fields, while one-shot delivery correlation belongs to ControllerSync.
func (r BehaviorRunner) Reconcile(ctx model.TickContext, current model.UtilityAIState) ReconcileResult {
	return r.ReconcileWithFeedback(ctx, current, model.GoalExitNone)
}

// ReconcileWithFeedback applies a controller outcome only for this arbitration
// tick; persisted LastExitReason is diagnostic state, not a future gate.
func (r BehaviorRunner) ReconcileWithFeedback(ctx model.TickContext, current model.UtilityAIState, feedbackReason model.GoalExitReason) ReconcileResult {
	next := current
	next = rememberHostiles(ctx, next)
	ctx = contextWithRecentHostiles(ctx, next)
	r.invalidateStaleOneShotPlans(ctx, &next)
	trace, goal, decision, ok := r.selectBest(ctx, next)
	if !ok {
		next.CurrentGoal = model.Idle
		next.Phase = model.GoalPhaseInactive
		next.Target = model.GoalTarget{}
		return ReconcileResult{State: next, Trace: trace}
	}
	previous := next.CurrentGoal
	feedbackExit := feedbackReason == model.GoalExitFailed || feedbackReason == model.GoalExitCompleted
	next.CurrentGoal = goal
	next.Phase = model.GoalPhaseExecuting
	applyDecision(&next, decision)
	if isOneShotAction(decision.Action) {
		hash := DecisionPreconditionsHash(ctx, decision)
		invalidateStalePlans(&next.CompletedPlans, decision, hash)
		invalidateStalePlans(&next.FailedPlans, decision, hash)
	}
	if feedbackExit {
		next.LastExitReason = feedbackReason
	} else if previous != goal {
		next.LastExitReason = model.GoalExitCancelled
	} else if !feedbackExit {
		next.LastExitReason = model.GoalExitNone
	}
	return ReconcileResult{State: next, Decision: decision, Trace: trace}
}

func isOneShotAction(action model.ControllerAction) bool {
	switch action {
	case model.ControllerActionCraft, model.ControllerActionEquip, model.ControllerActionConsume, model.ControllerActionPlace:
		return true
	default:
		return false
	}
}

// Emergency Flee trigger when health drop below 30%
func emergencyFlee(ctx model.TickContext, state model.UtilityAIState) bool {
	if state.CurrentGoal == model.Flee {
		return false
	}

	if state.Phase != model.GoalPhaseEntering && state.Phase != model.GoalPhaseExecuting {
		return false
	}

	return percentage(ctx.Health.Current, ctx.Health.Max) <= 0.30 && hasRecentHostile(ctx)
}

func (r BehaviorRunner) selectBest(ctx model.TickContext, state model.UtilityAIState) (UtilityTraceSnapshot, model.GoalType, BehaviorDecision, bool) {
	trace := UtilityTraceSnapshot{Lifecycle: state.Phase, RetainedGoal: state.CurrentGoal, RecentHostileCount: state.RecentHostileCount, NearestHostileDistance: -1}
	scores := make(map[GoalType]float64, len(r.catalog.behaviors))
	decisions := make(map[model.GoalType]BehaviorDecision, len(r.catalog.behaviors))
	for _, behavior := range r.catalog.behaviors {
		if behavior == nil {
			continue
		}
		eligible := behavior.CanEnter(ctx)
		score := behavior.Score(ctx)
		trace.Goals = append(trace.Goals, UtilityGoalTrace{Goal: behavior.Goal(), Score: score, Eligible: eligible})
		if behavior.Goal() == state.CurrentGoal {
			trace.RetainedScore, trace.RetainedEligible = score, eligible
		}
		if !eligible {
			continue
		}
		decision := stableDecision(ctx, state, behavior)
		hash := DecisionPreconditionsHash(ctx, decision)
		if failedPlan(state.FailedPlans, behavior.Goal(), decision.Target, hash) || (isOneShotAction(decision.Action) && failedPlan(state.CompletedPlans, behavior.Goal(), decision.Target, hash)) {
			continue
		}
		scores[behavior.Goal()] = score
		decisions[behavior.Goal()] = decision
	}
	trace.WinnerGoal = SelectGoal(scores, model.Idle)
	trace.WinnerScore = scores[trace.WinnerGoal]
	trace.ReconcileGate = UtilityTraceGateArbitrated
	decision, ok := decisions[trace.WinnerGoal]
	return trace, trace.WinnerGoal, decision, ok
}

// DecisionPreconditionsHash fingerprints the inputs that can make a one-shot
// decision executable. World revision is deliberately excluded: a chunk or
// perception update must not revive a rejected craft/eat/equip/place command.
func DecisionPreconditionsHash(ctx model.TickContext, decision BehaviorDecision) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d/%d/%d/%d/%d/%d/%d/%s/%d", decision.Goal, decision.Action, decision.Target.Kind, decision.Target.EntityID, decision.Target.Block.X, decision.Target.Block.Y, decision.Target.Block.Z, decision.Target.Item, decision.CraftCount)
	relevant := map[string]bool{decision.Target.Item: true}
	if decision.Action == model.ControllerActionCraft {
		relevant = craftRelevantItems(decision.Target.Item)
	}
	if decision.Action == model.ControllerActionPlace {
		relevant = map[string]bool{decision.PlaceItem: true}
	}
	if decision.Action == model.ControllerActionConsume {
		_, _ = fmt.Fprintf(h, "/%f/%f", ctx.Hunger.Current, ctx.Hunger.Max)
	}
	counts := make(map[string]int32)
	for _, slot := range ctx.Inventory.Slots {
		if relevant[slot.Item.Name] {
			counts[slot.Item.Name] += slot.Item.Count
		}
	}
	names := make([]string, 0, len(relevant))
	for name := range relevant {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		_, _ = fmt.Fprintf(h, "/%s:%d", name, counts[name])
	}
	return h.Sum64()
}

func craftRelevantItems(target string) map[string]bool {
	items := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if items[name] {
			return
		}
		items[name] = true
		recipe, ok := registry.RecipeFor(name)
		if !ok {
			return
		}
		if recipe.NeedsTable {
			visit("crafting_table")
		}
		for _, v := range recipe.Ingredients {
			visit(v.ItemName)
		}
		for _, v := range recipe.Prerequisites {
			visit(v.ItemName)
		}
	}
	visit(target)
	return items
}

func invalidateStalePlans(cache *model.FailedPlanCache, decision BehaviorDecision, hash uint64) {
	out := model.FailedPlanCache{}
	for _, plan := range cache.Entries[:cache.Len()] {
		if plan.Action == model.ControllerActionNone || plan.PreconditionsHash == 0 {
			out.Add(plan)
			continue
		}
		if plan.Goal == decision.Goal && plan.Action == decision.Action && plan.Target == decision.Target && plan.PreconditionsHash != hash {
			continue
		}
		out.Add(plan)
	}
	*cache = out
}

func (r BehaviorRunner) invalidateStaleOneShotPlans(ctx model.TickContext, state *model.UtilityAIState) {
	invalidateCachedPlans(ctx, &state.CompletedPlans)
	invalidateCachedPlans(ctx, &state.FailedPlans)
}

func invalidateCachedPlans(ctx model.TickContext, cache *model.FailedPlanCache) {
	out := model.FailedPlanCache{}
	for _, plan := range cache.Entries[:cache.Len()] {
		if plan.PreconditionsHash == 0 || !isOneShotAction(plan.Action) {
			out.Add(plan)
			continue
		}
		decision := BehaviorDecision{Goal: plan.Goal, Action: plan.Action, Target: plan.Target, CraftCount: plan.CraftCount, PlaceItem: plan.PlaceItem}
		if DecisionPreconditionsHash(ctx, decision) != plan.PreconditionsHash {
			continue
		}
		out.Add(plan)
	}
	*cache = out
}

func applyDecision(state *model.UtilityAIState, decision BehaviorDecision) {
	state.Target = decision.Target
}

func failedPlan(cache model.FailedPlanCache, goal model.GoalType, target model.GoalTarget, hash uint64) bool {
	for _, plan := range cache.Entries[:cache.Len()] {
		if plan.Goal == goal && plan.Target == target && plan.PreconditionsHash == hash {
			return true
		}
	}
	return false
}

func stableDecision(ctx model.TickContext, state model.UtilityAIState, behavior GoalBehavior) BehaviorDecision {
	if state.CurrentGoal != behavior.Goal() {
		return behavior.Enter(ctx)
	}
	if state.Target.Kind == model.GoalTargetNone {
		return behavior.Step(ctx)
	}
	switch behavior.Goal() {
	case model.Idle:
		if !arrived(ctx, state.Target) {
			return BehaviorDecision{Goal: model.Idle, Target: state.Target, MovementTarget: state.Target, Action: model.ControllerActionGoto}
		}
	case model.GatherResource:
		for _, block := range contextBlocks(ctx) {
			if block.Position == state.Target.Block && IsResource(block.Name) && CanMine(block.Name, ctx.Inventory.ToInventory()) {
				return BehaviorDecision{Goal: model.GatherResource, Target: state.Target, MovementTarget: state.Target, Action: model.ControllerActionBreak}
			}
		}
	case model.Fight:
		for _, hostile := range visibleHostiles(ctx) {
			if hostile.ID == state.Target.EntityID {
				return fightDecisionFor(hostile)
			}
		}
	}
	return behavior.Step(ctx)
}

func arrived(ctx model.TickContext, target model.GoalTarget) bool {
	if !ctx.Reality.HasGotoTarget || !ctx.Reality.HasArrivalDistance || ctx.Reality.ArrivalDistance >= 2 {
		return false
	}
	return ctx.Reality.GotoTarget == blockTarget(target)
}

func contextWithRecentHostiles(ctx model.TickContext, state model.UtilityAIState) model.TickContext {
	ctx.RecentHostiles = state.RecentHostiles
	ctx.RecentHostileCount = state.RecentHostileCount
	return ctx
}

func rememberHostiles(ctx model.TickContext, state model.UtilityAIState) model.UtilityAIState {
	var memories [model.RecentHostileMemoryCapacity]model.HostileMemory
	count := 0
	limit := state.RecentHostileCount
	if limit > model.RecentHostileMemoryCapacity {
		limit = model.RecentHostileMemoryCapacity
	}
	for _, memory := range state.RecentHostiles[:limit] {
		if hostileSeenWithin(ctx.Tick, memory.SeenTick, recentHostileWindowTicks) {
			memories[count] = memory
			count++
		}
	}
	for _, entity := range visibleHostiles(ctx) {
		memory := model.HostileMemory{EntityID: entity.ID, Position: entity.Position, SeenTick: ctx.Tick}
		found := -1
		for index := 0; index < count; index++ {
			if memories[index].EntityID == memory.EntityID {
				found = index
				break
			}
		}
		if found >= 0 {
			memories[found] = memory
			continue
		}
		if count < len(memories) {
			memories[count] = memory
			count++
		}
	}
	state.RecentHostiles = memories
	state.RecentHostileCount = uint8(count)
	return state
}

func hostileSeenWithin(tick, seenTick uint64, window uint64) bool {
	return tick >= seenTick && tick-seenTick <= window
}

type idleBehavior struct{}

func (*idleBehavior) Goal() model.GoalType            { return model.Idle }
func (*idleBehavior) Score(model.TickContext) float64 { return ScoreWander() }
func (*idleBehavior) CanEnter(ctx model.TickContext) bool {
	return ctx.Session.Phase == model.SessionPlayReady
}

func (*idleBehavior) CanContinue(ctx model.TickContext) bool {
	return ctx.Session.Phase == model.SessionPlayReady
}
func (*idleBehavior) Enter(ctx model.TickContext) BehaviorDecision { return wanderDecision(ctx) }
func (*idleBehavior) Step(ctx model.TickContext) BehaviorDecision  { return wanderDecision(ctx) }

func wanderDecision(ctx model.TickContext) BehaviorDecision {
	decision := BehaviorDecision{Goal: model.Idle}
	if ctx.World.HasWanderTarget {
		decision.Action = model.ControllerActionGoto
		decision.Target = model.GoalTarget{Kind: model.GoalTargetDestination, Destination: positionForBlock(ctx.World.WanderDestination)}
		decision.MovementTarget = decision.Target
	}
	return decision
}

type eatBehavior struct{}

func (*eatBehavior) Goal() model.GoalType { return model.Eat }
func (*eatBehavior) Score(ctx model.TickContext) float64 {
	inv := ctx.Inventory.ToInventory()
	return ScoreEat(ScoringInput{HungerPct: percentage(ctx.Hunger.Current, ctx.Hunger.Max), HasFood: HasFood(inv)})
}
func (b *eatBehavior) CanEnter(ctx model.TickContext) bool        { return b.Score(ctx) > 0 }
func (b *eatBehavior) CanContinue(ctx model.TickContext) bool     { return b.CanEnter(ctx) }
func (*eatBehavior) Enter(ctx model.TickContext) BehaviorDecision { return eatDecision(ctx) }
func (*eatBehavior) Step(ctx model.TickContext) BehaviorDecision  { return eatDecision(ctx) }

func eatDecision(ctx model.TickContext) BehaviorDecision {
	food, ok := FoodInHotbar(ctx.Inventory.ToInventory())
	if !ok {
		return BehaviorDecision{Goal: model.Eat}
	}
	return BehaviorDecision{
		Goal:   model.Eat,
		Action: model.ControllerActionConsume,
		Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: food},
	}
}

type craftToolBehavior struct{}

func (*craftToolBehavior) Goal() model.GoalType { return model.CraftTool }
func (*craftToolBehavior) Score(ctx model.TickContext) float64 {
	return ScoreCraftTool(ScoringInput{Inventory: ctx.Inventory.ToInventory()})
}
func (b *craftToolBehavior) CanEnter(ctx model.TickContext) bool        { return b.Score(ctx) > 0 }
func (b *craftToolBehavior) CanContinue(ctx model.TickContext) bool     { return b.CanEnter(ctx) }
func (*craftToolBehavior) Enter(ctx model.TickContext) BehaviorDecision { return craftDecision(ctx) }
func (*craftToolBehavior) Step(ctx model.TickContext) BehaviorDecision  { return craftDecision(ctx) }

func craftDecision(ctx model.TickContext) BehaviorDecision {
	inv := ctx.Inventory.ToInventory()
	for _, target := range []string{"crafting_table", "wooden_pickaxe", "wooden_axe", "stone_pickaxe", "wooden_sword"} {
		if HasItem(inv, target) {
			continue
		}
		steps := StepsFor(target, inv)
		if len(steps) == 0 {
			continue
		}
		return BehaviorDecision{
			Goal:       model.CraftTool,
			Action:     model.ControllerActionCraft,
			Target:     model.GoalTarget{Kind: model.GoalTargetItem, Item: steps[0].ItemName},
			CraftCount: int32(steps[0].Count),
		}
	}
	return BehaviorDecision{Goal: model.CraftTool}
}

type fleeBehavior struct{}

func (*fleeBehavior) Goal() model.GoalType { return model.Flee }
func (*fleeBehavior) Score(ctx model.TickContext) float64 {
	hostiles := visibleHostiles(ctx)
	score := ScoreFlee(ScoringInput{HealthPct: percentage(ctx.Health.Current, ctx.Health.Max), Hostiles: hostiles})
	if percentage(ctx.Health.Current, ctx.Health.Max) <= 0.30 && hasRecentHostile(ctx) {
		return math.Max(score, 100)
	}
	return score
}
func (b *fleeBehavior) CanEnter(ctx model.TickContext) bool        { return b.Score(ctx) > 0 }
func (b *fleeBehavior) CanContinue(ctx model.TickContext) bool     { return b.CanEnter(ctx) }
func (*fleeBehavior) Enter(ctx model.TickContext) BehaviorDecision { return fleeDecision(ctx) }
func (*fleeBehavior) Step(ctx model.TickContext) BehaviorDecision  { return fleeDecision(ctx) }

func fleeDecision(ctx model.TickContext) BehaviorDecision {
	hostiles := visibleHostiles(ctx)
	if len(hostiles) == 0 {
		hostiles = rememberedHostiles(ctx)
	}
	destination := EscapeDestination(ctx.Position, hostiles)
	return BehaviorDecision{
		Goal:           model.Flee,
		Action:         model.ControllerActionGoto,
		Target:         model.GoalTarget{Kind: model.GoalTargetDestination, Destination: positionForBlock(destination)},
		MovementTarget: model.GoalTarget{Kind: model.GoalTargetDestination, Destination: positionForBlock(destination)},
	}
}

func positionForBlock(block model.BlockPosition) model.Position {
	return model.Position{X: float64(block.X), Y: float64(block.Y), Z: float64(block.Z)}
}

type fightBehavior struct{}

func (*fightBehavior) Goal() model.GoalType { return model.Fight }
func (*fightBehavior) Score(ctx model.TickContext) float64 {
	return ScoreFight(ScoringInput{HealthPct: percentage(ctx.Health.Current, ctx.Health.Max), Hostiles: visibleHostiles(ctx)})
}
func (b *fightBehavior) CanEnter(ctx model.TickContext) bool        { return b.Score(ctx) > 0 }
func (b *fightBehavior) CanContinue(ctx model.TickContext) bool     { return b.CanEnter(ctx) }
func (*fightBehavior) Enter(ctx model.TickContext) BehaviorDecision { return fightDecision(ctx) }
func (*fightBehavior) Step(ctx model.TickContext) BehaviorDecision  { return fightDecision(ctx) }

func fightDecision(ctx model.TickContext) BehaviorDecision {
	hostiles := visibleHostiles(ctx)
	if len(hostiles) == 0 {
		return BehaviorDecision{Goal: model.Fight}
	}
	closest := hostiles[0]
	for _, hostile := range hostiles[1:] {
		if hostile.Distance < closest.Distance {
			closest = hostile
		}
	}
	return fightDecisionFor(closest)
}

func fightDecisionFor(hostile model.PerceivedEntity) BehaviorDecision {
	return BehaviorDecision{
		Goal: model.Fight, Action: model.ControllerActionAttack,
		Target:         model.GoalTarget{Kind: model.GoalTargetEntity, EntityID: hostile.ID},
		MovementTarget: model.GoalTarget{Kind: model.GoalTargetDestination, Destination: hostile.Position},
	}
}

type gatherResourceBehavior struct{}

func (*gatherResourceBehavior) Goal() model.GoalType { return model.GatherResource }
func (*gatherResourceBehavior) Score(ctx model.TickContext) float64 {
	inv := ctx.Inventory.ToInventory()
	count := 0
	for _, block := range contextBlocks(ctx) {
		if IsResource(block.Name) && CanMine(block.Name, inv) {
			count++
		}
	}
	return ScoreGatherResource(ScoringInput{Inventory: inv, VisibleResourceCount: count})
}
func (b *gatherResourceBehavior) CanEnter(ctx model.TickContext) bool    { return b.Score(ctx) > 0 }
func (b *gatherResourceBehavior) CanContinue(ctx model.TickContext) bool { return b.CanEnter(ctx) }
func (*gatherResourceBehavior) Enter(ctx model.TickContext) BehaviorDecision {
	return gatherDecision(ctx)
}

func (*gatherResourceBehavior) Step(ctx model.TickContext) BehaviorDecision {
	return gatherDecision(ctx)
}

func gatherDecision(ctx model.TickContext) BehaviorDecision {
	inv := ctx.Inventory.ToInventory()
	for _, block := range contextBlocks(ctx) {
		if !IsResource(block.Name) || !CanMine(block.Name, inv) {
			continue
		}
		return BehaviorDecision{
			Goal:           model.GatherResource,
			Action:         model.ControllerActionBreak,
			Target:         model.GoalTarget{Kind: model.GoalTargetBlock, Block: block.Position},
			MovementTarget: model.GoalTarget{Kind: model.GoalTargetBlock, Block: block.Position},
		}
	}
	return BehaviorDecision{Goal: model.GatherResource}
}

func percentage(current, max float64) float64 {
	return current / math.Max(max, 1)
}

func blockTarget(target model.GoalTarget) model.BlockPosition {
	if target.Kind == model.GoalTargetDestination {
		return model.BlockPosition{X: int32(target.Destination.X), Y: int32(target.Destination.Y), Z: int32(target.Destination.Z)}
	}
	return target.Block
}

func visibleHostiles(ctx model.TickContext) []model.PerceivedEntity {
	hostiles := make([]model.PerceivedEntity, 0, ctx.EntityCount)
	for _, entity := range contextEntities(ctx) {
		if IsHostile(entity.Name) {
			hostiles = append(hostiles, entity)
		}
	}
	return hostiles
}

func rememberedHostiles(ctx model.TickContext) []model.PerceivedEntity {
	limit := ctx.RecentHostileCount
	if limit > model.RecentHostileMemoryCapacity {
		limit = model.RecentHostileMemoryCapacity
	}
	hostiles := make([]model.PerceivedEntity, 0, limit)
	for _, memory := range ctx.RecentHostiles[:limit] {
		if !hostileSeenWithin(ctx.Tick, memory.SeenTick, recentHostileWindowTicks) {
			continue
		}
		dx := memory.Position.X - ctx.Position.X
		dy := memory.Position.Y - ctx.Position.Y
		dz := memory.Position.Z - ctx.Position.Z
		hostiles = append(hostiles, model.PerceivedEntity{
			ID: memory.EntityID, Position: memory.Position, Distance: math.Sqrt(dx*dx + dy*dy + dz*dz),
		})
	}
	return hostiles
}

func hasRecentHostile(ctx model.TickContext) bool {
	return len(rememberedHostiles(ctx)) > 0
}

func contextEntities(ctx model.TickContext) []model.PerceivedEntity {
	count := int(ctx.EntityCount)
	if count > len(ctx.Entities) {
		count = len(ctx.Entities)
	}
	return ctx.Entities[:count]
}

func contextBlocks(ctx model.TickContext) []model.PerceptionBlock {
	count := int(ctx.BlockCount)
	if count > len(ctx.Blocks) {
		count = len(ctx.Blocks)
	}
	return ctx.Blocks[:count]
}
