package core

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func mustCreateEntity(t testing.TB, w *World, b Bundle) Entity {
	t.Helper()
	e, err := w.createNow(b)
	if err != nil {
		t.Fatalf("createNow error = %v", err)
	}
	return e
}

func profileIDForTest(id uint64) model.ProfileID {
	var profileID model.ProfileID
	for offset := 0; id > 0; offset++ {
		profileID[len(profileID)-1-offset] = byte(id)
		id >>= 8
	}
	return profileID
}

func makeBundle(mask model.Mask, botID uint64) Bundle {
	var b Bundle
	for c := range model.ComponentCount {
		if !mask.Has(c) {
			continue
		}
		switch c {
		case model.CPosition:
			b.Set(c, model.Position{X: float64(botID*10) + 1, Y: 2, Z: 3})
		case model.CVelocity:
			b.Set(c, model.Velocity{X: 0.1, Y: 0.2, Z: 0.3})
		case model.CHealth:
			b.Set(c, model.Health{Current: 20, Max: 20})
		case model.CBot:
			b.Set(c, model.Bot{ProfileID: profileIDForTest(botID), Username: "test-bot"})
		}
	}
	return b
}

func TestNewWorld(t *testing.T) {
	w := NewWorld()
	if w == nil {
		t.Fatal("NewWorld returned nil")
	}
	if len(w.tables) != 0 {
		t.Fatalf("tables len = %d, want 0", len(w.tables))
	}
	if len(w.locations) != 1 {
		t.Fatalf("locations len = %d, want 1 (index 0 reserved)", len(w.locations))
	}
	if len(w.generations) != 1 {
		t.Fatalf("generations len = %d, want 1 (index 0 reserved)", len(w.generations))
	}
	if len(w.botIndex) != 0 {
		t.Fatalf("botIndex len = %d, want 0", len(w.botIndex))
	}
}

func TestWorld_EnsureTable_CreatesNew(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition, model.CBot)

	tbl := w.ensureTable(mask)
	if tbl == nil {
		t.Fatal("ensureTable returned nil")
	}
	if tbl.mask != mask {
		t.Fatalf("table mask = %v, want %v", tbl.mask, mask)
	}
	if len(w.tables) != 1 {
		t.Fatalf("tables len = %d, want 1", len(w.tables))
	}
}

func TestWorld_EnsureTable_ReturnsExisting(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition)

	t1 := w.ensureTable(mask)
	t2 := w.ensureTable(mask)

	if t1 != t2 {
		t.Fatal("ensureTable should return same table instance")
	}
	if len(w.tables) != 1 {
		t.Fatalf("tables len = %d, want 1", len(w.tables))
	}
}

func TestWorld_EnsureTable_KeepsOrderSorted(t *testing.T) {
	w := NewWorld()
	m2 := model.Components(model.CPosition, model.CBot)
	m1 := model.Components(model.CPosition)

	w.ensureTable(m2)
	w.ensureTable(m1)

	for i := 1; i < len(w.tableOrder); i++ {
		if w.tableOrder[i-1] > w.tableOrder[i] {
			t.Fatalf("tableOrder not sorted: %v", w.tableOrder)
		}
	}
}

func TestWorld_Matching(t *testing.T) {
	w := NewWorld()
	maskExact := model.Components(model.CPosition, model.CBot)
	maskSubset := model.Components(model.CPosition)
	maskOther := model.Components(model.CHealth)

	w.ensureTable(maskExact)
	w.ensureTable(maskOther)

	result := w.matching(maskSubset)
	if len(result) != 1 {
		t.Fatalf("matching returned %d tables, want 1", len(result))
	}
	if result[0].mask != maskExact {
		t.Fatalf("matched table mask = %v, want %v", result[0].mask, maskExact)
	}
}

func TestWorld_Matching_NoMatch(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CHealth)
	w.ensureTable(mask)

	result := w.matching(model.Components(model.CPosition))
	if len(result) != 0 {
		t.Fatalf("matching returned %d tables, want 0", len(result))
	}
}

func TestWorld_Matching_EmptyWorld(t *testing.T) {
	w := NewWorld()
	result := w.matching(model.Components(model.CPosition))
	if len(result) != 0 {
		t.Fatalf("matching returned %d tables, want 0", len(result))
	}
}

func TestWorld_AllocateEntity_Fresh(t *testing.T) {
	w := NewWorld()

	e := w.allocateEntity()
	if e.Index != 1 {
		t.Fatalf("Index = %d, want 1 (first allocatable index)", e.Index)
	}
	if e.Generation != 1 {
		t.Fatalf("Generation = %d, want 1", e.Generation)
	}
}

func TestWorld_AllocateEntity_ReuseIndex(t *testing.T) {
	w := NewWorld()
	e1 := w.allocateEntity()

	w.free = append(w.free, e1.Index)
	w.generations[e1.Index]++

	e2 := w.allocateEntity()
	if e2.Index != e1.Index {
		t.Fatalf("Index = %d, want reused index %d", e2.Index, e1.Index)
	}
	if e2.Generation != 2 {
		t.Fatalf("Generation = %d, want 2 (incremented)", e2.Generation)
	}
}

func TestWorld_Resolve_ZeroEntity(t *testing.T) {
	w := NewWorld()
	_, err := w.resolve(Entity{})
	if err == nil {
		t.Fatal("expected error for zero entity")
	}
}

func TestWorld_Resolve_OutOfBounds(t *testing.T) {
	w := NewWorld()
	_, err := w.resolve(Entity{Index: 999, Generation: 1})
	if err == nil {
		t.Fatal("expected error for out-of-bounds index")
	}
}

func TestWorld_Resolve_DeadEntity(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition, model.CBot)
	e := mustCreateEntity(t, w, makeBundle(mask, 1))

	w.destroyNow(e)

	_, err := w.resolve(e)
	if err == nil {
		t.Fatal("expected error for dead entity")
	}
}

func TestWorld_Resolve_WrongGeneration(t *testing.T) {
	w := NewWorld()
	var b Bundle
	b.Set(model.CPosition, model.Position{X: 1})
	e := mustCreateEntity(t, w, b)

	_, err := w.resolve(Entity{Index: e.Index, Generation: e.Generation + 1})
	if err == nil {
		t.Fatal("expected error for wrong generation")
	}
}

func TestWorld_Resolve_Success(t *testing.T) {
	w := NewWorld()
	var b Bundle
	b.Set(model.CPosition, model.Position{X: 1})
	e := mustCreateEntity(t, w, b)

	loc, err := w.resolve(e)
	if err != nil {
		t.Fatalf("resolve error = %v", err)
	}
	if !loc.alive {
		t.Fatal("location should be alive")
	}
	if loc.generation != e.Generation {
		t.Fatalf("generation = %d, want %d", loc.generation, e.Generation)
	}
}

func TestWorld_Bundle_InvalidEntity(t *testing.T) {
	w := NewWorld()
	_, err := w.bundle(Entity{})
	if err == nil {
		t.Fatal("expected error for invalid entity")
	}
}

func TestWorld_Bundle_Success(t *testing.T) {
	w := NewWorld()
	var b Bundle
	b.Set(model.CPosition, model.Position{X: 42})
	e := mustCreateEntity(t, w, b)

	bundle, err := w.bundle(e)
	if err != nil {
		t.Fatalf("bundle error = %v", err)
	}
	pos := bundle.Get(model.CPosition).(model.Position)
	if pos.X != 42 {
		t.Fatalf("Position.X = %f, want 42", pos.X)
	}
}

func TestWorld_Create_ValidateFailure(t *testing.T) {
	w := NewWorld()
	_, err := w.createNow(Bundle{})
	if err == nil {
		t.Fatal("expected error for empty bundle")
	}
}

func TestWorld_Create_DuplicateBot(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition, model.CBot)

	mustCreateEntity(t, w, makeBundle(mask, 100))

	_, err := w.createNow(makeBundle(mask, 100))
	if err == nil {
		t.Fatal("expected error for duplicate bot ID")
	}
}

func TestWorld_Create_Success(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition, model.CBot)
	e := mustCreateEntity(t, w, makeBundle(mask, 100))

	loc, err := w.resolve(e)
	if err != nil {
		t.Fatalf("resolve after create error = %v", err)
	}
	if !loc.alive {
		t.Fatal("entity should be alive after create")
	}
	if _, found := w.botIndex[profileIDForTest(100)]; !found {
		t.Fatal("bot profile ID should be in botIndex")
	}
}

func TestWorld_Create_Success_NoBot(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition)

	e := mustCreateEntity(t, w, makeBundle(mask, 0))

	loc, err := w.resolve(e)
	if err != nil {
		t.Fatalf("resolve after create error = %v", err)
	}
	if !loc.alive {
		t.Fatal("entity should be alive after create")
	}
	if len(w.botIndex) != 0 {
		t.Fatal("botIndex should be empty for non-bot entity")
	}
}

func TestWorld_Destroy_InvalidEntity(t *testing.T) {
	w := NewWorld()
	err := w.destroyNow(Entity{})
	if err == nil {
		t.Fatal("expected error for invalid entity")
	}
}

func TestWorld_Destroy_DeadEntity(t *testing.T) {
	w := NewWorld()
	var b Bundle
	b.Set(model.CPosition, model.Position{X: 1})
	e := mustCreateEntity(t, w, b)

	w.destroyNow(e)

	err := w.destroyNow(e)
	if err == nil {
		t.Fatal("expected error for already dead entity")
	}
}

func TestWorld_Destroy_NoBot(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition)
	e := mustCreateEntity(t, w, makeBundle(mask, 0))

	if err := w.destroyNow(e); err != nil {
		t.Fatalf("destroyNow error = %v, non-bot entities should be destroyable", err)
	}

	_, err := w.resolve(e)
	if err == nil {
		t.Fatal("entity should be dead after destroy")
	}
}

func TestWorld_Destroy_Success(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition, model.CBot)
	e := mustCreateEntity(t, w, makeBundle(mask, 100))

	if err := w.destroyNow(e); err != nil {
		t.Fatalf("destroyNow error = %v", err)
	}

	_, err := w.resolve(e)
	if err == nil {
		t.Fatal("entity should be dead after destroy")
	}

	if _, found := w.botIndex[profileIDForTest(100)]; found {
		t.Fatal("bot profile ID should be removed from botIndex")
	}
}

func TestWorld_Destroy_FreesIndex(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition, model.CBot)
	e1 := mustCreateEntity(t, w, makeBundle(mask, 1))

	w.destroyNow(e1)
	e2 := w.allocateEntity()

	if e2.Index != e1.Index {
		t.Fatalf("reused Index = %d, want %d", e2.Index, e1.Index)
	}
	if e2.Generation != e1.Generation+1 {
		t.Fatalf("Generation = %d, want %d (incremented)", e2.Generation, e1.Generation+1)
	}
}

func TestWorld_Destroy_MovesLastEntity(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition, model.CBot)

	e0 := mustCreateEntity(t, w, makeBundle(mask, 1))
	e1 := mustCreateEntity(t, w, makeBundle(mask, 2))
	e2 := mustCreateEntity(t, w, makeBundle(mask, 3))

	w.destroyNow(e0)

	loc0, _ := w.resolve(e0)
	if loc0.alive {
		t.Fatal("e0 should be dead")
	}

	loc1, _ := w.resolve(e1)
	if !loc1.alive {
		t.Fatal("e1 should still be alive")
	}

	loc2, err := w.resolve(e2)
	if err != nil {
		t.Fatalf("resolve e2 error = %v", err)
	}
	if !loc2.alive {
		t.Fatal("e2 should still be alive")
	}
	if loc2.row != 0 {
		t.Fatalf("e2.row = %d, want 0 (was moved from end)", loc2.row)
	}
}

func TestWorld_Migrate_InvalidEntity(t *testing.T) {
	w := NewWorld()
	err := w.migrateNow(Entity{}, model.Mask(0), Bundle{})
	if err == nil {
		t.Fatal("expected error for invalid entity")
	}
}

func TestWorld_Migrate_MaskMismatch(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition)
	e := mustCreateEntity(t, w, makeBundle(mask, 0))

	var dest Bundle
	dest.Set(model.CHealth, model.Health{Max: 20})

	err := w.migrateNow(e, model.Components(model.CVelocity), dest)
	if err == nil {
		t.Fatal("expected error for mask mismatch")
	}
}

func TestWorld_Migrate_InvalidDest(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition)
	e := mustCreateEntity(t, w, makeBundle(mask, 0))

	err := w.migrateNow(e, mask, Bundle{})
	if err == nil {
		t.Fatal("expected error for invalid dest bundle")
	}
}

func TestWorld_Migrate_NoBot(t *testing.T) {
	w := NewWorld()
	srcMask := model.Components(model.CPosition)
	e := mustCreateEntity(t, w, makeBundle(srcMask, 0))

	destMask := model.Components(model.CPosition, model.CHealth)
	var dest Bundle
	dest.Set(model.CPosition, model.Position{X: 99})
	dest.Set(model.CHealth, model.Health{Max: 20})

	if err := w.migrateNow(e, srcMask, dest); err != nil {
		t.Fatalf("migrateNow error = %v", err)
	}

	loc, err := w.resolve(e)
	if err != nil {
		t.Fatalf("resolve after migrate error = %v", err)
	}
	if loc.mask != destMask {
		t.Fatalf("loc.mask = %v, want %v", loc.mask, destMask)
	}
}

func TestWorld_Migrate_Success(t *testing.T) {
	w := NewWorld()
	srcMask := model.Components(model.CPosition, model.CBot)
	e := mustCreateEntity(t, w, makeBundle(srcMask, 100))

	destMask := model.Components(model.CPosition, model.CBot, model.CHealth)
	dest := makeBundle(destMask, 100)

	if err := w.migrateNow(e, srcMask, dest); err != nil {
		t.Fatalf("migrateNow error = %v", err)
	}

	loc, err := w.resolve(e)
	if err != nil {
		t.Fatalf("resolve after migrate error = %v", err)
	}
	if loc.mask != destMask {
		t.Fatalf("loc.mask = %v, want %v", loc.mask, destMask)
	}

	if _, found := w.botIndex[profileIDForTest(100)]; !found {
		t.Fatal("bot profile ID should still be in botIndex")
	}
}

func TestWorld_Migrate_BotRemoved(t *testing.T) {
	w := NewWorld()
	srcMask := model.Components(model.CPosition, model.CBot)
	e := mustCreateEntity(t, w, makeBundle(srcMask, 100))

	destMask := model.Components(model.CPosition)
	dest := makeBundle(destMask, 0)

	if err := w.migrateNow(e, srcMask, dest); err != nil {
		t.Fatalf("migrateNow error = %v", err)
	}

	if _, found := w.botIndex[profileIDForTest(100)]; found {
		t.Fatal("bot profile ID should be removed from botIndex")
	}
}

func TestWorld_Migrate_MovesLastEntity(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition, model.CBot)

	e0 := mustCreateEntity(t, w, makeBundle(mask, 1))
	e1 := mustCreateEntity(t, w, makeBundle(mask, 2))
	e2 := mustCreateEntity(t, w, makeBundle(mask, 3))

	destMask := model.Components(model.CPosition, model.CBot, model.CHealth)
	dest := makeBundle(destMask, 1)

	w.migrateNow(e0, mask, dest)

	loc1, _ := w.resolve(e1)
	if !loc1.alive {
		t.Fatal("e1 should still be alive")
	}

	loc2, err := w.resolve(e2)
	if err != nil {
		t.Fatalf("resolve e2 error = %v", err)
	}
	if !loc2.alive {
		t.Fatal("e2 should still be alive")
	}
	if loc2.row != 0 {
		t.Fatalf("e2.row = %d, want 0 (was moved from end)", loc2.row)
	}
}

func TestWorld_CreateDestroyCycle(t *testing.T) {
	w := NewWorld()
	mask := model.Components(model.CPosition, model.CBot)

	e1 := mustCreateEntity(t, w, makeBundle(mask, 100))

	b, _ := w.bundle(e1)
	pos := b.Get(model.CPosition).(model.Position)
	wantX := float64(100*10) + 1
	if pos.X != wantX {
		t.Fatalf("Position.X = %f, want %f", pos.X, wantX)
	}

	w.destroyNow(e1)

	_, err := w.resolve(e1)
	if err == nil {
		t.Fatal("entity should be dead")
	}
	if _, found := w.botIndex[profileIDForTest(100)]; found {
		t.Fatal("bot should be removed from botIndex")
	}

	e2 := w.allocateEntity()
	if e2.Index != e1.Index {
		t.Fatalf("reused Index = %d, want %d", e2.Index, e1.Index)
	}
	if e2.Generation != 2 {
		t.Fatalf("Generation = %d, want 2", e2.Generation)
	}
}
