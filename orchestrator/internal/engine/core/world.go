package core

//go:generate go run gen.go

import (
	"fmt"
	"slices"

	"minecraft_orchestrator/internal/engine/model"
)

// World internal record for one entity index 
type location struct {
	// archetype mask table which the entity belong to
	mask       model.Mask
	row        int
	generation uint32
	alive      bool
}

type World struct {
	// exact-match table lookup and deterministic mask-order iteration
	tables     map[model.Mask]*Table
	tableOrder []model.Mask

	// entity-to-table-row lookup and free-index reuse
	locations   []location
	generations []uint32
	free        []uint32

	// bot ID to live Entity
	botIndex map[uint64]Entity

	// deferred effects and declared affected archetypes
	queue []Envelop
	dirty map[model.Mask]struct{}
}

func NewWorld() *World {
	return &World{
		tables:   make(map[model.Mask]*Table),
		botIndex: make(map[uint64]Entity),
		dirty:    make(map[model.Mask]struct{}),
		// Reserve index 0 so a zero Entity is always invalid.
		locations:   make([]location, 1),
		generations: make([]uint32, 1),
	}
}

func (w *World) ensureTable(mask model.Mask) *Table {
	if t := w.tables[mask]; t != nil {
		return t
	}

	w.tables[mask] = NewTable(mask)
	w.tableOrder = append(w.tableOrder, mask)
	slices.Sort(w.tableOrder)

	return w.tables[mask]
}

func (w *World) matching(required model.Mask) []*Table {
	result := make([]*Table, 0)

	for _, mask := range w.tableOrder {
		if mask.Contains(required) {
			result = append(result, w.tables[mask])
		}
	}

	return result
}

func (w *World) allocateEntity() Entity {
	if n := len(w.free); n > 0 {
		index := w.free[n-1]
		w.free = w.free[:n-1]
		generation := w.generations[index]
		return Entity{
			Index:      index,
			Generation: generation,
		}
	}

	index := uint32(len(w.generations))
	w.generations = append(w.generations, 1)
	w.locations = append(w.locations, location{})
	return Entity{
		Index:      index,
		Generation: 1,
	}
}

func (w *World) resolve(e Entity) (location, error) {
	if e.Index == 0 || int(e.Index) >= len(w.locations) {
		return location{}, fmt.Errorf("entity %s has invalid index", e)
	}

	loc := w.locations[e.Index]

	if !loc.alive || loc.generation != e.Generation {
		return location{}, fmt.Errorf("entity %s us stale or dead", e)
	}

	return loc, nil
}

func (w *World) bundle(e Entity) (Bundle, error) {
	loc, err := w.resolve(e)
	if err != nil {
		return Bundle{}, err
	}

	return w.tables[loc.mask].bundleAt(loc.row), nil
}

func (w *World) createNow(bundle Bundle) (Entity, error) {
	if err := bundle.Validate(); err != nil {
		return Entity{}, err
	}

	if bundle.Mask.Has(model.CBot) {
		botData, ok := bundle.Get(model.CBot).(model.Bot)

		if !ok {
			return Entity{}, fmt.Errorf("corrupted bundle: CBot mask set but data is not model.CBot")
		}

		if existing, found := w.botIndex[botData.ID]; found {
			return existing, fmt.Errorf("bot with ID %d already mapped to entity %s", botData.ID, existing)
		}
	}

	entity := w.allocateEntity()
	table := w.ensureTable(bundle.Mask)
	row, err := table.AddEntity(entity, bundle)
	if err != nil {
		return Entity{}, err
	}

	w.locations[entity.Index] = location{
		mask:       bundle.Mask,
		row:        row,
		generation: entity.Generation,
		alive:      true,
	}

	if bundle.Mask.Has(model.CBot) {
		botData, ok := bundle.Get(model.CBot).(model.Bot)
		if !ok {
			return Entity{}, fmt.Errorf("corrupted bundle: CBot mask set but data is not model.CBot")
		}

		w.botIndex[botData.ID] = entity
	}

	return entity, nil
}

func (w *World) destroyNow(entity Entity) error {
	loc, err := w.resolve(entity)
	if err != nil {
		return err
	}

	table := w.tables[loc.mask]
	removed, moved, didMove, err := table.removeSwap(loc.row)
	if err != nil {
		return err
	}

	if didMove {
		movedLoc := w.locations[moved.Index]
		movedLoc.row = loc.row
		w.locations[moved.Index] = movedLoc
	}

	if removed.Mask.Has(model.CBot) {
		botData, ok := removed.Get(model.CBot).(model.Bot)
		if !ok {
			return fmt.Errorf("corrupted bundle: CBot mask set but data is not model.CBot")
		}
		delete(w.botIndex, botData.ID)
	}

	w.generations[entity.Index]++
	w.locations[entity.Index] = location{
		generation: w.generations[entity.Index],
	}
	w.free = append(w.free, entity.Index)

	return nil
}

func (w *World) migrateNow(entity Entity, expectedSource model.Mask, dest Bundle) error {
	loc, err := w.resolve(entity)
	if err != nil {
		return err
	}

	if loc.mask != expectedSource {
		return fmt.Errorf("entity %s expected in %s, found in %s", entity, expectedSource, loc.mask)
	}

	if err := dest.Validate(); err != nil {
		return err
	}

	oldTable := w.tables[loc.mask]
	removed, moved, didMove, err := oldTable.removeSwap(loc.row)
	if err != nil {
		return err
	}

	if removed.Mask.Has(model.CBot) {
		botData, ok := removed.Get(model.CBot).(model.Bot)
		if !ok {
			return fmt.Errorf("corrupted bundle: CBot mask set but data is not model.CBot")
		}
		delete(w.botIndex, botData.ID)
	}

	if didMove {
		movedLoc := w.locations[moved.Index]
		movedLoc.row = loc.row
		w.locations[moved.Index] = movedLoc
	}

	newTable := w.ensureTable(dest.Mask)
	row, err := newTable.AddEntity(entity, dest)
	if err != nil {
		return fmt.Errorf("append migration destination after source removal: %w", err)
	}

	w.locations[entity.Index] = location{
		mask:       dest.Mask,
		row:        row,
		generation: entity.Generation,
		alive:      true,
	}

	if dest.Mask.Has(model.CBot) {
		botData, ok := dest.Get(model.CBot).(model.Bot)
		if !ok {
			return fmt.Errorf("corrupted bundle: CBot mask set but data is not model.CBot")
		}
		w.botIndex[botData.ID] = entity
	}

	return nil
}


// func (w *World) Sync() error {
// 	if len(w.queue) == 0 {
// 		clear(w.dirty)
// 		return nil
// 	}

// 	slices.SortFunc(w.queue, func(a,b int) bool {
// 		if w.queue[a].SystemOrder != w.queue[j].SystemOrder {
// 			return 
// 		}
// 	})
// }