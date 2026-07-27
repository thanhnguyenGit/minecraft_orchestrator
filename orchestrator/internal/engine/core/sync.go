package core

import (
	"cmp"
	"fmt"
	"slices"

	"minecraft_orchestrator/internal/engine/model"
)

// Validate all queued structural effects, reserve destination storage,
// apply them in deterministic order, then clear the queue and dirty set.
func (w *World) Sync() error {
	if len(w.queue) == 0 {
		clear(w.dirty)
		return nil
	}

	slices.SortStableFunc(w.queue, func(a, b Envelop) int {
		if result := cmp.Compare(a.SystemOrder, b.SystemOrder); result != 0 {
			return result
		}

		return cmp.Compare(a.Sequence, b.Sequence)
	})

	shadow := NewShadowState(w)
	validated := make([]validatedCommand, 0, len(w.queue))

	for index, envelop := range w.queue {
		command, err := envelop.Command.validate(w, shadow)
		if err != nil {
			return fmt.Errorf("sync validation failed at command %d (%s): %w", index, envelop.Command, err)
		}

		validated = append(validated, command)
	}

	// Reserve before any logical mutation.
	// If validation fails, no entity or archetype membership has changed
	reserve := make(map[model.Mask]int)
	for _, cmd := range validated {
		if mask, ok := cmd.destinationMask(); ok {
			reserve[mask]++
		}
	}

	for mask, count := range reserve {
		w.ensureTable(mask).reserve(count)
	}

	for idx, cmd := range validated {
		if err := cmd.apply(w); err != nil {
			return fmt.Errorf("sync commit invariant failed at command %d: %w", idx, err)
		}
	}

	w.queue = w.queue[:0]
	clear(w.dirty)
	
	return nil
}
