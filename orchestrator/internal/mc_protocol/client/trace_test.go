package client

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

func TestSessionTracesInboundPacketAndAutomaticReplies(t *testing.T) {
	recorder := newTraceHandler()
	var output bytes.Buffer
	session := &Session{
		cfg:                 Config{Logger: slog.New(recorder)},
		phase:               PhasePlay,
		codec:               wire.NewCodec(),
		writer:              &output,
		playerLoadedPending: true,
	}

	err := session.handleInbound(wire.Packet{ID: 0x46, Body: []byte{1, 2, 3}}, SynchronizePlayerPosition{TeleportID: 7, X: 1, Y: 2, Z: 3})
	if err != nil {
		t.Fatalf("handleInbound() error = %v", err)
	}

	records := recorder.records()
	if len(records) != 3 {
		t.Fatalf("record count = %d, want 3", len(records))
	}
	wantTraceRecord(t, records[0], map[string]any{
		"direction": "inbound", "sequence": uint64(1), "phase": "play", "packet_id": "0x46", "body_bytes": int64(3),
		"message_type": "client.SynchronizePlayerPosition", "teleport_id": int64(7),
	})
	wantTraceRecord(t, records[1], map[string]any{
		"direction": "outbound", "sequence": uint64(2), "caused_by": uint64(1), "phase": "play", "packet_id": "0x0", "body_bytes": int64(1),
		"message_type": "client.TeleportConfirm", "teleport_id": int64(7),
	})
	wantTraceRecord(t, records[2], map[string]any{
		"direction": "outbound", "sequence": uint64(3), "caused_by": uint64(1), "phase": "play", "packet_id": "0x2b", "body_bytes": int64(0),
		"message_type": "client.PlayerLoaded",
	})
	if _, ok := traceAttrs(records[0])["raw_body"]; ok {
		t.Fatal("inbound trace includes raw_body")
	}
}

func TestTraceSessionIDDoesNotExposeUsername(t *testing.T) {
	if got := nextTraceSessionID("private_bot_name"); strings.Contains(got, "private_bot_name") {
		t.Fatalf("session ID %q exposes the username", got)
	}
}

func TestSessionSkipsPacketTraceWorkWhenDebugIsDisabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelInfo}))
	session := &Session{cfg: Config{Logger: logger}, phase: PhasePlay}

	session.tracePacket("inbound", wire.Packet{ID: 0x46}, SynchronizePlayerPosition{}, 0)
	if got := session.traceSequence.Load(); got != 0 {
		t.Fatalf("trace sequence = %d, want no debug trace work", got)
	}
}

type traceHandler struct {
	mu       sync.Mutex
	recorded []slog.Record
}

func newTraceHandler() *traceHandler { return new(traceHandler) }

func (h *traceHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *traceHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	h.recorded = append(h.recorded, record.Clone())
	h.mu.Unlock()
	return nil
}
func (h *traceHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *traceHandler) WithGroup(string) slog.Handler      { return h }

func (h *traceHandler) records() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.recorded...)
}

func wantTraceRecord(t *testing.T, record slog.Record, want map[string]any) {
	t.Helper()
	if record.Message != "minecraft.packet" || record.Level != slog.LevelDebug {
		t.Fatalf("record = %#v, want debug minecraft.packet", record)
	}
	attrs := traceAttrs(record)
	for key, value := range want {
		if attrs[key] != value {
			t.Fatalf("attribute %q = %#v, want %#v; attrs = %#v", key, attrs[key], value, attrs)
		}
	}
}

func traceAttrs(record slog.Record) map[string]any {
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	return attrs
}
