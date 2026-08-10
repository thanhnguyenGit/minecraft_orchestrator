package ai

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestEvaluateUtilityTraceReportsTheAppliedArbitrationWinner(t *testing.T) {
	ctx := model.NewTickContext(model.TickContextInput{
		Tick:    17,
		Session: model.Session{Phase: model.SessionPlayReady},
		Health:  model.Health{Current: 20, Max: 20},
		Hunger:  model.Hunger{Current: 20, Max: 20},
		Entities: []model.PerceivedEntity{{
			ID:       9,
			Name:     "zombie",
			Position: model.Position{X: 0, Y: 64, Z: -1},
			Distance: 1,
		}},
		Blocks: []model.PerceptionBlock{{
			Position: model.BlockPosition{X: 0, Y: 64, Z: -2},
			Name:     "oak_log",
			Distance: 2,
		}},
	})
	state := model.UtilityAIState{
		CurrentGoal: model.Idle,
		Phase:       model.GoalPhaseExecuting,
	}

	trace := EvaluateUtilityTrace(ctx, state)

	if trace.ReconcileGate != UtilityTraceGateArbitrated {
		t.Fatalf("gate = %q, want arbitrated", trace.ReconcileGate)
	}
	if trace.Lifecycle != model.GoalPhaseExecuting || trace.RetainedGoal != model.Idle {
		t.Fatalf("lifecycle/retained = %#v, want executing idle", trace)
	}
	if len(trace.Goals) != 6 {
		t.Fatalf("goal traces = %#v, want all six active goals", trace.Goals)
	}
	if trace.WinnerGoal != model.Fight || trace.WinnerScore <= trace.RetainedScore {
		t.Fatalf("winner/retained = %#v, want higher fight winner than idle", trace)
	}
	if trace.HostileCount != 1 || trace.NearestHostileDistance != 1 || trace.Threat != 1 || trace.RecentHostileCount != 1 {
		t.Fatalf("hostile facts = %#v, want one adjacent hostile", trace)
	}
	if trace.ResourceCandidate == nil || trace.ResourceCandidate.Name != "oak_log" || trace.MineableResourceCount != 1 {
		t.Fatalf("resource facts = %#v, want visible mineable oak log", trace)
	}
}
