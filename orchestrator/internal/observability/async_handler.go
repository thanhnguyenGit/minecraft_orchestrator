// Package observability contains non-blocking logging infrastructure.
package observability

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// AsyncHandler passes records to a dedicated worker so producers never wait on
// a potentially slow log writer. Records submitted to a full queue are dropped.
type AsyncHandler struct {
	state *asyncState
	next  slog.Handler
}

type asyncState struct {
	base    slog.Handler
	records chan asyncRecord
	done    chan struct{}

	mu     sync.Mutex
	closed bool

	closeOnce sync.Once
	dropped   atomic.Uint64

	errMu sync.Mutex
	err   error
}

type asyncRecord struct {
	ctx     context.Context
	record  slog.Record
	handler slog.Handler
}

// NewAsyncHandler wraps next with a queue of capacity records. A capacity below
// one is treated as one so logging remains asynchronous.
func NewAsyncHandler(next slog.Handler, capacity int) *AsyncHandler {
	if next == nil {
		panic("async log handler requires a wrapped handler")
	}
	if capacity < 1 {
		capacity = 1
	}
	state := &asyncState{
		base:    next,
		records: make(chan asyncRecord, capacity),
		done:    make(chan struct{}),
	}
	
	go state.run()
	
	return &AsyncHandler{
		state: state, 
		next: next,
	}
}

func (h *AsyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *AsyncHandler) Handle(ctx context.Context, record slog.Record) error {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	if h.state.closed {
		return nil
	}

	pending := asyncRecord{ctx: ctx, record: record.Clone(), handler: h.next}
	select {
	case h.state.records <- pending:
	default:
		h.state.dropped.Add(1)
	}
	return nil
}

func (h *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &AsyncHandler{state: h.state, next: h.next.WithAttrs(attrs)}
}

func (h *AsyncHandler) WithGroup(name string) slog.Handler {
	return &AsyncHandler{state: h.state, next: h.next.WithGroup(name)}
}

// Close stops accepting new records, drains records already accepted, and
// returns the first error reported by the wrapped handler.
func (h *AsyncHandler) Close() error {
	h.state.closeOnce.Do(func() {
		h.state.mu.Lock()
		h.state.closed = true
		close(h.state.records)
		h.state.mu.Unlock()
	})
	<-h.state.done

	h.state.errMu.Lock()
	defer h.state.errMu.Unlock()
	return h.state.err
}

func (s *asyncState) run() {
	defer close(s.done)
	for pending := range s.records {
		s.handle(pending.ctx, pending.handler, pending.record)
		s.logDroppedIfIdle()
	}
	s.logDroppedIfIdle()
}

func (s *asyncState) logDroppedIfIdle() {
	s.mu.Lock()
	if len(s.records) != 0 {
		s.mu.Unlock()
		return
	}
	dropped := s.dropped.Swap(0)
	s.mu.Unlock()
	if dropped == 0 {
		return
	}
	if !s.base.Enabled(context.Background(), slog.LevelWarn) {
		return
	}
	record := slog.NewRecord(time.Now(), slog.LevelWarn, "minecraft.log_dropped", 0)
	record.AddAttrs(slog.Uint64("count", dropped))
	s.handle(context.Background(), s.base, record)
}

func (s *asyncState) handle(ctx context.Context, handler slog.Handler, record slog.Record) {
	if err := handler.Handle(ctx, record); err != nil {
		s.errMu.Lock()
		if s.err == nil {
			s.err = err
		}
		s.errMu.Unlock()
	}
}

var _ slog.Handler = (*AsyncHandler)(nil)
