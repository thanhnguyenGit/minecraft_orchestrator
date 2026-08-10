package core

import (
	"context"
	"log/slog"
	"testing"

	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/engine/scheduler"
)

func TestGoalSelectorAtomicallyReplacesIdleGotoWithGatherControls(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x47}
	stageMirroredBot(t, world, profileID)
	view := world.MirroredBotViews()[0]
	view.Sessions[0] = model.Session{Phase: model.SessionPlayReady, RemoteSessionID: "session-a"}
	old := model.BlockPosition{X: 20, Y: 64, Z: 20}
	view.UtilityAIs[0] = model.UtilityAIState{CurrentGoal: model.Idle, Phase: model.GoalPhaseExecuting, Target: model.GoalTarget{Kind: model.GoalTargetDestination, Destination: model.Position{X: 20, Y: 64, Z: 20}}}
	view.ControllerSyncs[0] = model.ControllerSyncState{Desired: model.ControllerState{GotoTarget: &old}, LastSent: model.ControllerState{GotoTarget: &old}, ControllerSequence: 6}
	block := model.BlockPosition{X: 4, Y: 64, Z: 4}
	world.Resources().PerceptionBlockView().Set(profileID, []model.PerceptionBlock{{Position: block, Name: "oak_log", Distance: 2}})

	outbox := network.NewOutbox()
	runGoalSelector(t, &GoalSelectorSystem{}, world, outbox, 19)
	intents := outbox.Drain()
	if len(intents) != 1 || intents[0].ControllerState == nil {
		t.Fatalf("intents = %#v, want one state delta", intents)
	}
	got := intents[0].ControllerState
	if got.GoToTarget == nil || *got.GoToTarget != block || got.BreakTarget == nil || *got.BreakTarget != block {
		t.Fatalf("delta = %#v, want gather goto+break", got)
	}
	if state := world.MirroredBotViews()[0].UtilityAIs[0]; state.CurrentGoal != model.GatherResource {
		t.Fatalf("selected state = %#v, want gather", state)
	}
}

func TestGoalSelectorAtomicallyReplacesIdleGotoWithFightControlsAndLogsAppliedGoal(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x48}
	stageMirroredBot(t, world, profileID)
	view := world.MirroredBotViews()[0]
	view.Sessions[0] = model.Session{Phase: model.SessionPlayReady, RemoteSessionID: "session-a"}
	old := model.BlockPosition{X: 20, Y: 64, Z: 20}
	view.UtilityAIs[0] = model.UtilityAIState{CurrentGoal: model.Idle, Phase: model.GoalPhaseExecuting, Target: model.GoalTarget{Kind: model.GoalTargetDestination, Destination: model.Position{X: 20, Y: 64, Z: 20}}}
	view.ControllerSyncs[0] = model.ControllerSyncState{Desired: model.ControllerState{GotoTarget: &old}, LastSent: model.ControllerState{GotoTarget: &old}, ControllerSequence: 6}
	world.Resources().PerceptionView().Set(profileID, []model.PerceivedEntity{{ID: 88, Name: "zombie", Position: model.Position{X: 3, Y: 64, Z: 1}, Distance: 2}})

	sink := &recordSink{}
	outbox := network.NewOutbox()
	err := (&GoalSelectorSystem{}).Run(&scheduler.RunContext{Context: context.Background(), World: world, Tick: 20, Data: &TickData{Outbox: outbox}, Logger: slog.New(sink)})
	if err != nil {
		t.Fatalf("GoalSelectorSystem.Run() error = %v", err)
	}
	intents := outbox.Drain()
	if len(intents) != 1 || intents[0].ControllerState == nil || intents[0].ControllerState.GoToTarget == nil || intents[0].ControllerState.AttackTarget == nil || *intents[0].ControllerState.AttackTarget != 88 {
		t.Fatalf("intents = %#v, want fight goto+attack", intents)
	}
	assertLogAttr(t, sink, 0, "selected_goal", "fight")
	assertLogAttr(t, sink, 0, "previous_goal", "idle")
	assertLogAttr(t, sink, 0, "winner_goal", "fight")
	assertLogAttr(t, sink, 0, "preempted", true)
}

func TestGoalSelectorIgnoresSupersededOneShotOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status model.ActionOutcomeStatus
	}{{"success", model.ActionOutcomeCompleted}, {"failure", model.ActionOutcomeFailed}} {
		t.Run(tt.name, func(t *testing.T) {
			world := enginecore.NewWorld()
			profileID := model.ProfileID{0x49}
			stageMirroredBot(t, world, profileID)
			view := world.MirroredBotViews()[0]
			view.Sessions[0] = model.Session{Phase: model.SessionPlayReady, RemoteSessionID: "session-a"}
			fightTarget := int32(88)
			view.UtilityAIs[0] = model.UtilityAIState{CurrentGoal: model.Fight, Phase: model.GoalPhaseExecuting, Target: model.GoalTarget{Kind: model.GoalTargetEntity, EntityID: fightTarget}}
			view.ControllerSyncs[0] = model.ControllerSyncState{
				Desired: model.ControllerState{AttackTarget: &fightTarget}, LastSent: model.ControllerState{AttackTarget: &fightTarget},
				InFlightOneShot: model.InFlightOneShot{Action: model.ControllerActionCraft, Correlation: 7, Goal: model.CraftTool, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "oak_planks"}, PreconditionsHash: 3},
			}
			world.Resources().PerceptionView().Set(profileID, []model.PerceivedEntity{{ID: fightTarget, Name: "zombie", Position: model.Position{X: 3, Y: 64}, Distance: 2}})
			world.Resources().RealityView().Set(profileID, model.RealityState{ActionOutcomes: []model.ActionOutcome{{ControllerSequence: 7, Action: model.ControllerActionCraft, Status: tt.status}}})
			runGoalSelector(t, &GoalSelectorSystem{}, world, network.NewOutbox(), 21)
			got := world.MirroredBotViews()[0]
			if got.UtilityAIs[0].CurrentGoal != model.Fight || got.UtilityAIs[0].FailedPlans.Len() != 0 {
				t.Fatalf("utility after stale %s = %#v, want unchanged Fight without failed plan", tt.name, got.UtilityAIs[0])
			}
			if got.ControllerSyncs[0].Desired.AttackTarget == nil || *got.ControllerSyncs[0].Desired.AttackTarget != fightTarget {
				t.Fatalf("controller after stale %s = %#v, want retained fight controls", tt.name, got.ControllerSyncs[0])
			}
		})
	}
}

func TestGoalSelectorCachesActiveOneShotFailureAndSelectsAlternative(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x50}
	stageMirroredBot(t, world, profileID)
	view := world.MirroredBotViews()[0]
	view.Sessions[0] = model.Session{Phase: model.SessionPlayReady, RemoteSessionID: "session-a"}
	craftTarget := model.GoalTarget{Kind: model.GoalTargetItem, Item: "oak_planks"}
	view.UtilityAIs[0] = model.UtilityAIState{CurrentGoal: model.CraftTool, Phase: model.GoalPhaseExecuting, Target: craftTarget}
	view.Inventorys[0] = model.Inventory{Slots: []model.InventorySlot{{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 1}}}}
	view.ControllerSyncs[0] = model.ControllerSyncState{InFlightOneShot: model.InFlightOneShot{Action: model.ControllerActionCraft, Correlation: 7, Goal: model.CraftTool, Target: craftTarget}}
	world.Resources().PerceptionView().Set(profileID, []model.PerceivedEntity{{ID: 88, Name: "zombie", Position: model.Position{X: 3, Y: 64}, Distance: 1}})
	world.Resources().RealityView().Set(profileID, model.RealityState{ActionOutcomes: []model.ActionOutcome{{ControllerSequence: 7, Action: model.ControllerActionCraft, Status: model.ActionOutcomeFailed}}})
	runGoalSelector(t, &GoalSelectorSystem{}, world, network.NewOutbox(), 22)
	got := world.MirroredBotViews()[0].UtilityAIs[0]
	if got.FailedPlans.Len() != 1 || got.FailedPlans.Entries[0].Goal != model.CraftTool || got.CurrentGoal != model.Fight {
		t.Fatalf("active craft failure = %#v, want one craft failure and Fight selection", got)
	}
	if got.LastExitReason != model.GoalExitFailed {
		t.Fatalf("exit reason = %v, want failed", got.LastExitReason)
	}
}

func assertLogAttr(t testing.TB, sink *recordSink, index int, key string, want any) {
	t.Helper()
	value, found := sink.attr(index, key)
	if !found {
		t.Fatalf("log attribute %q missing", key)
	}
	switch expected := want.(type) {
	case string:
		if value.String() != expected {
			t.Fatalf("log attribute %q = %q, want %q", key, value.String(), expected)
		}
	case bool:
		if value.Bool() != expected {
			t.Fatalf("log attribute %q = %t, want %t", key, value.Bool(), expected)
		}
	default:
		t.Fatalf("unsupported expected log type %T", want)
	}
}

func runGoalSelector(t testing.TB, system *GoalSelectorSystem, world *enginecore.World, outbox *network.Outbox, tick uint64) {
	t.Helper()
	if err := system.Run(&scheduler.RunContext{Context: context.Background(), World: world, Tick: tick, Data: &TickData{Outbox: outbox}, Logger: slog.New(&recordSink{})}); err != nil {
		t.Fatalf("GoalSelectorSystem.Run() error = %v", err)
	}
}
