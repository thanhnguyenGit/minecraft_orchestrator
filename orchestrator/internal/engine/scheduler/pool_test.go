package scheduler

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewWorkerPool_NegativeWorkers(t *testing.T) {
	_, err := NewWorkerPool(-1, 0)
	if err == nil {
		t.Fatal("expected error for negative workers")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewWorkerPool_ZeroWorkers(t *testing.T) {
	pool, err := NewWorkerPool(0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()
}

func TestNewWorkerPool_CapacityClamped(t *testing.T) {
	pool, err := NewWorkerPool(4, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()

	if cap(pool.jobs) < 4 {
		t.Fatalf("capacity = %d, want >= 4", cap(pool.jobs))
	}
}

func TestNewWorkerPool_CapacityRespected(t *testing.T) {
	pool, err := NewWorkerPool(4, 16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()

	if cap(pool.jobs) != 16 {
		t.Fatalf("capacity = %d, want 16", cap(pool.jobs))
	}
}

func TestParallelFor_ZeroTotal(t *testing.T) {
	pool, err := NewWorkerPool(2, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()

	err = pool.ParallelFor(context.Background(), 0, 1, func(start, end int) {
		t.Fatal("fn should not be called when total is 0")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParallelFor_ExecutesWork(t *testing.T) {
	pool, err := NewWorkerPool(4, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()

	var counter atomic.Int64
	err = pool.ParallelFor(context.Background(), 100, 10, func(start, end int) {
		for i := start; i < end; i++ {
			counter.Add(1)
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count := counter.Load(); count != 100 {
		t.Fatalf("counter = %d, want 100", count)
	}
}

func TestParallelFor_Coverage(t *testing.T) {
	pool, err := NewWorkerPool(4, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()

	seen := make([]bool, 10)
	err = pool.ParallelFor(context.Background(), 10, 3, func(start, end int) {
		for i := start; i < end; i++ {
			seen[i] = true
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("index %d was not processed", i)
		}
	}
}

func TestParallelFor_DefaultGrain(t *testing.T) {
	pool, err := NewWorkerPool(4, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()

	var counter atomic.Int64
	err = pool.ParallelFor(context.Background(), 100, 0, func(start, end int) {
		counter.Add(int64(end - start))
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count := counter.Load(); count != 100 {
		t.Fatalf("counter = %d, want 100", count)
	}
}

func TestParallelFor_WorkerPanic(t *testing.T) {
	pool, err := NewWorkerPool(1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()

	err = pool.ParallelFor(context.Background(), 5, 1, func(start, end int) {
		panic("intentional panic")
	})
	if err == nil {
		t.Fatal("expected error from worker panic")
	}
	if !strings.Contains(err.Error(), "worker panic") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	pool, err := NewWorkerPool(2, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.Close()
	pool.Close()
}

func TestClose_BlocksNewWork(t *testing.T) {
	pool, err := NewWorkerPool(1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pool.Close()

	err = pool.ParallelFor(context.Background(), 10, 1, func(start, end int) {})
	if err == nil {
		t.Fatal("expected error after pool closed")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClose_WorkersExit(t *testing.T) {
	pool, err := NewWorkerPool(2, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.Close()

	done := make(chan struct{})
	go func() {
		pool.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("workers did not exit within 1 second")
	}
}

func TestWorkerPool_ContextCancelled(t *testing.T) {
	pool, err := NewWorkerPool(1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = pool.ParallelFor(ctx, 1000000, 1, func(start, end int) {})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
