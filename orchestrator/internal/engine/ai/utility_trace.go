package ai

import (
	"math"

	"minecraft_orchestrator/internal/engine/model"
)

// UtilityTraceGate names the reconciliation condition that prevents or permits
// the normal lifecycle step. It is diagnostic only and never influences goal
// selection.
type UtilityTraceGate string

const (
	UtilityTraceGateInactive   UtilityTraceGate = "inactive"
	UtilityTraceGateEntering   UtilityTraceGate = "entering"
	UtilityTraceGateExecuting  UtilityTraceGate = "executing"
	UtilityTraceGateArbitrated UtilityTraceGate = "arbitrated"
)

// UtilityGoalTrace is the score and entry eligibility for one executable goal.
type UtilityGoalTrace struct {
	Goal     model.GoalType
	Score    float64
	Eligible bool
}

// UtilityTraceSnapshot is a bounded, value-only view of a utility evaluation.
// It deliberately has no controller or lifecycle mutation methods.
type UtilityTraceSnapshot struct {
	Lifecycle              model.GoalLifecyclePhase
	ReconcileGate          UtilityTraceGate
	Goals                  []UtilityGoalTrace
	WinnerGoal             model.GoalType
	WinnerScore            float64
	RetainedGoal           model.GoalType
	RetainedScore          float64
	RetainedEligible       bool
	HostileCount           int
	NearestHostileDistance float64
	Threat                 float64
	RecentHostileCount     uint8
	ResourceCandidate      *model.PerceptionBlock
	MineableResourceCount  int
}

// EvaluateUtilityTrace evaluates the same six executable behaviors used by
// BehaviorRunner without reconciling state or making an action decision.
func EvaluateUtilityTrace(ctx model.TickContext, state model.UtilityAIState) UtilityTraceSnapshot {
	state = rememberHostiles(ctx, state)
	ctx = contextWithRecentHostiles(ctx, state)
	trace, _, _, _ := NewBehaviorRunner(NewBehaviorCatalog()).selectBest(ctx, state)

	inv := ctx.Inventory.ToInventory()
	for _, hostile := range visibleHostiles(ctx) {
		trace.HostileCount++
		trace.Threat += 1 / math.Max(hostile.Distance, 0.5)
		if trace.NearestHostileDistance < 0 || hostile.Distance < trace.NearestHostileDistance {
			trace.NearestHostileDistance = hostile.Distance
		}
	}
	for _, block := range contextBlocks(ctx) {
		if !IsResource(block.Name) || !CanMine(block.Name, inv) {
			continue
		}
		trace.MineableResourceCount++
		if trace.ResourceCandidate == nil || block.Distance < trace.ResourceCandidate.Distance {
			candidate := block
			trace.ResourceCandidate = &candidate
		}
	}

	return trace
}
