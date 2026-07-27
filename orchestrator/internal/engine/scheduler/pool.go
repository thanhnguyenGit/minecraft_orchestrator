package scheduler

import (
	"context"
	"sync"
)

type jobGroup struct {
	wg sync.WaitGroup
	once sync.Once
	err error
}

type poolJob struct {
	group *jobGroup
	fn func()
}

type WorkerPool struct {
	jobs chan poolJob
	stop chan struct{}
	wg sync.WaitGroup
	once sync.Once
}

func (p *WorkerPool) ParallelFor(ctx context.Context, total, grain int, fn func(start, end int)) error {
	return nil
}