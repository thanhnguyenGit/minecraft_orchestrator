package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/engine/scheduler"
	enginesystem "minecraft_orchestrator/internal/engine/system"
)

// Runtime is the only owner of the ECS World. The Mineflayer host receives
// only Inbox/Outbox handles, never a World pointer.
type Runtime struct {
	World *enginecore.World

	executor *scheduler.Executor
	pool     *scheduler.WorkerPool
	inbox    *network.Inbox
	outbox   *network.Outbox

	config HostConfig
	bots   []BotSpec
	clock  Clock

	mu   sync.Mutex
	host *HostSupervisor
}

func NewRuntime(
	config HostConfig,
	bots []BotSpec,
	clock Clock,
) (*Runtime, error) {
	if len(bots) == 0 {
		return nil, errors.New("runtime requires at least one bot")
	}
	
	plan, err := enginesystem.BuildScheduler()
	if err != nil {
		return nil, fmt.Errorf("build engine scheduler: %w", err)
	}
	
	pool, err := scheduler.NewWorkerPool(0, 16)
	if err != nil {
		return nil, fmt.Errorf("create engine worker pool: %w", err)
	}
	
	executor, err := scheduler.NewExecutor(plan, pool)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create engine executor: %w", err)
	}

	return &Runtime{
		World:    enginecore.NewWorld(),
		executor: executor,
		pool:     pool,
		inbox:    network.NewInbox(),
		outbox:   network.NewOutbox(),
		config:   config,
		bots:     append([]BotSpec(nil), bots...),
		clock:    clock,
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("runtime context is required")
	}
	host, err := NewHostSupervisor(ctx, r.config, r.inbox, r.bots)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.host = host
	r.mu.Unlock()
	defer func() {
		host.Close()
		r.mu.Lock()
		r.host = nil
		r.mu.Unlock()
	}()

	bootstrap := make([]model.Bot, len(r.bots))
	for index, bot := range r.bots {
		bootstrap[index] = model.Bot{ProfileID: bot.ProfileID, Username: bot.Username}
	}

	loop := FixedLoop{
		Clock: r.clock,
	}
	return loop.Run(ctx, func(tick uint64, delta time.Duration) error {
		data := &enginesystem.TickData{
			Bootstrap: bootstrap,
			Network:   r.inbox.Drain(),
			Outbox:    r.outbox,
		}
		bootstrap = nil
		if err := r.executor.RunFrame(ctx, r.World, tick, delta, data); err != nil {
			return err
		}
		return host.Apply(ctx, r.outbox.Drain())
	})
}

func (r *Runtime) Close() {
	r.mu.Lock()
	host := r.host
	r.mu.Unlock()
	if host != nil {
		host.Close()
	}
	if r.pool != nil {
		r.pool.Close()
	}
}
