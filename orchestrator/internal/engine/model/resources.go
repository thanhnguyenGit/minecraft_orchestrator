package model

import (
	"fmt"
	"maps"
)

type DimensionKey string

type ChunkPosition struct {
	X, Z int32
}

type Perception struct {
	Dimension    DimensionKey
	CenterChunk  ChunkPosition
	ViewDistance int32
	Revision     uint64
}

func (p *Perception) String() string {
	return fmt.Sprintf("Perception{Dimension: %q, CenterChunk: (%d, %d), ViewDistance: %d, Revision: %d}",
		p.Dimension, p.CenterChunk.X, p.CenterChunk.Z, p.ViewDistance, p.Revision)
}

type DimensionType struct {
	RegistryID int32
	Key        string
	MinY       int32
	Height     int32
}

// WorldView tracks chunks, dimension type registry entries, and perception
// metadata for one active session attempt of a single bot.
type WorldView struct {
	AttemptID               uint64
	DimensionEpoch          uint64
	Perception              Perception
	DimensionTypes          []DimensionType
	ActiveDimensionTypeID   int32
	ActiveDimensionType     DimensionType
	HasDimensionTypeBinding bool
	HasActiveDimensionType  bool
	chunks                  map[ChunkPosition]ChunkColumn
}

// WorldViews is a cache of loaded chunk columns partitioned by bot profile ID.
type WorldViews struct {
	views map[ProfileID]WorldView
}

func (v *WorldViews) BeginAttempt(profileID ProfileID, attemptID uint64) {
	if v.views == nil {
		v.views = make(map[ProfileID]WorldView)
	}
	v.views[profileID] = WorldView{AttemptID: attemptID}
}

func (v *WorldViews) Get(profileID ProfileID) (WorldView, bool) {
	view, ok := v.views[profileID]
	view.DimensionTypes = append([]DimensionType(nil), view.DimensionTypes...)
	return view, ok
}

func (v *WorldViews) SetDimensionTypes(profileID ProfileID, attemptID uint64, types []DimensionType) bool {
	view, ok := v.views[profileID]
	if !ok || view.AttemptID != attemptID {
		return false
	}
	previousGeometry := view.ActiveDimensionType
	hadGeometry := view.HasActiveDimensionType
	view.DimensionTypes = append([]DimensionType(nil), types...)
	if view.HasDimensionTypeBinding {
		active, found := view.dimensionType(view.ActiveDimensionTypeID)
		view.HasActiveDimensionType = found
		if found {
			view.ActiveDimensionType = active
			if hadGeometry && (previousGeometry.MinY != active.MinY || previousGeometry.Height != active.Height) {
				view.clearChunks()
				view.DimensionEpoch++
				view.Perception.Revision++
			}
		} else {
			view.ActiveDimensionType = DimensionType{}
		}
	}
	v.views[profileID] = view
	return true
}

func (v *WorldViews) BindDimension(profileID ProfileID, attemptID uint64, dimensionTypeID int32) bool {
	view, ok := v.views[profileID]
	if !ok || view.AttemptID != attemptID {
		return false
	}
	dimensionType, found := view.dimensionType(dimensionTypeID)
	if !found {
		return false
	}
	view.ActiveDimensionTypeID = dimensionTypeID
	view.ActiveDimensionType = dimensionType
	view.HasDimensionTypeBinding = true
	view.HasActiveDimensionType = true
	v.views[profileID] = view
	return true
}

func (v *WorldViews) UpdatePerception(profileID ProfileID, attemptID uint64, dimension DimensionKey, centerChunk ChunkPosition, viewDistance int32) bool {
	view, ok := v.views[profileID]
	if !ok || view.AttemptID != attemptID {
		return false
	}
	view.Perception.Dimension = dimension
	view.Perception.CenterChunk = centerChunk
	view.Perception.ViewDistance = viewDistance
	view.Perception.Revision++
	v.views[profileID] = view
	return true
}

func (v *WorldViews) Remove(profileID ProfileID, attemptID uint64) bool {
	view, ok := v.views[profileID]
	if !ok || view.AttemptID != attemptID {
		return false
	}
	delete(v.views, profileID)
	return true
}

func (v *WorldViews) ReplaceChunk(profileID ProfileID, attemptID uint64, position ChunkPosition, column ChunkColumn) bool {
	view, ok := v.views[profileID]
	if !ok || view.AttemptID != attemptID || !view.HasActiveDimensionType || !column.geometryMatches(view.ActiveDimensionType) {
		return false
	}
	if view.chunks == nil {
		view.chunks = make(map[ChunkPosition]ChunkColumn)
	}
	if previous, found := view.chunks[position]; found {
		column.version = previous.version + 1
	} else {
		column.version = 1
	}
	view.chunks[position] = column
	v.views[profileID] = view
	return true
}

func (v *WorldViews) UnloadChunk(profileID ProfileID, attemptID uint64, position ChunkPosition) bool {
	view, ok := v.views[profileID]
	if !ok || view.AttemptID != attemptID || !view.HasActiveDimensionType {
		return false
	}
	if _, found := view.chunks[position]; !found {
		return false
	}
	delete(view.chunks, position)
	v.views[profileID] = view
	return true
}

func (v *WorldViews) SetBlockState(profileID ProfileID, attemptID uint64, position BlockPosition, stateID uint32) bool {
	view, ok := v.views[profileID]
	if !ok || view.AttemptID != attemptID || !view.HasActiveDimensionType || position.Y < view.ActiveDimensionType.MinY || position.Y >= view.ActiveDimensionType.MinY+view.ActiveDimensionType.Height {
		return false
	}
	chunk := ChunkPosition{X: floorChunk(position.X), Z: floorChunk(position.Z)}
	column, found := view.chunks[chunk]
	if !found {
		return false
	}
	if !column.setBlockState(position.X-chunk.X*chunkWidth, position.Y, position.Z-chunk.Z*chunkWidth, stateID) {
		return false
	}
	column.version++
	view.chunks[chunk] = column
	v.views[profileID] = view
	return true
}

type BlockUpdate struct {
	Position BlockPosition
	StateID  uint32
}

func (v *WorldViews) SetBlockStates(profileID ProfileID, attemptID uint64, updates []BlockUpdate) bool {
	view, ok := v.views[profileID]
	if !ok || view.AttemptID != attemptID || !view.HasActiveDimensionType {
		return false
	}

	if view.chunks == nil {
		return false
	}

	for _, b := range updates {
		if b.Position.Y < view.ActiveDimensionType.MinY || b.Position.Y >= view.ActiveDimensionType.MinY+view.ActiveDimensionType.Height {
			continue
		}

		chunkKey := ChunkPosition{
			X: floorChunk(b.Position.X),
			Z: floorChunk(b.Position.Z),
		}

		column, found := view.chunks[chunkKey]
		if !found {
			continue
		}

		if column.setBlockState(b.Position.X-chunkKey.X*chunkWidth, b.Position.Y, b.Position.Z-chunkKey.Z*chunkWidth, b.StateID) {
			column.version++
			view.chunks[chunkKey] = column
		}

	}

	v.views[profileID] = view

	return true
}

func (v *WorldViews) BlockState(profileID ProfileID, position BlockPosition) (uint32, bool) {
	view, ok := v.views[profileID]
	if !ok || !view.HasActiveDimensionType || position.Y < view.ActiveDimensionType.MinY || position.Y >= view.ActiveDimensionType.MinY+view.ActiveDimensionType.Height {
		return 0, false
	}
	chunk := ChunkPosition{X: floorChunk(position.X), Z: floorChunk(position.Z)}
	column, found := view.chunks[chunk]
	if !found {
		return 0, false
	}
	return column.blockState(position.X-chunk.X*chunkWidth, position.Y, position.Z-chunk.Z*chunkWidth)
}

func (v *WorldViews) ChunkVersion(profileID ProfileID, position ChunkPosition) (uint64, bool) {
	view, ok := v.views[profileID]
	if !ok {
		return 0, false
	}
	column, ok := view.chunks[position]
	return column.version, ok
}

func (v *WorldView) GetChunks() map[ChunkPosition]ChunkColumn {
	_c := v.chunks
	if _c == nil {
		return nil
	}

	out := make(map[ChunkPosition]ChunkColumn, len(_c))
	maps.Copy(out, _c)

	return out
}

func (v *WorldView) ChunksCount() int {
	return len(v.chunks)
}

func (v *WorldView) clearChunks() {
	v.chunks = nil
}

func floorChunk(value int32) int32 {
	if value >= 0 {
		return value / chunkWidth
	}
	return (value - (chunkWidth - 1)) / chunkWidth
}

func (v WorldView) dimensionType(registryID int32) (DimensionType, bool) {
	if registryID < 0 || int(registryID) >= len(v.DimensionTypes) {
		return DimensionType{}, false
	}
	candidate := v.DimensionTypes[registryID]
	if candidate.RegistryID != registryID {
		return DimensionType{}, false
	}
	return candidate, true
}

type Entity struct {
	ID       int32
	Name     string
	Position Position
	Yaw      float32
	Pitch    float32
}

type EntityViews struct {
	entities map[ProfileID]map[int32]Entity
}

func (v *EntityViews) AddEntities(profileID ProfileID, additions []Entity) {
	if v.entities == nil {
		v.entities = make(map[ProfileID]map[int32]Entity)
	}

	m, ok := v.entities[profileID]
	if !ok {
		m = make(map[int32]Entity)
		v.entities[profileID] = m
	}

	for _, e := range additions {
		m[e.ID] = e
	}
}

func (v *EntityViews) RemoveEntities(profileID ProfileID, ids []int32) {
	m, ok := v.entities[profileID]
	if !ok {
		return
	}

	for _, id := range ids {
		delete(m, id)
	}
}

func (v *EntityViews) MoveEntities(profileID ProfileID, moves []Entity) {
	m, ok := v.entities[profileID]
	if !ok {
		return
	}

	for _, e := range moves {
		if existing, exists := m[e.ID]; exists {
			existing.Position = e.Position
			existing.Yaw = e.Yaw
			existing.Pitch = e.Pitch
			m[e.ID] = existing
		}
	}
}

func (v *EntityViews) GetNearby(profileID ProfileID, pos Position, radius float64) []Entity {
	m, ok := v.entities[profileID]
	if !ok {
		return nil
	}

	result := make([]Entity, 0)
	for _, e := range m {
		dx := e.Position.X - pos.X
		dy := e.Position.Y - pos.Y
		dz := e.Position.Z - pos.Z
		if dx*dx+dy*dy+dz*dz <= radius*radius {
			result = append(result, e)
		}
	}

	return result
}

func (v *EntityViews) GetAll(profileID ProfileID) map[int32]Entity {
	m, ok := v.entities[profileID]
	if !ok {
		return nil
	}

	out := make(map[int32]Entity, len(m))
	maps.Copy(out, m)

	return out
}

type PerceivedEntity struct {
	ID       int32
	Name     string
	Position Position
	Distance float64
	Angle    float64
}

type PerceptionView struct {
	entities map[ProfileID][]PerceivedEntity
}

func (v *PerceptionView) Set(profileID ProfileID, ents []PerceivedEntity) {
	if v.entities == nil {
		v.entities = make(map[ProfileID][]PerceivedEntity)
	}

	stored := make([]PerceivedEntity, len(ents))
	copy(stored, ents)
	v.entities[profileID] = stored
}

func (v *PerceptionView) Get(profileID ProfileID) []PerceivedEntity {
	_s := v.entities[profileID]
	if _s == nil {
		return nil
	}

	out := make([]PerceivedEntity, len(_s))
	copy(out, _s)

	return out
}

// ActionOutcomeStatus is the terminal status reported by the controller for a
// correlated action. It mirrors the host wire status without importing the
// transport-generated enum into the engine model.
type ActionOutcomeStatus uint8

const (
	ActionOutcomeUnspecified ActionOutcomeStatus = iota
	ActionOutcomeCompleted
	ActionOutcomeFailed
)

// ActionOutcome is one controller result. Detail is the existing host wire
// detail field; the current protocol has no separate reason field.
type ActionOutcome struct {
	ControllerSequence uint64
	Action             ControllerAction
	Status             ActionOutcomeStatus
	Detail             string
}

type RealityState struct {
	ArrivalDistance          *float64
	DiggingBlock             *BlockPosition
	AttackingEntity          *int32
	EquippedItem             *string
	GotoTarget               *BlockPosition
	ActionOutcomes           []ActionOutcome
	ActionFailed             bool
	Failure                  string
	ActionFailureCorrelation uint64
}

type RealityView struct {
	states map[ProfileID]RealityState
}

func (v *RealityView) Set(profileID ProfileID, s RealityState) {
	if v.states == nil {
		v.states = make(map[ProfileID]RealityState)
	}
	v.states[profileID] = s
}

func (v *RealityView) Get(profileID ProfileID) (RealityState, bool) {
	s, ok := v.states[profileID]
	return s, ok
}

// Clear removes feedback belonging to a previous host session.
func (v *RealityView) Clear(profileID ProfileID) {
	delete(v.states, profileID)
}

type PerceptionBlock struct {
	Position    BlockPosition
	Name        string
	Distance    float64
	Angle       float64
	VisibleFace BlockPosition
}

type PerceptionBlockView struct {
	blocks map[ProfileID][]PerceptionBlock
}

func (v *PerceptionBlockView) Set(profileID ProfileID, blocks []PerceptionBlock) {
	if v.blocks == nil {
		v.blocks = make(map[ProfileID][]PerceptionBlock)
	}
	stored := make([]PerceptionBlock, len(blocks))
	copy(stored, blocks)
	v.blocks[profileID] = stored
}

func (v *PerceptionBlockView) Get(profileID ProfileID) []PerceptionBlock {
	_s := v.blocks[profileID]
	if _s == nil {
		return nil
	}
	out := make([]PerceptionBlock, len(_s))
	copy(out, _s)
	return out
}
