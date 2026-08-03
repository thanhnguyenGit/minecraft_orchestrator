package core

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/engine/scheduler"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type recordSink struct {
	mu      sync.Mutex
	records []slog.Record
}

func (s *recordSink) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelDebug
}

func (s *recordSink) Handle(_ context.Context, record slog.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record.Clone())
	return nil
}

func (s *recordSink) WithAttrs([]slog.Attr) slog.Handler { return s }
func (s *recordSink) WithGroup(string) slog.Handler      { return s }

func (s *recordSink) messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := make([]string, len(s.records))
	for index, record := range s.records {
		messages[index] = record.Message
	}
	return messages
}

func (s *recordSink) attr(index int, key string) (slog.Value, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.records) {
		return slog.Value{}, false
	}
	var value slog.Value
	found := false
	s.records[index].Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			value = attr.Value
			found = true
			return false
		}
		return true
	})
	return value, found
}

func TestNetworkApplySystemRejectsStaleHostObservations(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x04}
	stageMirroredBot(t, world, profileID)
	snapshot := func(x float64) *network.HostSnapshot {
		return &network.HostSnapshot{Vitals: network.HostVitals{Health: 12}, Position: model.Position{X: x}, Inventory: model.Inventory{SelectedHotbarSlot: 1}}
	}
	err := (NetworkApplySystem{}).Run(&scheduler.RunContext{Context: context.Background(), World: world, Data: &TickData{Network: network.Batch{Events: []network.Event{
		{ProfileID: profileID, Kind: network.EventHostSnapshot, RemoteSessionID: "host-a", Sequence: 2, Snapshot: snapshot(2)},
		{ProfileID: profileID, Kind: network.EventHostSnapshot, RemoteSessionID: "host-a", Sequence: 1, Snapshot: snapshot(1)},
		{ProfileID: profileID, Kind: network.EventHostSnapshot, RemoteSessionID: "host-b", Sequence: 1, Snapshot: snapshot(3)},
	}}}, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	view := world.MirroredBotViews()[0]
	if view.Positions[0].X != 3 || view.Sessions[0].RemoteSessionID != "host-b" || view.Sessions[0].LastSequence != 1 {
		t.Fatalf("host state = position=%+v session=%+v", view.Positions[0], view.Sessions[0])
	}
}

func TestNetworkApplySystemLogsApplyOutcomes(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x07}
	stageMirroredBot(t, world, profileID)
	sink := &recordSink{}

	err := (NetworkApplySystem{}).Run(&scheduler.RunContext{Context: context.Background(), World: world, Data: &TickData{Network: network.Batch{Events: []network.Event{
		{ProfileID: profileID, Kind: network.EventHostStatus, HostStatus: network.HostConnecting, RemoteSessionID: "host-a", Sequence: 1},
		{ProfileID: profileID, Kind: network.EventHostStatus, HostStatus: network.HostConnecting, RemoteSessionID: "host-a", Sequence: 1},
		{ProfileID: profileID, Kind: network.EventHostSnapshot, RemoteSessionID: "host-a", Sequence: 2, Snapshot: &network.HostSnapshot{Position: model.Position{X: 1}}},
		{ProfileID: profileID, Kind: network.EventHostVitals, RemoteSessionID: "host-a", Sequence: 1, Vitals: &network.HostVitals{Health: 12}},
	}}}, Logger: slog.New(sink)})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	messages := sink.messages()
	want := []string{"ecs.status", "ecs.status_drop", "ecs.apply", "ecs.apply_drop"}
	if len(messages) != len(want) {
		t.Fatalf("messages = %v, want %v", messages, want)
	}
	for index, message := range want {
		if messages[index] != message {
			t.Fatalf("messages = %v, want %v", messages, want)
		}
	}

	if value, found := sink.attr(0, "phase_from"); !found || value.String() != "stopped" {
		t.Fatalf("ecs.status phase_from = %v (found=%v), want stopped", value, found)
	}
	if value, found := sink.attr(0, "phase_to"); !found || value.String() != "connecting" {
		t.Fatalf("ecs.status phase_to = %v (found=%v), want connecting", value, found)
	}
	if value, found := sink.attr(2, "kind"); !found || value.String() != "host_snapshot" {
		t.Fatalf("ecs.apply kind = %v (found=%v), want host_snapshot", value, found)
	}
	if value, found := sink.attr(3, "kind"); !found || value.String() != "host_vitals" {
		t.Fatalf("ecs.apply_drop kind = %v (found=%v), want host_vitals", value, found)
	}
	if value, found := sink.attr(3, "reason"); !found || value.String() != "stale_sequence" {
		t.Fatalf("ecs.apply_drop reason = %v (found=%v), want stale_sequence", value, found)
	}
	if value, found := sink.attr(3, "last_seq"); !found || value.Uint64() != 2 {
		t.Fatalf("ecs.apply_drop last_seq = %v (found=%v), want 2", value, found)
	}
}

func TestBootstrapSystemCreatesMirroredBotAndEmitsStartIntent(t *testing.T) {
	world := enginecore.NewWorld()
	outbox := network.NewOutbox()
	profileID := model.ProfileID{0x03}
	commands := enginecore.NewCommandBuffer(0)
	if err := (BootstrapSystem{}).Run(&scheduler.RunContext{Context: context.Background(), World: world, Commands: commands, Data: &TickData{Bootstrap: []model.Bot{{ProfileID: profileID, Username: "bot"}}, Outbox: outbox}, Logger: discardLogger()}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	world.Stage(commands.Envelopes(), []model.Mask{model.MirroredBotMask})
	if err := world.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if intents := outbox.Drain(); len(intents) != 1 || intents[0].Kind != network.IntentStartHost {
		t.Fatalf("intents = %#v", intents)
	}
}

func stageMirroredBot(t testing.TB, world *enginecore.World, profileID model.ProfileID) {
	t.Helper()
	var bundle enginecore.Bundle
	bundle.Set(model.CBot, model.Bot{ProfileID: profileID, Username: "bot"})
	bundle.Set(model.CSession, model.Session{})
	bundle.Set(model.CPosition, model.Position{})
	bundle.Set(model.CRotation, model.Rotation{})
	bundle.Set(model.CVelocity, model.Velocity{})
	bundle.Set(model.CHealth, model.Health{Current: 20, Max: 20})
	bundle.Set(model.CGameMode, model.GameModeSurvival)
	bundle.Set(model.CInventory, model.Inventory{})
	bundle.Set(model.CEffects, model.Effects{})
	commands := enginecore.NewCommandBuffer(0)
	commands.Stage(enginecore.CreateCommand{Bundle: bundle})
	world.Stage(commands.Envelopes(), []model.Mask{model.MirroredBotMask})
	if err := world.Sync(); err != nil {
		t.Fatalf("Sync(): %v", err)
	}
}
