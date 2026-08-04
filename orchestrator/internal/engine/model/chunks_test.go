package model

import "testing"

func TestWorldViewsTracksLoadedAirAndBlockChanges(t *testing.T) {
	var views WorldViews
	profileID := ProfileID{0x01}
	const attemptID uint64 = 7
	activateChunkView(t, &views, profileID, attemptID)

	section, err := NewSingleValueSection(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	column, err := NewChunkColumn(-64, 16, []ChunkSection{section})
	if err != nil {
		t.Fatal(err)
	}
	chunk := ChunkPosition{X: 2, Z: -1}
	if !views.ReplaceChunk(profileID, attemptID, chunk, column) {
		t.Fatal("ReplaceChunk() = false, want current attempt accepted")
	}

	position := BlockPosition{X: 32, Y: -64, Z: -16}
	if stateID, loaded := views.BlockState(profileID, position); !loaded || stateID != 0 {
		t.Fatalf("BlockState() = (%d, %t), want loaded air", stateID, loaded)
	}
	if !views.SetBlockState(profileID, attemptID, position, 42) {
		t.Fatal("SetBlockState() = false, want loaded current chunk accepted")
	}
	if stateID, loaded := views.BlockState(profileID, position); !loaded || stateID != 42 {
		t.Fatalf("BlockState() = (%d, %t), want (42, true)", stateID, loaded)
	}
	if version, loaded := views.ChunkVersion(profileID, chunk); !loaded || version != 2 {
		t.Fatalf("ChunkVersion() = (%d, %t), want (2, true)", version, loaded)
	}

	if !views.UnloadChunk(profileID, attemptID, chunk) {
		t.Fatal("UnloadChunk() = false, want loaded chunk removed")
	}
	if _, loaded := views.BlockState(profileID, position); loaded {
		t.Fatal("BlockState() reports unloaded chunk as loaded")
	}
}

func TestNewSingleValueSectionRejectsInconsistentNonAirCount(t *testing.T) {
	if _, err := NewSingleValueSection(0, 42); err == nil {
		t.Fatal("NewSingleValueSection() error = nil, want non-air count validation")
	}
	if _, err := NewSingleValueSection(sectionBlockCount, 0); err == nil {
		t.Fatal("NewSingleValueSection() error = nil, want air count validation")
	}
}

func TestPaletteSectionsRejectInconsistentNonAirCount(t *testing.T) {
	indirectWords := make([]uint64, packedWordCount(sectionBlockCount, 4))
	if _, err := NewIndirectPaletteSection(sectionBlockCount, 4, []uint32{0}, indirectWords); err == nil {
		t.Fatal("NewIndirectPaletteSection() error = nil, want non-air count validation")
	}
	directWords := make([]uint64, packedWordCount(sectionBlockCount, 9))
	directWords[0] = 1
	if _, err := NewDirectPaletteSection(0, 9, directWords); err == nil {
		t.Fatal("NewDirectPaletteSection() error = nil, want non-air count validation")
	}
}

func TestWorldViewsRejectsStaleAndUnloadedBlockChanges(t *testing.T) {
	var views WorldViews
	profileID := ProfileID{0x02}
	activateChunkView(t, &views, profileID, 7)
	position := BlockPosition{X: 0, Y: -64, Z: 0}
	if views.SetBlockState(profileID, 6, position, 1) {
		t.Fatal("stale SetBlockState() = true")
	}
	if views.SetBlockState(profileID, 7, position, 1) {
		t.Fatal("unloaded SetBlockState() = true")
	}
}

func TestWorldViewsClearsChunksWhenDimensionGeometryChanges(t *testing.T) {
	var views WorldViews
	profileID := ProfileID{0x03}
	const attemptID uint64 = 7
	activateChunkView(t, &views, profileID, attemptID)
	section, err := NewSingleValueSection(sectionBlockCount, 9)
	if err != nil {
		t.Fatal(err)
	}
	column, err := NewChunkColumn(-64, 16, []ChunkSection{section})
	if err != nil {
		t.Fatal(err)
	}
	chunk := ChunkPosition{}
	if !views.ReplaceChunk(profileID, attemptID, chunk, column) {
		t.Fatal("ReplaceChunk() = false")
	}

	if !views.SetDimensionTypes(profileID, attemptID, []DimensionType{{RegistryID: 0, Key: "minecraft:overworld", MinY: 0, Height: 128}}) {
		t.Fatal("SetDimensionTypes() = false")
	}
	if _, loaded := views.ChunkVersion(profileID, chunk); loaded {
		t.Fatal("geometry replacement retained old chunk")
	}
	view, ok := views.Get(profileID)
	if !ok || view.DimensionEpoch != 1 {
		t.Fatalf("view = %#v, want epoch increment", view)
	}
}

func activateChunkView(t *testing.T, views *WorldViews, profileID ProfileID, attemptID uint64) {
	t.Helper()
	views.BeginAttempt(profileID, attemptID)
	types := []DimensionType{{RegistryID: 0, Key: "minecraft:overworld", MinY: -64, Height: 16}}
	if !views.SetDimensionTypes(profileID, attemptID, types) {
		t.Fatal("SetDimensionTypes() = false")
	}
	if !views.BindDimension(profileID, attemptID, 0) {
		t.Fatal("BindDimension() = false")
	}
}
