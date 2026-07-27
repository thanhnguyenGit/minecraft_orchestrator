package scheduler

import (
	"cmp"
	"context"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
)

type Executor struct {
	plan *ExecutionPlan
	pool *WorkerPool
}

func NewExecutor(plan *ExecutionPlan, pool *WorkerPool) (*Executor, error) {
	if plan == nil {
		return nil, fmt.Errorf("execution plan is nil")
	}
	if pool == nil {
		return nil, fmt.Errorf("worker pool is nil")
	}

	return &Executor{
		plan: plan,
		pool: pool,
	}, nil
}

func (e *Executor) RunFrame(
	ctx context.Context,
	world *core.World,
	tick uint64,
	delta time.Duration,
	data any,
) error {
	for nodeIndex, node := range e.plan.Nodes {
		switch node.Kind {
		case NodeSync:
			if err := world.Sync(); err != nil {
				return fmt.Errorf("plan node %d sync (%s): %w", nodeIndex, node.Reason, err)
			}
		case NodeWave:
			for _, compiled := range node.Systems {
				for _, query := range compiled.System.Access().Queries {
					if dirty, conflict := world.QueryTouchesDirty(query); conflict {
						return fmt.Errorf("plan/runtime mismatch: system %q queries %s while archetype %s is dirty", compiled.System.ID(), query, dirty)
					}
				}
			}

			if err := e.runWave(ctx, world, node, tick, delta, data); err != nil {
				return fmt.Errorf("plan node %d phase %q wave %d: %w", nodeIndex, node.Phase, node.Wave, err)
			}

		default:
			return fmt.Errorf("plan node %d has unknown kind %d", nodeIndex, node.Kind)
		}
	}

	if world.PendingCommands() != 0 {
		return fmt.Errorf("execution plan completed with %d pending commands", world.PendingCommands())
	}

	return nil
}

type systemResult struct {
	compiled CompiledSystem
	buffer   *core.CommandBuffer
	err      error
}

func (e *Executor) runWave(ctx context.Context, world *core.World, node PlanNode, tick uint64, delta time.Duration, data any) error {
	results := make([]systemResult, len(node.Systems))
	var wait sync.WaitGroup
	wait.Add(len(node.Systems))

	for index, compiled := range node.Systems {
		_index, _compiled := index, compiled
		go func() {
			defer wait.Done()
			buffer := core.NewCommandBuffer(_compiled.Order)
			results[_index] = systemResult{
				compiled: _compiled,
				buffer:   buffer,
			}

			defer func() {
				if recovered := recover(); recovered != nil {
					results[_index].err = fmt.Errorf("system %q panic: %v\n%s", _compiled.System.ID(), recovered, debug.Stack())
				}
			}()

			results[_index].err = _compiled.System.Run(&RunContext{
				Context:  ctx,
				World:    world,
				Commands: buffer,
				Pool:     e.pool,
				Tick:     tick,
				Delta:    delta,
				Data:     data,
			})
		}()
	}

	wait.Wait()

	for _, result := range results {
		if result.err != nil {
			return result.err
		}

		if result.buffer.Len() > 0 && !result.compiled.System.Access().HashStructuralEffects() {
			return fmt.Errorf("system %q emitted structural commands without declaring structural effects", result.compiled.System.ID())
		}

		if err := validateCommandEffects(result.compiled.System.Access().Structural, result.buffer.Envelopes()); err != nil {
			return fmt.Errorf("system %q: %w", result.compiled.System.ID(), err)
		}
	}

	all := make([]core.Envelop, 0)
	for _, result := range results {
		all = append(all, result.buffer.Envelopes()...)
	}

	slices.SortStableFunc(all, func(a, b core.Envelop) int {
		if a.SystemOrder != b.SystemOrder {
			return cmp.Compare(a.SystemOrder, b.SystemOrder)
		}

		return cmp.Compare(a.Sequence, b.Sequence)
	})

	world.Stage(all, node.DirtyAfter)

	return nil
}

func validateCommandEffects(declared []model.Mask, envelops []core.Envelop) error {
	for _, envelop := range envelops {
		actual := envelop.Command.DeclaredAffected()

		for _, affected := range actual {
			if !slices.Contains(declared, affected) {
				return fmt.Errorf("command %s affects undeclared archetype %s", envelop.Command, affected)
			}
		}
	}

	return nil
}
