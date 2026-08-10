package core

import (
	"context"
	"log/slog"
	"testing"

	enginecore "minecraft_orchestrator/internal/engine/core"
	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/scheduler"
)

type allVisibleFOV struct{}

func (allVisibleFOV) IsInFOV(model.Position, model.Rotation, float32, model.Position) bool {
	return true
}

func TestPerceptionBlockCountersDistinguishVisibleAndOccludedResources(t *testing.T) {
	const attemptID uint64 = 1
	profileID := model.ProfileID{0x52}
	var views model.WorldViews
	views.BeginAttempt(profileID, attemptID)
	if !views.SetDimensionTypes(profileID, attemptID, []model.DimensionType{{RegistryID: 0, Key: "minecraft:overworld", MinY: 48, Height: 32}}) ||
		!views.BindDimension(profileID, attemptID, 0) {
		t.Fatal("could not initialize loaded world view")
	}

	air, err := model.NewSingleValueSection(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []model.ChunkPosition{{X: -1, Z: -1}, {X: -1, Z: 0}, {X: 0, Z: -1}, {X: 0, Z: 0}} {
		column, err := model.NewChunkColumn(48, 32, []model.ChunkSection{air, air})
		if err != nil {
			t.Fatal(err)
		}
		if !views.ReplaceChunk(profileID, attemptID, chunk, column) {
			t.Fatalf("could not load chunk %#v", chunk)
		}
	}

	set := func(position model.BlockPosition, stateID uint32) {
		t.Helper()
		if !views.SetBlockState(profileID, attemptID, position, stateID) {
			t.Fatalf("could not set block %#v", position)
		}
	}
	set(model.BlockPosition{X: 0, Y: 64, Z: -1}, 130) // visible oak_log
	set(model.BlockPosition{X: 0, Y: 64, Z: -5}, 130) // exposed but occluded oak_log
	set(model.BlockPosition{X: 1, Y: 64, Z: -5}, 10)
	set(model.BlockPosition{X: -1, Y: 64, Z: -5}, 10)
	set(model.BlockPosition{X: 0, Y: 65, Z: -5}, 10)
	set(model.BlockPosition{X: 0, Y: 63, Z: -5}, 10)
	set(model.BlockPosition{X: 0, Y: 64, Z: -6}, 10)
	set(model.BlockPosition{X: 0, Y: 64, Z: -3}, 10)

	blocks, counters := NewPerceptionSystem(allVisibleFOV{}).scanBlocksInFOV(
		profileID,
		model.Position{X: 0, Y: 64, Z: 0},
		model.Rotation{},
		&views,
		model.Inventory{},
	)

	if len(blocks) != 1 || blocks[0].Name != "oak_log" {
		t.Fatalf("visible blocks = %#v, want one visible oak log", blocks)
	}
	if counters.resourceCandidates != 2 || counters.exposedResourceCandidates != 2 ||
		counters.occludedResourceCandidates != 1 || counters.nonMineableResourceCandidates != 0 ||
		counters.visibleMineableResources != 1 {
		t.Fatalf("resource counters = %#v, want visible and occluded resources distinguished", counters)
	}
}

func TestPerceptionSystemLogsEntityAndBlockFOVDiagnostics(t *testing.T) {
	world := enginecore.NewWorld()
	profileID := model.ProfileID{0x53}
	stageMirroredBot(t, world, profileID)
	view := world.MirroredBotViews()[0]
	view.Sessions[0] = model.Session{Phase: model.SessionPlayReady, RemoteSessionID: "session-a"}
	view.Positions[0] = model.Position{X: 0, Y: 64, Z: 0}
	world.Resources().EntityViews().AddEntities(profileID, []model.Entity{{
		ID: 7, Name: "zombie", Position: model.Position{X: 0, Y: 64, Z: -1},
	}})
	loadDiagnosticPerceptionWorld(t, world.Resources().WorldViews(), profileID)

	sink := &recordSink{}
	err := NewPerceptionSystem(allVisibleFOV{}).Run(&scheduler.RunContext{
		Context: context.Background(), World: world, Logger: slog.New(sink),
	})
	if err != nil {
		t.Fatalf("PerceptionSystem.Run() error = %v", err)
	}

	entityRecord := perceptionRecord(t, sink, "perception.entities_fov")
	blockRecord := perceptionRecord(t, sink, "perception.blocks_fov")
	if entityRecord.Level != slog.LevelInfo || blockRecord.Level != slog.LevelInfo {
		t.Fatalf("diagnostic levels = %s, %s; want Info", entityRecord.Level, blockRecord.Level)
	}
	assertRecordHasAttrs(t, entityRecord,
		"profile_id", "username", "nearby_entities", "visible_entities", "fov_rejected_entities",
		"visible_hostiles", "nearest_hostile_distance", "threat",
	)
	assertRecordHasAttrs(t, blockRecord,
		"profile_id", "username", "resource_candidates", "exposed_resource_candidates",
		"buried_resource_candidates", "occluded_resource_candidates", "non_mineable_resource_candidates",
		"visible_mineable_resources", "nearest_resource_candidate", "nearest_resource_candidate_distance",
	)
	assertRecordInt(t, entityRecord, "visible_hostiles", 1)
	assertRecordInt(t, blockRecord, "resource_candidates", 1)
	assertRecordInt(t, blockRecord, "visible_mineable_resources", 1)
}

func loadDiagnosticPerceptionWorld(t testing.TB, views *model.WorldViews, profileID model.ProfileID) {
	t.Helper()
	const attemptID uint64 = 1
	views.BeginAttempt(profileID, attemptID)
	if !views.SetDimensionTypes(profileID, attemptID, []model.DimensionType{{RegistryID: 0, Key: "minecraft:overworld", MinY: 48, Height: 32}}) ||
		!views.BindDimension(profileID, attemptID, 0) {
		t.Fatal("could not initialize loaded world view")
	}
	air, err := model.NewSingleValueSection(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []model.ChunkPosition{{X: -1, Z: -1}, {X: -1, Z: 0}, {X: 0, Z: -1}, {X: 0, Z: 0}} {
		column, err := model.NewChunkColumn(48, 32, []model.ChunkSection{air, air})
		if err != nil {
			t.Fatal(err)
		}
		if !views.ReplaceChunk(profileID, attemptID, chunk, column) {
			t.Fatalf("could not load chunk %#v", chunk)
		}
	}
	if !views.SetBlockState(profileID, attemptID, model.BlockPosition{X: 0, Y: 64, Z: -1}, 130) {
		t.Fatal("could not set visible oak log")
	}
}

func perceptionRecord(t testing.TB, sink *recordSink, message string) slog.Record {
	t.Helper()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for _, record := range sink.records {
		if record.Message == message {
			return record.Clone()
		}
	}
	t.Fatalf("log message %q not found in %#v", message, sink.records)
	return slog.Record{}
}

func assertRecordHasAttrs(t testing.TB, record slog.Record, keys ...string) {
	t.Helper()
	attrs := map[string]slog.Value{}
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value
		return true
	})
	for _, key := range keys {
		if _, ok := attrs[key]; !ok {
			t.Fatalf("%s missing required attribute %q", record.Message, key)
		}
	}
}

func assertRecordInt(t testing.TB, record slog.Record, key string, want int64) {
	t.Helper()
	var got slog.Value
	found := false
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			got, found = attr.Value, true
			return false
		}
		return true
	})
	if !found || got.Kind() != slog.KindInt64 || got.Int64() != want {
		t.Fatalf("%s %q = %v, want int %d", record.Message, key, got, want)
	}
}
