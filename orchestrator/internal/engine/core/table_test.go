package core

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestBundle_Set(t *testing.T) {
	var b Bundle
	b.Set(model.CPosition, model.Position{X: 1, Y: 2, Z: 3})

	if !b.Mask.Has(model.CPosition) {
		t.Fatal("mask should have CPosition after Set")
	}

	pos, ok := b.Components[model.CPosition].(model.Position)
	if !ok {
		t.Fatal("component data should be Position type")
	}
	if pos.X != 1 || pos.Y != 2 || pos.Z != 3 {
		t.Fatalf("position = %+v, want {1,2,3}", pos)
	}
}

func TestBundle_Validate_Empty(t *testing.T) {
	var b Bundle
	err := b.Validate()
	if err == nil {
		t.Fatal("expected error for empty bundle")
	}
}

func TestBundle_Validate_NegativeMaxHealth(t *testing.T) {
	var b Bundle
	b.Set(model.CHealth, model.Health{Max: -1})
	err := b.Validate()
	if err == nil {
		t.Fatal("expected error for negative max health")
	}
}

func TestBundle_Validate_ZeroBotID(t *testing.T) {
	var b Bundle
	b.Set(model.CBot, model.Bot{ID: 0})
	err := b.Validate()
	if err == nil {
		t.Fatal("expected error for zero bot ID")
	}
}

func TestBundle_Validate_Success(t *testing.T) {
	var b Bundle
	b.Set(model.CBot, model.Bot{ID: 1})
	b.Set(model.CHealth, model.Health{Max: 20})
	if err := b.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestBundle_Validate_NoHealthNoBot(t *testing.T) {
	var b Bundle
	b.Set(model.CPosition, model.Position{X: 1})
	if err := b.Validate(); err != nil {
		t.Fatalf("bundle with only Position should be valid, got %v", err)
	}
}

func TestNewColumn(t *testing.T) {
	col := NewColumn[model.Position]()
	if col == nil {
		t.Fatal("NewColumn returned nil")
	}
	if col.Len() != 0 {
		t.Fatalf("Len = %d, want 0", col.Len())
	}
}

func TestColumn_AppendRaw(t *testing.T) {
	col := NewColumn[model.Position]()
	col.AppendRaw(model.Position{X: 1})
	col.AppendRaw(model.Position{X: 2})

	if col.Len() != 2 {
		t.Fatalf("Len = %d, want 2", col.Len())
	}

	got := col.GetRaw(1).(model.Position)
	want := model.Position{X: 2}
	if got != want {
		t.Fatalf("GetRaw(1) = %+v, want %+v", got, want)
	}
}

func TestColumn_GetRaw(t *testing.T) {
	col := NewColumn[int]()
	col.AppendRaw(10)
	col.AppendRaw(20)

	if got := col.GetRaw(0).(int); got != 10 {
		t.Fatalf("GetRaw(0) = %d, want 10", got)
	}
	if got := col.GetRaw(1).(int); got != 20 {
		t.Fatalf("GetRaw(1) = %d, want 20", got)
	}
}

func TestColumn_Reserve(t *testing.T) {
	col := NewColumn[int]()
	oldCap := cap(col.Data)
	col.Reserve(100)

	if cap(col.Data) <= oldCap {
		t.Fatalf("cap after Reserve(100) = %d, should be > %d", cap(col.Data), oldCap)
	}
	if col.Len() != 0 {
		t.Fatalf("Len after Reserve = %d, want 0 (reserve should not change length)", col.Len())
	}
}

func TestColumn_RemoveSwap_NotLast(t *testing.T) {
	col := NewColumn[int]()
	col.AppendRaw(10)
	col.AppendRaw(20)
	col.AppendRaw(30)

	col.RemoveSwap(0)

	if col.Len() != 2 {
		t.Fatalf("Len = %d, want 2", col.Len())
	}
	if got := col.GetRaw(0).(int); got != 30 {
		t.Fatalf("GetRaw(0) = %d, want 30 (last element swapped in)", got)
	}
	if got := col.GetRaw(1).(int); got != 20 {
		t.Fatalf("GetRaw(1) = %d, want 20", got)
	}
}

func TestColumn_RemoveSwap_Last(t *testing.T) {
	col := NewColumn[int]()
	col.AppendRaw(10)
	col.AppendRaw(20)

	col.RemoveSwap(1)

	if col.Len() != 1 {
		t.Fatalf("Len = %d, want 1", col.Len())
	}
	if got := col.GetRaw(0).(int); got != 10 {
		t.Fatalf("GetRaw(0) = %d, want 10", got)
	}
}

func TestColumn_RemoveSwap_Single(t *testing.T) {
	col := NewColumn[int]()
	col.AppendRaw(10)

	col.RemoveSwap(0)

	if col.Len() != 0 {
		t.Fatalf("Len = %d, want 0", col.Len())
	}
}

func TestGrow_NoExtra(t *testing.T) {
	s := make([]int, 0, 4)
	result := grow(s, 0)

	if len(result) != 0 {
		t.Fatalf("len = %d, want 0", len(result))
	}
	if cap(result) != 4 {
		t.Fatalf("cap = %d, want 4", cap(result))
	}
	if &result[0:1][0] != &s[0:1][0] {
		t.Fatal("should return same backing array for no extra")
	}
}

func TestGrow_NoExtra_Negative(t *testing.T) {
	s := make([]int, 0, 4)
	result := grow(s, -1)

	if len(result) != 0 || cap(result) != 4 {
		t.Fatalf("len=%d cap=%d, want len=0 cap=4", len(result), cap(result))
	}
}

func TestGrow_SufficientCapacity(t *testing.T) {
	s := make([]int, 0, 8)
	result := grow(s, 8)

	if cap(result) != 8 {
		t.Fatalf("cap = %d, want 8 (existing capacity sufficient)", cap(result))
	}
}

func TestGrow_NeedsGrowth(t *testing.T) {
	s := make([]int, 0, 4)
	result := grow(s, 10)

	if cap(result) < 10 {
		t.Fatalf("cap = %d, want at least 10", cap(result))
	}
	if len(result) != 0 {
		t.Fatalf("len = %d, want 0", len(result))
	}
}

func TestGrow_DoublesCapacity(t *testing.T) {
	s := make([]int, 100)
	result := grow(s, 50)

	if cap(result) != 200 {
		t.Fatalf("cap = %d, want 200 (100*2 > 100+50, doubled)", cap(result))
	}
}

func TestGrow_MinimumCapacity(t *testing.T) {
	s := make([]int, 0, 2)
	result := grow(s, 5)

	if cap(result) != 16 {
		t.Fatalf("cap = %d, want 16 (minimum capacity)", cap(result))
	}
}

func TestGrow_PreservesElements(t *testing.T) {
	s := []int{1, 2, 3}
	s = s[:2]
	result := grow(s, 10)

	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0] != 1 || result[1] != 2 {
		t.Fatalf("elements = %v, want [1,2]", result)
	}
}

func TestNewTable_SingleComponent(t *testing.T) {
	mask := model.Components(model.CPosition)
	tbl := NewTable(mask)

	if tbl.mask != mask {
		t.Fatalf("mask = %v, want %v", tbl.mask, mask)
	}
	if tbl.Len() != 0 {
		t.Fatalf("Len = %d, want 0", tbl.Len())
	}
	if _, ok := tbl.columns[uint8(model.CPosition)]; !ok {
		t.Fatal("expected CPosition column")
	}
	if _, ok := tbl.columns[uint8(model.CVelocity)]; ok {
		t.Fatal("unexpected CVelocity column")
	}
}

func TestNewTable_MultipleComponents(t *testing.T) {
	mask := model.Components(model.CPosition, model.CVelocity, model.CBot)
	tbl := NewTable(mask)

	if tbl.mask != mask {
		t.Fatalf("mask = %v, want %v", tbl.mask, mask)
	}
	if _, ok := tbl.columns[uint8(model.CPosition)]; !ok {
		t.Fatal("expected CPosition column")
	}
	if _, ok := tbl.columns[uint8(model.CVelocity)]; !ok {
		t.Fatal("expected CVelocity column")
	}
	if _, ok := tbl.columns[uint8(model.CBot)]; !ok {
		t.Fatal("expected CBot column")
	}
}

func TestNewTable_EmptyMask(t *testing.T) {
	tbl := NewTable(0)
	if tbl.Len() != 0 {
		t.Fatalf("Len = %d, want 0", tbl.Len())
	}
	if len(tbl.columns) != 0 {
		t.Fatalf("columns count = %d, want 0", len(tbl.columns))
	}
}

func TestNewTable_PanicsOnMissingConstructor(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing column constructor")
		}
	}()
	NewTable(model.Components(model.CHunger))
}

func TestTable_AddEntity_MaskMismatch(t *testing.T) {
	tbl := NewTable(model.Components(model.CPosition, model.CVelocity))

	var b Bundle
	b.Set(model.CPosition, model.Position{X: 1})

	_, err := tbl.AddEntity(Entity{Index: 1}, b)
	if err == nil {
		t.Fatal("expected error for mask mismatch")
	}
}

func TestTable_AddEntity_Success(t *testing.T) {
	tbl := NewTable(model.Components(model.CPosition, model.CVelocity))

	e := Entity{Index: 1, Generation: 1}

	var b Bundle
	b.Set(model.CPosition, model.Position{X: 1, Y: 2, Z: 3})
	b.Set(model.CVelocity, model.Velocity{X: 0.1, Y: 0.2, Z: 0.3})

	if _, err := tbl.AddEntity(e, b); err != nil {
		t.Fatalf("AddEntity error = %v", err)
	}
	if tbl.Len() != 1 {
		t.Fatalf("Len = %d, want 1", tbl.Len())
	}

	got := tbl.bundleAt(0)
	pos := got.Components[model.CPosition].(model.Position)
	if pos.X != 1 || pos.Y != 2 || pos.Z != 3 {
		t.Fatalf("position = %+v, want {1,2,3}", pos)
	}
	vel := got.Components[model.CVelocity].(model.Velocity)
	if vel.X != 0.1 || vel.Y != 0.2 || vel.Z != 0.3 {
		t.Fatalf("velocity = %+v, want {0.1,0.2,0.3}", vel)
	}
}

func TestTable_Len(t *testing.T) {
	tbl := NewTable(model.Components(model.CPosition))

	for i := range 5 {
		tbl.entities = append(tbl.entities, Entity{Index: uint32(i)})
	}

	if tbl.Len() != 5 {
		t.Fatalf("Len = %d, want 5", tbl.Len())
	}
}

func TestTable_BundleAt(t *testing.T) {
	tbl := NewTable(model.Components(model.CPosition, model.CBot))

	e1 := Entity{Index: 1, Generation: 1}
	b1 := Bundle{Mask: tbl.mask}
	b1.Set(model.CPosition, model.Position{X: 1, Y: 2, Z: 3})
	b1.Set(model.CBot, model.Bot{ID: 100})

	e2 := Entity{Index: 2, Generation: 1}
	b2 := Bundle{Mask: tbl.mask}
	b2.Set(model.CPosition, model.Position{X: 4, Y: 5, Z: 6})
	b2.Set(model.CBot, model.Bot{ID: 200})

	tbl.AddEntity(e1, b1)
	tbl.AddEntity(e2, b2)

	got := tbl.bundleAt(1)
	pos := got.Components[model.CPosition].(model.Position)
	if pos.X != 4 {
		t.Fatalf("bundleAt(1).Position.X = %f, want 4", pos.X)
	}
	bot := got.Components[model.CBot].(model.Bot)
	if bot.ID != 200 {
		t.Fatalf("bundleAt(1).Bot.ID = %d, want 200", bot.ID)
	}
}

func TestTable_Reserve(t *testing.T) {
	tbl := NewTable(model.Components(model.CPosition, model.CVelocity))

	tbl.reserve(10)

	if cap(tbl.entities) < 10 {
		t.Fatalf("entities cap = %d, want at least 10", cap(tbl.entities))
	}
	if len(tbl.entities) != 0 {
		t.Fatalf("entities len = %d, want 0 (reserve should not change length)", len(tbl.entities))
	}
}

func TestTable_RemoveSwap_OutOfRange(t *testing.T) {
	tbl := NewTable(model.Components(model.CPosition))

	_, _, _, err := tbl.removeSwap(-1)
	if err == nil {
		t.Fatal("expected error for negative row")
	}

	_, _, _, err = tbl.removeSwap(0)
	if err == nil {
		t.Fatal("expected error for row out of range on empty table")
	}
}

func TestTable_RemoveSwap_NotLast(t *testing.T) {
	mask := model.Components(model.CPosition, model.CBot)
	tbl := NewTable(mask)

	e0 := Entity{Index: 0, Generation: 1}
	b0 := Bundle{Mask: mask}
	b0.Set(model.CPosition, model.Position{X: 0})
	b0.Set(model.CBot, model.Bot{ID: 1})

	e1 := Entity{Index: 1, Generation: 1}
	b1 := Bundle{Mask: mask}
	b1.Set(model.CPosition, model.Position{X: 1})
	b1.Set(model.CBot, model.Bot{ID: 2})

	e2 := Entity{Index: 2, Generation: 1}
	b2 := Bundle{Mask: mask}
	b2.Set(model.CPosition, model.Position{X: 2})
	b2.Set(model.CBot, model.Bot{ID: 3})

	tbl.AddEntity(e0, b0)
	tbl.AddEntity(e1, b1)
	tbl.AddEntity(e2, b2)

	removed, moved, didMove, err := tbl.removeSwap(0)

	if err != nil {
		t.Fatalf("removeSwap error = %v", err)
	}
	if !didMove {
		t.Fatal("didMove should be true")
	}
	if moved != e2 {
		t.Fatalf("moved = %v, want %v", moved, e2)
	}

	removedPart := removed.Components[model.CPosition].(model.Position)
	if removedPart.X != 0 {
		t.Fatalf("removed position.X = %f, want 0", removedPart.X)
	}

	if tbl.Len() != 2 {
		t.Fatalf("Len after remove = %d, want 2", tbl.Len())
	}

	finalPos := tbl.bundleAt(0).Components[model.CPosition].(model.Position)
	if finalPos.X != 2 {
		t.Fatalf("bundleAt(0).Position.X = %f, want 2 (was moved from end)", finalPos.X)
	}
}

func TestTable_RemoveSwap_Last(t *testing.T) {
	mask := model.Components(model.CPosition)
	tbl := NewTable(mask)

	e0 := Entity{Index: 0, Generation: 1}
	b0 := Bundle{Mask: mask}
	b0.Set(model.CPosition, model.Position{X: 0})

	e1 := Entity{Index: 1, Generation: 1}
	b1 := Bundle{Mask: mask}
	b1.Set(model.CPosition, model.Position{X: 1})

	tbl.AddEntity(e0, b0)
	tbl.AddEntity(e1, b1)

	removed, moved, didMove, err := tbl.removeSwap(1)

	if err != nil {
		t.Fatalf("removeSwap error = %v", err)
	}
	if didMove {
		t.Fatal("didMove should be false for last row")
	}
	if moved.IsZero() == false {
		t.Fatalf("moved should be zero Entity for last row, got %v", moved)
	}

	pos := removed.Components[model.CPosition].(model.Position)
	if pos.X != 1 {
		t.Fatalf("removed position.X = %f, want 1", pos.X)
	}

	if tbl.Len() != 1 {
		t.Fatalf("Len after remove = %d, want 1", tbl.Len())
	}
}

func TestTable_RemoveSwap_Single(t *testing.T) {
	mask := model.Components(model.CPosition)
	tbl := NewTable(mask)

	e := Entity{Index: 0, Generation: 1}
	b := Bundle{Mask: mask}
	b.Set(model.CPosition, model.Position{X: 42})

	tbl.AddEntity(e, b)

	removed, _, didMove, err := tbl.removeSwap(0)

	if err != nil {
		t.Fatalf("removeSwap error = %v", err)
	}
	if didMove {
		t.Fatal("didMove should be false for single element")
	}

	pos := removed.Components[model.CPosition].(model.Position)
	if pos.X != 42 {
		t.Fatalf("removed position.X = %f, want 42", pos.X)
	}

	if tbl.Len() != 0 {
		t.Fatalf("Len after remove = %d, want 0", tbl.Len())
	}
}

func TestTable_RemoveSwap_ClearsColumn(t *testing.T) {
	mask := model.Components(model.CPosition, model.CBot)
	tbl := NewTable(mask)

	e0 := Entity{Index: 0, Generation: 1}
	b0 := Bundle{Mask: mask}
	b0.Set(model.CPosition, model.Position{X: 100})
	b0.Set(model.CBot, model.Bot{ID: 1})

	e1 := Entity{Index: 1, Generation: 1}
	b1 := Bundle{Mask: mask}
	b1.Set(model.CPosition, model.Position{X: 200})
	b1.Set(model.CBot, model.Bot{ID: 2})

	tbl.AddEntity(e0, b0)
	tbl.AddEntity(e1, b1)

	tbl.removeSwap(0)

	posCol := tbl.columns[uint8(model.CPosition)].(*Column[model.Position])
	botCol := tbl.columns[uint8(model.CBot)].(*Column[model.Bot])

	if posCol.Len() != 1 {
		t.Fatalf("Position column Len = %d, want 1", posCol.Len())
	}
	if botCol.Len() != 1 {
		t.Fatalf("Bot column Len = %d, want 1", botCol.Len())
	}

	if posCol.GetRaw(0).(model.Position).X != 200 {
		t.Fatalf("remaining position.X = %f, want 200", posCol.GetRaw(0).(model.Position).X)
	}
}
