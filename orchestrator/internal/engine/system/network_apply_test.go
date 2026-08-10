package core

import (
	"context"
	"io"
	"log/slog"
	"reflect"
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

func TestNetworkApplySystemResetsUtilityStateForNewRemoteSession(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x05}
	stageMirroredBot(t, world, profileID)
	view := world.MirroredBotViews()[0]
	view.Sessions[0] = model.Session{Phase: model.SessionPlayReady, RemoteSessionID: "old-session", LastSequence: 8}
	view.UtilityAIs[0] = model.UtilityAIState{
		CurrentGoal:    model.Fight,
		Phase:          model.GoalPhaseExecuting,
		LastExitReason: model.GoalExitFailed,
	}
	view.ControllerSyncs[0] = model.ControllerSyncState{ControllerSequence: 8}

	err := (NetworkApplySystem{}).Run(&scheduler.RunContext{Context: context.Background(), World: world, Data: &TickData{Network: network.Batch{Events: []network.Event{
		{ProfileID: profileID, Kind: network.EventHostSnapshot, RemoteSessionID: "new-session", Sequence: 1, Snapshot: &network.HostSnapshot{}},
	}}}, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	view = world.MirroredBotViews()[0]
	if got := view.UtilityAIs[0]; !reflect.DeepEqual(got, model.UtilityAIState{}) {
		t.Fatalf("UtilityAIState after new session = %#v, want zero value", got)
	}
	if got := view.ControllerSyncs[0]; got != (model.ControllerSyncState{}) {
		t.Fatalf("ControllerSyncState after new session = %#v, want zero value", got)
	}
}

func TestNetworkApplySystemClearsOldRealityFeedbackForNewRemoteSession(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x06}
	stageMirroredBot(t, world, profileID)
	view := world.MirroredBotViews()[0]
	view.Sessions[0] = model.Session{Phase: model.SessionPlayReady, RemoteSessionID: "old-session", LastSequence: 8}
	target := model.BlockPosition{X: 3, Y: 64, Z: 1}
	world.Resources().RealityView().Set(profileID, model.RealityState{
		GotoTarget:               &target,
		ArrivalDistance:          float64Ptr(1),
		DiggingBlock:             &target,
		ActionFailed:             true,
		Failure:                  "old failure",
		ActionFailureCorrelation: 1,
	})

	err := (NetworkApplySystem{}).Run(&scheduler.RunContext{
		Context: context.Background(),
		World:   world,
		Data: &TickData{Network: network.Batch{Events: []network.Event{
			{ProfileID: profileID, Kind: network.EventHostSnapshot, RemoteSessionID: "new-session", Sequence: 1, Snapshot: &network.HostSnapshot{}},
		}}},
		Logger: discardLogger(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, ok := world.Resources().RealityView().Get(profileID); ok {
		t.Fatalf("reality after new session = %#v, want cleared", got)
	}

	view = world.MirroredBotViews()[0]
	view.UtilityAIs[0] = model.UtilityAIState{
		CurrentGoal: model.Idle,
		Phase:       model.GoalPhaseExecuting,
	}
	view.ControllerSyncs[0] = model.ControllerSyncState{
		Desired:  model.ControllerState{GotoTarget: &target},
		LastSent: model.ControllerState{GotoTarget: &target},
	}
	outbox := network.NewOutbox()
	runGoalSelector(t, &GoalSelectorSystem{}, world, outbox, 2)
	if got := world.MirroredBotViews()[0].UtilityAIs[0]; got.Phase == model.GoalPhaseBlocked {
		t.Fatalf("old-session reality affected first new-session action: %#v", got)
	}
}

func TestNetworkApplySystemStoresActionFailureRealityFeedback(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x08}
	stageMirroredBot(t, world, profileID)
	world.MirroredBotViews()[0].Sessions[0] = model.Session{Phase: model.SessionPlayReady, RemoteSessionID: "session-a"}

	err := (NetworkApplySystem{}).Run(&scheduler.RunContext{
		Context: context.Background(),
		World:   world,
		Data: &TickData{Network: network.Batch{Events: []network.Event{{
			ProfileID:       profileID,
			Kind:            network.EventRealityState,
			RemoteSessionID: "session-a",
			RealityState: &network.RealityState{
				ActionFailed:             true,
				Failure:                  "cannot craft",
				ActionFailureCorrelation: 42,
			},
		}}}},
		Logger: discardLogger(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, ok := world.Resources().RealityView().Get(profileID)
	if !ok || !got.ActionFailed || got.Failure != "cannot craft" || got.ActionFailureCorrelation != 42 {
		t.Fatalf("stored reality = %#v, want mapped action failure", got)
	}
}

func TestNetworkApplySystemRejectsCrossSessionRealitySequenceCollision(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x09}
	stageMirroredBot(t, world, profileID)
	view := world.MirroredBotViews()[0]
	view.Sessions[0] = model.Session{Phase: model.SessionPlayReady, RemoteSessionID: "new-session"}
	world.Resources().RealityView().Set(profileID, model.RealityState{ActionFailed: true, Failure: "current-session"})
	err := (NetworkApplySystem{}).Run(&scheduler.RunContext{Context: context.Background(), World: world, Data: &TickData{Network: network.Batch{Events: []network.Event{{
		ProfileID: profileID, Kind: network.EventRealityState, RemoteSessionID: "old-session", Sequence: 7,
		RealityState: &network.RealityState{ActionOutcomes: []model.ActionOutcome{{ControllerSequence: 7, Action: model.ControllerActionCraft, Status: model.ActionOutcomeFailed}}},
	}}}}, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, ok := world.Resources().RealityView().Get(profileID); !ok || got.Failure != "current-session" {
		t.Fatalf("stale reality cleared current-session state: %#v", got)
	}
	craft := model.CraftTarget{ItemName: "oak_planks", Count: 4}
	view.Inventorys[0] = model.Inventory{Slots: []model.InventorySlot{{Slot: 0, Item: &model.ItemStack{Name: "oak_log", Count: 1}}}}
	view.UtilityAIs[0] = model.UtilityAIState{CurrentGoal: model.CraftTool, Phase: model.GoalPhaseExecuting, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "oak_planks"}}
	view.ControllerSyncs[0] = model.ControllerSyncState{Desired: model.ControllerState{CraftTarget: &craft}, LastSent: model.ControllerState{CraftTarget: &craft}, InFlightOneShot: model.InFlightOneShot{Action: model.ControllerActionCraft, Correlation: 7, Goal: model.CraftTool, Target: model.GoalTarget{Kind: model.GoalTargetItem, Item: "oak_planks"}}}
	runGoalSelector(t, &GoalSelectorSystem{}, world, network.NewOutbox(), 1)
	if got := world.MirroredBotViews()[0].ControllerSyncs[0].InFlightOneShot; got.Correlation != 7 {
		t.Fatalf("old-session outcome altered new in-flight action: %#v", got)
	}

	err = (NetworkApplySystem{}).Run(&scheduler.RunContext{Context: context.Background(), World: world, Data: &TickData{Network: network.Batch{Events: []network.Event{{
		ProfileID: profileID, Kind: network.EventRealityState, RemoteSessionID: "new-session", Sequence: 7,
		RealityState: &network.RealityState{ActionOutcomes: []model.ActionOutcome{{ControllerSequence: 7, Action: model.ControllerActionCraft, Status: model.ActionOutcomeCompleted}}},
	}}}}, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, ok := world.Resources().RealityView().Get(profileID); !ok || len(got.ActionOutcomes) != 1 || got.ActionOutcomes[0].Status != model.ActionOutcomeCompleted {
		t.Fatalf("new-session reality = %#v, want matching outcome", got)
	}
	runGoalSelector(t, &GoalSelectorSystem{}, world, network.NewOutbox(), 2)
	if got := world.MirroredBotViews()[0].ControllerSyncs[0].InFlightOneShot; got.Action != model.ControllerActionNone {
		t.Fatalf("matching new-session outcome did not apply: %#v", got)
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
	want := []string{"ecs.status", "ecs.status_drop", "ecs.apply", "ecs.apply_drop", "ecs.state"}
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
	view := world.MirroredBotViews()[0]
	if got := view.UtilityAIs[0]; !reflect.DeepEqual(got, model.UtilityAIState{}) {
		t.Fatalf("bootstrap utility state = %#v, want zero value", got)
	}
	if got := view.ControllerSyncs[0]; got != (model.ControllerSyncState{}) {
		t.Fatalf("bootstrap controller sync = %#v, want zero value", got)
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
	bundle.Set(model.CHunger, model.Hunger{Current: 20, Max: 20})
	bundle.Set(model.CGameMode, model.GameModeSurvival)
	bundle.Set(model.CInventory, model.Inventory{})
	bundle.Set(model.CEffects, model.Effects{})
	bundle.Set(model.CUtilityAI, model.UtilityAIState{})
	bundle.Set(model.CControllerSync, model.ControllerSyncState{})
	commands := enginecore.NewCommandBuffer(0)
	commands.Stage(enginecore.CreateCommand{Bundle: bundle})
	world.Stage(commands.Envelopes(), []model.Mask{model.MirroredBotMask})
	if err := world.Sync(); err != nil {
		t.Fatalf("Sync(): %v", err)
	}
}

func float64Ptr(value float64) *float64 { return &value }
