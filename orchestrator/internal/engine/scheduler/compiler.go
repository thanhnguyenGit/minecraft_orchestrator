package scheduler

import (
	"cmp"
	"fmt"
	"slices"

	"minecraft_orchestrator/internal/engine/common"
	"minecraft_orchestrator/internal/engine/model"
)

func (b *Builder) Compile() (*ExecutionPlan, error) {
	phases, err := b.sortdPhase()
	if err != nil {
		return nil, err
	}

	executionPlan := &ExecutionPlan{}
	dirty := make(map[model.Mask]struct{})
	globalSystemOrder := 0

	for _, phase := range phases {
		waves, err := b.compilePhase(phase)
		if err != nil {
			return nil, err
		}

		for waveIndex, waveDefs := range waves {
			if touched, masks := waveTouchesDirty(waveDefs, dirty); touched {
				executionPlan.Nodes = append(executionPlan.Nodes, PlanNode{
					Kind:   NodeSync,
					Phase:  phase.id,
					Reason: "next wave queries dirty archetype(s): " + common.FormatMasks(masks),
				})

				clear(dirty)
			}

			compiled := make([]CompiledSystem, 0, len(waveDefs))
			waveDirty := make([]model.Mask, 0)

			for _, def := range waveDefs {
				compiled = append(compiled, CompiledSystem{
					System: def.system,
					Order:  globalSystemOrder,
				})
				globalSystemOrder++

				for _, mask := range def.system.Access().Structural {
					dirty[mask] = struct{}{}
					waveDirty = appendUniqueMask(waveDirty, mask)
				}
			}

			executionPlan.Nodes = append(executionPlan.Nodes, PlanNode{
				Kind:       NodeWave,
				Phase:      phase.id,
				Wave:       waveIndex,
				Systems:    compiled,
				DirtyAfter: waveDirty,
			})
		}
	}

	mapKeysFunc := func(values map[model.Mask]struct{}) []model.Mask {
		result := make([]model.Mask, 0, len(values))

		for mask := range values {
			result = append(result, mask)
		}

		slices.Sort(result)
		return result
	}

	if len(dirty) > 0 {
		masks := mapKeysFunc(dirty)
		phase := PhaseID("finalize")

		if len(executionPlan.Nodes) > 0 {
			phase = executionPlan.Nodes[len(executionPlan.Nodes)-1].Phase
		}
		executionPlan.Nodes = append(executionPlan.Nodes, PlanNode{
			Kind:   NodeSync,
			Phase:  phase,
			Reason: "end-of-frame commit for " + common.FormatMasks(masks),
		})
	}

	return executionPlan, nil
}

func (b *Builder) sortdPhase() ([]*phaseDef, error) {
	indegree := make(map[PhaseID]int, len(b.phases))
	out := make(map[PhaseID][]PhaseID, len(b.phases))

	for id := range b.phases {
		indegree[id] = 0
	}

	for _, phase := range b.phases {
		for _, dependency := range phase.after {
			if b.phases[dependency] == nil {
				return nil, fmt.Errorf("phase %q depend on unknown phase %q", phase.id, dependency)
			}

			out[dependency] = append(out[dependency], phase.id)
			indegree[phase.id]++
		}
	}

	ready := make([]*phaseDef, 0)
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, b.phases[id])
		}
	}

	slices.SortFunc(ready, func(a, b *phaseDef) int {
		return cmp.Compare(a.order, b.order)
	})

	result := make([]*phaseDef, 0, len(b.phases))
	for len(ready) > 0 {
		phase := ready[0]
		ready = ready[1:]
		result = append(result, phase)

		for _, next := range out[phase.id] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, b.phases[next])
				slices.SortFunc(ready, func(a, b *phaseDef) int {
					return cmp.Compare(a.order, b.order)
				})
			}
		}
	}

	if len(result) != len(b.phases) {
		return nil, fmt.Errorf("phase dependency cycle detected")
	}

	return result, nil
}

func (b *Builder) compilePhase(phase *phaseDef) ([][]*systemDef, error) {
	indegree := make(map[SystemID]int, len(phase.systems))
	out := make(map[SystemID][]SystemID, len(phase.systems))
	local := make(map[SystemID]*systemDef, len(phase.systems))

	for _, sysDef := range phase.systems {
		id := sysDef.system.ID()
		indegree[id] = 0
		local[id] = sysDef
	}

	addEdge := func(from, to SystemID) error {
		if local[from] == nil {
			return fmt.Errorf("phase %q: dependency references system %q outside this phase", phase.id, from)
		}

		if local[to] == nil {
			return fmt.Errorf("phase %q: dependency references system %q outside this phase", phase.id, to)
		}

		if slices.Contains(out[from], to) {
			return nil
		}

		out[from] = append(out[from], to)
		indegree[to]++

		return nil
	}

	for _, sysDef := range phase.systems {
		id := sysDef.system.ID()

		for _, dependency := range sysDef.after {
			if err := addEdge(dependency, id); err != nil {
				return nil, err
			}
		}

		for _, next := range sysDef.before {
			if err := addEdge(id, next); err != nil {
				return nil, err
			}
		}
	}

	remaining := len(phase.systems)
	waves := make([][]*systemDef, 0)

	for remaining > 0 {
		ready := make([]*systemDef, 0)

		for id, degree := range indegree {
			if degree == 0 {
				ready = append(ready, local[id])
			}
		}

		slices.SortFunc(ready, func(a, b *systemDef) int {
			return cmp.Compare(a.order, b.order)
		})

		if len(ready) == 0 {
			return nil, fmt.Errorf("phase %q contains a system dependency cycle", phase.id)
		}

		wave := make([]*systemDef, 0, len(ready))
		for _, candidate := range ready {
			compatible := true
			for _, selected := range wave {
				if systemsConflict(candidate.system.Access(), selected.system.Access()) {
					compatible = false
					break
				}
			}

			if compatible {
				wave = append(wave, candidate)
			}
		}

		waves = append(waves, wave)
		for _, selected := range wave {
			id := selected.system.ID()
			delete(indegree, id)
			remaining--
			for _, next := range out[id] {
				indegree[next]--
			}
		}
	}

	return waves, nil
}

// systemsConflict(a, b) checks three categories:
// - Ordinary component read/write conflicts
// - Structural producer versus query conflict
// - Structural producer versus structural producer conflict
func systemsConflict(a, b AccessSpec) bool {
	if a.Writes.Intersects(b.Reads|b.Writes) || b.Writes.Intersects(a.Reads|a.Writes) {
		return true
	}

	// A structural effect cannot share a wave with a system whose query would
	// observe the affected archetype. Both systems must see the same stable pre-wave table membership
	for _, dirty := range a.Structural {
		if slices.ContainsFunc(b.Queries, dirty.Contains) {
			return true
		}
	}

	for _, dirty := range b.Structural {
		if slices.ContainsFunc(a.Queries, dirty.Contains) {
			return true
		}
	}

	if len(a.Structural) > 0 && len(b.Structural) > 0 {
		return true
	}

	return false
}

func waveTouchesDirty(wave []*systemDef, dirty map[model.Mask]struct{}) (bool, []model.Mask) {
	matched := make([]model.Mask, 0)

	for _, sysDef := range wave {
		for _, query := range sysDef.system.Access().Queries {
			for mask := range dirty {
				if mask.Contains(query) {
					matched = appendUniqueMask(matched, mask)
				}
			}
		}
	}

	return len(matched) > 0, matched
}

func appendUniqueMask(masks []model.Mask, candidate model.Mask) []model.Mask {
	if slices.Contains(masks, candidate) {
		return masks
	}

	return append(masks, candidate)
}
