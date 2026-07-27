package scheduler

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
)

type jobGroup struct {
	wg   sync.WaitGroup
	once sync.Once
	err  error
}

func (g *jobGroup) fail(err error) {
	g.once.Do(func() {
		g.err = err
	})
}

type poolJob struct {
	group *jobGroup
	fn    func()
}

type WorkerPool struct {
	jobs chan poolJob
	stop chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

func NewWorkerPool(workers, queueCapacity int) (*WorkerPool, error) {
	if workers < 0 {
		return nil, fmt.Errorf("workers count must be positive")
	}

	if queueCapacity <= 0 {
		queueCapacity = 16
	}

	if queueCapacity < workers {
		queueCapacity = workers
	}

	pool := &WorkerPool{
		jobs: make(chan poolJob, queueCapacity),
		stop: make(chan struct{}),
	}

	pool.wg.Add(workers)

	for range workers {
		go pool.runWorker()
	}

	return pool, nil
}

func (p *WorkerPool) runWorker() {
	defer p.wg.Done()
	for {
		select {
		case job := <-p.jobs:
			func() {
				defer job.group.wg.Done()
				defer func() {
					if recovered := recover(); recovered != nil {
						job.group.fail(fmt.Errorf("worker panic: %v\n%s", recovered, debug.Stack()))
					}
				}()

				job.fn()
			}()
		case <-p.stop:
			return
		}
	}
}

func (p *WorkerPool) ParallelFor(ctx context.Context, total, grain int, fn func(start, end int)) error {
	if total <= 0 {
		return nil
	}

	if grain <= 0 {
		grain = 256
	}

	group := &jobGroup{}
	chunks := (total + grain - 1) / grain
	group.wg.Add(chunks)

	for start := 0; start < total; start += grain {
		end := start + grain
		end = min(end, total)

		_start, _end := start, end
		select {
		case p.jobs <- poolJob{
			group: group,
			fn: func() {
				fn(_start, _end)
			},
		}:

		case <-ctx.Done():
			remaining := (total - start + grain - 1) / grain

			for range remaining {
				group.wg.Done()
			}

			group.wg.Wait()
			return ctx.Err()

		case <-p.stop:
			remaining := (total - start + grain - 1) / grain
			for range remaining {
				group.wg.Done()
			}

			group.wg.Wait()
			return fmt.Errorf("worker pool is closed")
		}
	}

	group.wg.Wait()

	return group.err
}

func (p *WorkerPool) Close() {
	p.once.Do(func() {
		close(p.stop)
		p.wg.Wait()
	})
}
