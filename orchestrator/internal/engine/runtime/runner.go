package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/mc_protocol/client"
)

const bootstrapStagger = 250 * time.Millisecond

type workerHandle struct {
	id     uint64
	cancel context.CancelFunc
}

// SessionRunner owns worker contexts and the initial connection stagger. It
// never reads or writes ECS state; it receives lifecycle intent through Outbox
// and reports session observations through Inbox.
type SessionRunner struct {
	ctx    context.Context
	cancel context.CancelFunc

	config  client.Config
	inbox   *network.Inbox
	factory SessionFactory
	sleep   SleepFunc

	mu          sync.Mutex
	bots        map[model.ProfileID]BotSpec
	workers     map[model.ProfileID]workerHandle
	nextWorker  uint64
	startNumber uint64
	wg          sync.WaitGroup

	runWorker func(context.Context, BotSpec) error
}

func NewSessionRunner(ctx context.Context, config client.Config, inbox *network.Inbox, factory SessionFactory, bots []BotSpec) (*SessionRunner, error) {
	if ctx == nil {
		return nil, errors.New("session runner context is required")
	}
	if inbox == nil {
		return nil, errors.New("session runner inbox is required")
	}
	if factory == nil {
		return nil, errors.New("session runner factory is required")
	}
	runner := newSessionRunner(ctx, bots)
	runner.config = config
	runner.inbox = inbox
	runner.factory = factory
	runner.runWorker = func(workerCtx context.Context, bot BotSpec) error {
		return Worker{Bot: bot, Config: runner.config, Inbox: runner.inbox, Factory: runner.factory, Sleep: runner.sleep}.Run(workerCtx)
	}
	return runner, nil
}

func newSessionRunner(ctx context.Context, bots []BotSpec) *SessionRunner {
	runnerCtx, cancel := context.WithCancel(ctx)
	runner := &SessionRunner{
		ctx:     runnerCtx,
		cancel:  cancel,
		sleep:   sleep,
		bots:    make(map[model.ProfileID]BotSpec, len(bots)),
		workers: make(map[model.ProfileID]workerHandle, len(bots)),
	}
	for _, bot := range bots {
		runner.bots[bot.ProfileID] = bot
	}
	return runner
}

func (r *SessionRunner) Apply(intents []network.Intent) error {
	for _, intent := range intents {
		switch intent.Kind {
		case network.IntentStartSession:
			if err := r.start(intent.ProfileID); err != nil {
				return err
			}
		case network.IntentStopSession:
			r.stop(intent.ProfileID)
		default:
			return fmt.Errorf("unknown session intent kind: %d", intent.Kind)
		}
	}
	return nil
}

func (r *SessionRunner) start(profileID model.ProfileID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.workers[profileID]; exists {
		return nil
	}
	bot, found := r.bots[profileID]
	if !found {
		return fmt.Errorf("start session for unknown bot profile %x", profileID)
	}

	r.nextWorker++
	handle := workerHandle{id: r.nextWorker}
	workerCtx, cancel := context.WithCancel(r.ctx)
	handle.cancel = cancel
	r.workers[profileID] = handle
	delay := time.Duration(r.startNumber) * bootstrapStagger
	r.startNumber++
	r.wg.Add(1)

	go r.run(profileID, handle.id, workerCtx, bot, delay)
	return nil
}

func (r *SessionRunner) run(profileID model.ProfileID, workerID uint64, ctx context.Context, bot BotSpec, delay time.Duration) {
	defer r.wg.Done()
	defer r.forget(profileID, workerID)

	if delay > 0 && r.sleep(ctx, delay) != nil {
		return
	}
	if r.runWorker != nil {
		_ = r.runWorker(ctx, bot)
	}
}

func (r *SessionRunner) stop(profileID model.ProfileID) {
	r.mu.Lock()
	handle, exists := r.workers[profileID]
	if exists {
		delete(r.workers, profileID)
	}
	r.mu.Unlock()
	if exists {
		handle.cancel()
	}
}

func (r *SessionRunner) forget(profileID model.ProfileID, workerID uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if handle, found := r.workers[profileID]; found && handle.id == workerID {
		delete(r.workers, profileID)
	}
}

func (r *SessionRunner) Close() {
	r.cancel()
	r.mu.Lock()
	workers := make([]workerHandle, 0, len(r.workers))
	for _, worker := range r.workers {
		workers = append(workers, worker)
	}
	clear(r.workers)
	r.mu.Unlock()
	for _, worker := range workers {
		worker.cancel()
	}
}

func (r *SessionRunner) Wait() {
	r.wg.Wait()
}
