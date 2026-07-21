package observability

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAsyncHandlerDropsNewestRecordWhenQueueIsFull(t *testing.T) {
	next := newBlockingHandler()
	handler := NewAsyncHandler(next, 1)
	logger := slog.New(handler)

	logger.Info("first")
	select {
	case <-next.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not begin handling first record")
	}
	logger.Info("second")
	logger.Info("third")

	close(next.release)
	if err := handler.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	got := next.messages()
	if !contains(got, "first") || !contains(got, "second") {
		t.Fatalf("messages = %v, want first and second", got)
	}
	if contains(got, "third") {
		t.Fatalf("messages = %v, newest record was not dropped", got)
	}
	if !contains(got, "minecraft.log_dropped") {
		t.Fatalf("messages = %v, want dropped-record warning", got)
	}
	if got[len(got)-1] != "minecraft.log_dropped" {
		t.Fatalf("messages = %v, want dropped-record warning after queued records", got)
	}
}

func TestAsyncHandlerDoesNotMisattributeDroppedWarning(t *testing.T) {
	writer := newBlockingWriter()
	base := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := NewAsyncHandler(base, 1)
	logger := slog.New(handler).With("component", "minecraft_protocol")

	logger.Info("first")
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not begin writing first record")
	}
	logger.Info("second")
	logger.Info("third")
	close(writer.release)
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	got := writer.String()
	if !strings.Contains(got, "msg=minecraft.log_dropped") {
		t.Fatalf("log output = %q, want dropped warning", got)
	}
	if strings.Contains(got, "msg=minecraft.log_dropped component=minecraft_protocol") {
		t.Fatalf("log output = %q, dropped warning was falsely attributed", got)
	}
}

type blockingWriter struct {
	bytes.Buffer
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingWriter() *blockingWriter {
	return &blockingWriter{started: make(chan struct{}), release: make(chan struct{})}
}

func (w *blockingWriter) Write(data []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	return w.Buffer.Write(data)
}

var _ io.Writer = (*blockingWriter)(nil)

func TestAsyncHandlerRespectsWrappedWarningLevel(t *testing.T) {
	next := newBlockingHandler()
	next.allowWarnings = false
	handler := NewAsyncHandler(next, 1)
	first := slog.NewRecord(time.Now(), slog.LevelInfo, "first", 0)
	second := slog.NewRecord(time.Now(), slog.LevelInfo, "second", 0)
	third := slog.NewRecord(time.Now(), slog.LevelInfo, "third", 0)
	if err := handler.Handle(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	select {
	case <-next.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not begin handling first record")
	}
	_ = handler.Handle(context.Background(), second)
	_ = handler.Handle(context.Background(), third)
	close(next.release)
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	if contains(next.messages(), "minecraft.log_dropped") {
		t.Fatalf("messages = %v, warning bypassed wrapped handler level", next.messages())
	}
}

type blockingHandler struct {
	mu            sync.Mutex
	records       []slog.Record
	started       chan struct{}
	release       chan struct{}
	blockOne      sync.Once
	allowWarnings bool
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{started: make(chan struct{}), release: make(chan struct{}), allowWarnings: true}
}

func (h *blockingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.allowWarnings || level != slog.LevelWarn
}

func (h *blockingHandler) Handle(_ context.Context, record slog.Record) error {
	h.blockOne.Do(func() {
		close(h.started)
		<-h.release
	})
	h.mu.Lock()
	h.records = append(h.records, record.Clone())
	h.mu.Unlock()
	return nil
}

func (h *blockingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *blockingHandler) WithGroup(string) slog.Handler      { return h }

func (h *blockingHandler) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	messages := make([]string, len(h.records))
	for i, record := range h.records {
		messages[i] = record.Message
	}
	return messages
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
