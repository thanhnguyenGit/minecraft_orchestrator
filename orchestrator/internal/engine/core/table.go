package core

import (
	"fmt"
	"minecraft_orchestrator/internal/engine/model"
)

type Bundle struct {
	Mask model.Mask

	Components [model.ComponentCount]any
}

func (b *Bundle) Set(c model.Component, data any) {
	b.Mask |= model.Bit(c)
	b.Components[c] = data
}

func (b *Bundle) Get(c model.Component) any {
	if !b.Mask.Has(c) {
		return nil
	}

	return b.Components[c]
}

func (b *Bundle) Validate() error {
	if b.Mask == 0 {
		return fmt.Errorf("bundle has empty component mask")
	}

	if b.Mask.Has(model.CHealth) {
		health := b.Components[model.CHealth].(model.Health)
		if health.Max < 0 {
			return fmt.Errorf("maximum health cannot be negative")
		}
	}

	if b.Mask.Has(model.CBot) {
		bot := b.Components[model.CBot].(model.Bot)
		if bot.ProfileID == (model.ProfileID{}) {
			return fmt.Errorf("bot profile ID must be non-zero")
		}
	}

	return nil
}

type ComponentColumn interface {
	AppendRaw(val any)
	Len() int
	Reserve(extra int)
	GetRaw(row int) any
	RemoveSwap(row int)
}

type Column[T any] struct {
	Data []T
}

func NewColumn[T any]() *Column[T] {
	return &Column[T]{
		Data: make([]T, 0),
	}
}

func (c *Column[T]) AppendRaw(val any) {
	c.Data = append(c.Data, val.(T))
}

func (c *Column[T]) Len() int {
	return len(c.Data)
}

func (c *Column[T]) Reserve(extra int) {
	c.Data = grow(c.Data, extra)
}

func (c *Column[T]) GetRaw(row int) any {
	return c.Data[row]
}

func (c *Column[T]) RemoveSwap(row int) {
	last := len(c.Data) - 1

	if row != last {
		c.Data[row] = c.Data[last]
	}

	var zero T
	c.Data[last] = zero

	c.Data = c.Data[:last]
}

type Table struct {
	mask     model.Mask
	entities []Entity
	columns  map[uint8]ComponentColumn
}

var columnConstructors = map[model.Component]func() ComponentColumn{
	model.CPosition:     func() ComponentColumn { return NewColumn[model.Position]() },
	model.CVelocity:     func() ComponentColumn { return NewColumn[model.Velocity]() },
	model.CHealth:       func() ComponentColumn { return NewColumn[model.Health]() },
	model.CBot:          func() ComponentColumn { return NewColumn[model.Bot]() },
	model.CRotation:     func() ComponentColumn { return NewColumn[model.Rotation]() },
	model.CGameMode:     func() ComponentColumn { return NewColumn[model.GameMode]() },
	model.CSession:      func() ComponentColumn { return NewColumn[model.Session]() },
	model.CInventory:    func() ComponentColumn { return NewColumn[model.Inventory]() },
	model.CEffects:      func() ComponentColumn { return NewColumn[model.Effects]() },
}

func NewTable(mask model.Mask) *Table {
	tbl := &Table{
		mask:     mask,
		entities: make([]Entity, 0),
		columns:  make(map[uint8]ComponentColumn),
	}

	for c := range model.ComponentCount {
		if mask.Has(c) {
			if constructor, ok := columnConstructors[c]; ok {
				tbl.columns[uint8(c)] = constructor()
			} else {
				panic(fmt.Sprintf("missing columns constructor for component %v", c))
			}
		}
	}

	return tbl
}

func (t *Table) Len() int {
	return len(t.entities)
}

func grow[T any](slice []T, extra int) []T {
	if extra <= 0 || cap(slice)-len(slice) >= extra {
		return slice
	}

	newCap := len(slice) + extra

	if doubled := cap(slice) * 2; doubled > newCap {
		newCap = doubled
	}

	if newCap < 16 {
		newCap = 16
	}

	result := make([]T, len(slice), newCap)
	copy(result, slice)

	return result
}

func (tbl *Table) AddEntity(e Entity, b Bundle) (int, error) {
	if b.Mask != tbl.mask {
		return 0, fmt.Errorf("bundle mask %v does not match table mask %v", b.Mask.String(), tbl.mask.String())
	}

	row := len(tbl.entities)
	tbl.entities = append(tbl.entities, e)

	for c := range model.ComponentCount {
		if tbl.mask.Has(c) {
			tbl.columns[uint8(c)].AppendRaw(b.Components[c])
		}
	}

	return row, nil
}

func (t *Table) reserve(extra int) {
	t.entities = grow(t.entities, extra)

	for c := range model.ComponentCount {
		if !t.mask.Has(c) {
			continue
		}

		components := t.columns[uint8(c)]
		components.Reserve(extra)
	}
}

func (t *Table) bundleAt(row int) Bundle {
	bundle := Bundle{
		Mask: t.mask,
	}

	for c := range model.ComponentCount {
		if t.mask.Has(c) {
			bundle.Components[c] = t.columns[uint8(c)].GetRaw(row)
		}
	}

	return bundle
}

func (t *Table) removeSwap(row int) (removed Bundle, moved Entity, didMove bool, err error) {
	if row < 0 || row >= len(t.entities) {
		return Bundle{}, Entity{}, false, fmt.Errorf("row %d out of range", row)
	}

	last := len(t.entities) - 1
	removed = t.bundleAt(row)

	if row != last {
		moved = t.entities[last]

		didMove = true
		t.entities[row] = moved
	}

	t.entities = t.entities[:last]

	for c := range model.ComponentCount {
		if t.mask.Has(c) {
			t.columns[uint8(c)].RemoveSwap(row)
		}
	}

	return removed, moved, didMove, nil
}
