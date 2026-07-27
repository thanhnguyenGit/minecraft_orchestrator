package scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
)

type errorSystem struct {
	id  SystemID
	err error
}

func (s errorSystem) ID() SystemID      { return s.id }
func (s errorSystem) Access() AccessSpec { return testAccess(0, 0, nil, nil) }
func (s errorSystem) Run(_ *RunContext) error {
	return s.err
}

type panicSystem struct {
	id    SystemID
	value any
}

func (s panicSystem) ID() SystemID      { return s.id }
func (s panicSystem) Access() AccessSpec { return testAccess(0, 0, nil, nil) }
func (s panicSystem) Run(_ *RunContext) error {
	panic(s.value)
}

type stagingSystem struct {
	id       SystemID
	access   AccessSpec
	stageCmd core.Command
}

func (s stagingSystem) ID() SystemID       { return s.id }
func (s stagingSystem) Access() AccessSpec  { return s.access }
func (s stagingSystem) Run(ctx *RunContext) error {
	ctx.Commands.Stage(s.stageCmd)
	return nil
}

func mustCreatePool(t testing.TB, workers, cap int) *WorkerPool {
	t.Helper()
	pool, err := NewWorkerPool(workers, cap)
	if err != nil {
		t.Fatalf("NewWorkerPool error: %v", err)
	}
	return pool
}

func mustCreateExecutor(t testing.TB, plan *ExecutionPlan, pool *WorkerPool) *Executor {
	t.Helper()
	executor, err := NewExecutor(plan, pool)
	if err != nil {
		t.Fatalf("NewExecutor error: %v", err)
	}
	return executor
}

func TestNewExecutor_NilPlan(t *testing.T) {
	pool := mustCreatePool(t, 1, 1)
	defer pool.Close()

	_, err := NewExecutor(nil, pool)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}
}

func TestNewExecutor_NilPool(t *testing.T) {
	plan := &ExecutionPlan{}

	_, err := NewExecutor(plan, nil)
	if err == nil {
		t.Fatal("expected error for nil pool")
	}
}

func TestNewExecutor_Valid(t *testing.T) {
	plan := &ExecutionPlan{}
	pool := mustCreatePool(t, 1, 1)
	defer pool.Close()

	executor, err := NewExecutor(plan, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if executor == nil {
		t.Fatal("executor is nil")
	}
}

func TestRunFrame_EmptyPlan(t *testing.T) {
	plan := &ExecutionPlan{}
	pool := mustCreatePool(t, 1, 1)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFrame_PendingCommandsAfterPlan(t *testing.T) {
	plan := &ExecutionPlan{}
	pool := mustCreatePool(t, 1, 1)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	world.Stage([]core.Envelop{
		{SystemOrder: 1, Sequence: 0, Command: core.CreateCommand{Bundle: core.Bundle{Mask: model.Components(model.CPosition)}}},
	}, nil)

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err == nil {
		t.Fatal("expected error for pending commands")
	}
	if !strings.Contains(err.Error(), "pending commands") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFrame_SyncNode(t *testing.T) {
	plan := &ExecutionPlan{
		Nodes: []PlanNode{
			{
				Kind:   NodeSync,
				Reason: "test sync",
			},
		},
	}
	pool := mustCreatePool(t, 1, 1)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFrame_SyncNode_Error(t *testing.T) {
	plan := &ExecutionPlan{
		Nodes: []PlanNode{
			{
				Kind:   NodeSync,
				Reason: "test sync",
			},
			{
				Kind:   NodeSync,
				Reason: "test sync 2",
			},
		},
	}
	pool := mustCreatePool(t, 1, 1)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	world.Stage([]core.Envelop{
		{SystemOrder: 0, Sequence: 0, Command: core.CreateCommand{Bundle: core.Bundle{}}},
	}, nil)

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err == nil {
		t.Fatal("expected sync validation error")
	}
	if !strings.Contains(err.Error(), "plan node 0 sync") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFrame_WaveNode_RunsSystem(t *testing.T) {
	id := SystemID("test-system")
	plan := &ExecutionPlan{
		Nodes: []PlanNode{
			{
				Kind:    NodeWave,
				Phase:   "main",
				Wave:    0,
				Systems: []CompiledSystem{{System: testSystem{id: id, access: testAccess(0, 0, nil, nil)}, Order: 0}},
			},
		},
	}
	pool := mustCreatePool(t, 4, 8)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFrame_WaveNode_SystemError(t *testing.T) {
	id := SystemID("error-system")
	plan := &ExecutionPlan{
		Nodes: []PlanNode{
			{
				Kind:    NodeWave,
				Phase:   "main",
				Wave:    0,
				Systems: []CompiledSystem{{System: errorSystem{id: id, err: context.DeadlineExceeded}, Order: 0}},
			},
		},
	}
	pool := mustCreatePool(t, 4, 8)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err == nil {
		t.Fatal("expected error from system")
	}
	if !strings.Contains(err.Error(), "wave 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFrame_WaveNode_SystemPanic(t *testing.T) {
	id := SystemID("panic-system")
	plan := &ExecutionPlan{
		Nodes: []PlanNode{
			{
				Kind:    NodeWave,
				Phase:   "main",
				Wave:    0,
				Systems: []CompiledSystem{{System: panicSystem{id: id, value: "boom"}, Order: 0}},
			},
		},
	}
	pool := mustCreatePool(t, 4, 8)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err == nil {
		t.Fatal("expected panic error")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should contain panic value: %v", err)
	}
}

func TestRunFrame_WaveNode_DirtyQuery(t *testing.T) {
	queryMask := model.Components(model.CPosition)
	dirtyMask := model.Components(model.CPosition, model.CVelocity)

	id := SystemID("query-system")
	plan := &ExecutionPlan{
		Nodes: []PlanNode{
			{
				Kind:  NodeWave,
				Phase: "main",
				Wave:  0,
				Systems: []CompiledSystem{{
					System: testSystem{
						id:     id,
						access: testAccess(0, 0, []model.Mask{queryMask}, nil),
					},
					Order: 0,
				}},
			},
		},
	}
	pool := mustCreatePool(t, 1, 1)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	world.Stage(nil, []model.Mask{dirtyMask})

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err == nil {
		t.Fatal("expected dirty query conflict error")
	}
	if !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFrame_UnknownKind(t *testing.T) {
	plan := &ExecutionPlan{
		Nodes: []PlanNode{
			{
				Kind: NodeKind(99),
			},
		},
	}
	pool := mustCreatePool(t, 1, 1)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFrame_StructuralCommandWithoutDeclaration(t *testing.T) {
	id := SystemID("staging-system")
	plan := &ExecutionPlan{
		Nodes: []PlanNode{
			{
				Kind:    NodeWave,
				Phase:   "main",
				Wave:    0,
				Systems: []CompiledSystem{{
					System: stagingSystem{
						id:       id,
						access:   testAccess(0, 0, nil, nil),
						stageCmd: core.CreateCommand{Bundle: core.Bundle{Mask: model.Components(model.CPosition)}},
					},
					Order: 0,
				}},
			},
		},
	}
	pool := mustCreatePool(t, 4, 8)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err == nil {
		t.Fatal("expected error for structural commands without declaration")
	}
	if !strings.Contains(err.Error(), "structural commands without declaring structural effects") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFrame_CommandEffectsUndeclaredMask(t *testing.T) {
	declaredMask := model.Components(model.CPosition)
	actualMask := model.Components(model.CVelocity)

	id := SystemID("staging-system")
	plan := &ExecutionPlan{
		Nodes: []PlanNode{
			{
				Kind:  NodeWave,
				Phase: "main",
				Wave:  0,
				Systems: []CompiledSystem{{
					System: stagingSystem{
						id: id,
						access: testAccess(0, 0, nil, []model.Mask{declaredMask}),
						stageCmd: core.CreateCommand{Bundle: core.Bundle{Mask: actualMask}},
					},
					Order: 0,
				}},
			},
		},
	}
	pool := mustCreatePool(t, 4, 8)
	defer pool.Close()
	executor := mustCreateExecutor(t, plan, pool)
	world := core.NewWorld()

	err := executor.RunFrame(context.Background(), world, 0, time.Second, nil)
	if err == nil {
		t.Fatal("expected error for undeclared command effects")
	}
	if !strings.Contains(err.Error(), "affects undeclared archetype") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommandEffects_Empty(t *testing.T) {
	err := validateCommandEffects(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommandEffects_AllDeclared(t *testing.T) {
	mask := model.Components(model.CPosition)
	declared := []model.Mask{mask}
	envelopes := []core.Envelop{
		{
			SystemOrder: 0,
			Sequence:    0,
			Command:     core.CreateCommand{Bundle: core.Bundle{Mask: mask}},
		},
	}

	err := validateCommandEffects(declared, envelopes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommandEffects_UndeclaredMask(t *testing.T) {
	declaredMask := model.Components(model.CPosition)
	actualMask := model.Components(model.CVelocity)
	declared := []model.Mask{declaredMask}
	envelopes := []core.Envelop{
		{
			SystemOrder: 0,
			Sequence:    0,
			Command:     core.CreateCommand{Bundle: core.Bundle{Mask: actualMask}},
		},
	}

	err := validateCommandEffects(declared, envelopes)
	if err == nil {
		t.Fatal("expected error for undeclared mask")
	}
	if !strings.Contains(err.Error(), "affects undeclared archetype") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommandEffects_MultipleDeclared(t *testing.T) {
	maskA := model.Components(model.CPosition)
	maskB := model.Components(model.CBot)
	declared := []model.Mask{maskA, maskB}
	envelopes := []core.Envelop{
		{
			SystemOrder: 0,
			Sequence:    0,
			Command:     core.CreateCommand{Bundle: core.Bundle{Mask: maskA}},
		},
		{
			SystemOrder: 0,
			Sequence:    1,
			Command:     core.CreateCommand{Bundle: core.Bundle{Mask: maskB}},
		},
	}

	err := validateCommandEffects(declared, envelopes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommandEffects_PartialMatchFails(t *testing.T) {
	maskA := model.Components(model.CPosition)
	maskB := model.Components(model.CBot)
	maskC := model.Components(model.CVelocity)
	declared := []model.Mask{maskA, maskB}
	envelopes := []core.Envelop{
		{
			SystemOrder: 0,
			Sequence:    0,
			Command:     core.CreateCommand{Bundle: core.Bundle{Mask: maskA}},
		},
		{
			SystemOrder: 0,
			Sequence:    1,
			Command:     core.CreateCommand{Bundle: core.Bundle{Mask: maskC}},
		},
	}

	err := validateCommandEffects(declared, envelopes)
	if err == nil {
		t.Fatal("expected error for undeclared mask in mixed batch")
	}
}
