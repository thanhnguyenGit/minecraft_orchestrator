package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
)

type (
	PhaseID  string
	SystemID string
)

type AccessSpec struct {
	// Components signatures selected by the system
	Queries []model.Mask

	// Components columns for READ/WRITE
	Reads  model.Mask
	Writes model.Mask

	// List exact archetypes that deferred commands may dirty.
	Structural []model.Mask
}

func (a AccessSpec) HashStructuralEffects() bool {
	return len(a.Structural) > 0
}

func (a AccessSpec) Validate() error {
	if a.Reads.Intersects(a.Writes) {
		return fmt.Errorf("reads and writes overlap at %s", a.Reads&a.Writes)
	}

	if slices.Contains(a.Queries, 0) {
		return fmt.Errorf("query mask cannot be empty")
	}

	return nil
}

type System interface {
	ID() SystemID
	Access() AccessSpec
	Run(*RunContext) error
}

type RunContext struct {
	Context  context.Context
	World    *core.World
	Commands *core.CommandBuffer
	Pool     *WorkerPool
	Tick     uint64
	Delta    time.Duration
	Data     any
	Logger   *slog.Logger
}

func (c *RunContext) DeltaSeconds() float64 {
	return c.Delta.Seconds()
}

func (c *RunContext) ParallelFor(total, grain int, fn func(start, end int)) error {
	return c.Pool.ParallelFor(c.Context, total, grain, fn)
}
