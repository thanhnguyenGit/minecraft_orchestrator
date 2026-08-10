package model

import "slices"

// GoalType identifies a utility-AI goal. FindFood, Hunt, and
// ReturnToShelter are placeholders until their behaviors are implemented.
type GoalType uint8

const (
	Idle GoalType = iota
	Eat
	CraftTool
	FindFood
	Flee
	Fight
	GatherResource
	Hunt
	ReturnToShelter
)

// GoalLifecyclePhase describes the state of the currently selected goal.
type GoalLifecyclePhase uint8

const (
	GoalPhaseInactive GoalLifecyclePhase = iota
	GoalPhaseEntering
	GoalPhaseExecuting
	GoalPhaseExiting
	GoalPhaseBlocked
)

// GoalTargetKind tags the concrete value stored in GoalTarget.
type GoalTargetKind uint8

const (
	GoalTargetNone GoalTargetKind = iota
	GoalTargetEntity
	GoalTargetBlock
	GoalTargetItem
	GoalTargetDestination
)

// GoalTarget stores exactly the concrete target described by Kind.
type GoalTarget struct {
	Kind        GoalTargetKind
	EntityID    int32
	Block       BlockPosition
	Item        string
	Destination Position
}

// HostileMemory is a small, behavior-independent record of a recently seen hostile.
type HostileMemory struct {
	EntityID int32
	Position Position
	SeenTick uint64
}

// ControllerAction names an independently correlated controller action.
type ControllerAction uint8

const (
	ControllerActionNone ControllerAction = iota
	ControllerActionGoto
	ControllerActionBreak
	ControllerActionAttack
	ControllerActionCraft
	ControllerActionEquip
	ControllerActionPlace
	ControllerActionConsume
	ControllerActionCount
)

// InFlightOneShot records controller delivery metadata for an asynchronous
// one-shot command. It is deliberately controller-owned: it never gates
// utility selection or sustained controls.
type InFlightOneShot struct {
	Action            ControllerAction
	Correlation       uint64
	Goal              GoalType
	Target            GoalTarget
	PreconditionsHash uint64
	CraftCount        int32
	PlaceItem         string
}

// GoalExitReason records why a goal's previous lifecycle ended.
type GoalExitReason uint8

const (
	GoalExitNone GoalExitReason = iota
	GoalExitCompleted
	GoalExitCancelled
	GoalExitFailed
	GoalExitSessionReset
	GoalExitBlocked
)

const FailedPlanCacheCapacity = 16

// RecentHostileMemoryCapacity bounds hostile memory so UtilityAIState remains
// safe to store by value in ECS columns.
const RecentHostileMemoryCapacity = 16

// FailedPlan is recorded for later reconciliation; it never schedules an automatic retry.
type FailedPlan struct {
	Goal              GoalType
	Action            ControllerAction
	Target            GoalTarget
	Reason            GoalExitReason
	Correlation       uint64
	PreconditionsHash uint64
	CraftCount        int32
	PlaceItem         string
}

// FailedPlanCache uses fixed storage so failure history is explicitly bounded.
type FailedPlanCache struct {
	Entries [FailedPlanCacheCapacity]FailedPlan
	Count   uint8
}

// Len returns the number of valid entries without exposing an invalid count.
func (c FailedPlanCache) Len() int {
	if c.Count > FailedPlanCacheCapacity {
		return FailedPlanCacheCapacity
	}
	return int(c.Count)
}

// Add records a failed plan. Once full, it deterministically evicts the oldest
// entry, retaining the newest FailedPlanCacheCapacity failures.
func (c *FailedPlanCache) Add(plan FailedPlan) {
	count := c.Len()
	if count == FailedPlanCacheCapacity {
		copy(c.Entries[:FailedPlanCacheCapacity-1], c.Entries[1:])
		c.Entries[FailedPlanCacheCapacity-1] = plan
		c.Count = FailedPlanCacheCapacity
		return
	}

	c.Entries[count] = plan
	c.Count = uint8(count + 1)
}

// UtilityAIState owns all lifecycle data used by utility-AI behaviors.
type UtilityAIState struct {
	CurrentGoal        GoalType
	Phase              GoalLifecyclePhase
	Target             GoalTarget
	RecentHostiles     [RecentHostileMemoryCapacity]HostileMemory
	RecentHostileCount uint8
	LastExitReason     GoalExitReason
	FailedPlans        FailedPlanCache
	CompletedPlans     FailedPlanCache
}

// UtilityAI is the ECS component name retained for generated mirrored views.
type UtilityAI = UtilityAIState

type ControllerField uint8

const (
	ControllerFieldGotoTarget ControllerField = iota + 1
	ControllerFieldBreakTarget
	ControllerFieldAttackTarget
	ControllerFieldCraftTarget
	ControllerFieldEquipTarget
	ControllerFieldPlaceTarget
	ControllerFieldConsumeTarget
)

type CraftTarget struct {
	ItemName string
	Count    int32
}

type PlaceTarget struct {
	X, Y, Z, FaceX, FaceY, FaceZ int32
}

// ControllerState represents desired Mineflayer controller targets. Nil values
// retain the existing delta semantics: in a delta, they mean unchanged, and
// ControllerField identifies explicit clears.
type ControllerState struct {
	GotoTarget    *BlockPosition
	BreakTarget   *BlockPosition
	AttackTarget  *int32
	CraftTarget   *CraftTarget
	EquipTarget   *string
	ConsumeTarget *string
	PlaceTarget   *PlaceTarget
}

// Clone returns a controller state with independent target values.
func (s ControllerState) Clone() ControllerState {
	out := s
	if s.GotoTarget != nil {
		value := *s.GotoTarget
		out.GotoTarget = &value
	}
	if s.BreakTarget != nil {
		value := *s.BreakTarget
		out.BreakTarget = &value
	}
	if s.AttackTarget != nil {
		value := *s.AttackTarget
		out.AttackTarget = &value
	}
	if s.CraftTarget != nil {
		value := *s.CraftTarget
		out.CraftTarget = &value
	}
	if s.EquipTarget != nil {
		value := *s.EquipTarget
		out.EquipTarget = &value
	}
	if s.ConsumeTarget != nil {
		value := *s.ConsumeTarget
		out.ConsumeTarget = &value
	}
	if s.PlaceTarget != nil {
		value := *s.PlaceTarget
		out.PlaceTarget = &value
	}
	return out
}

// ControllerSyncState tracks ECS-owned controller intent and delivery state.
type ControllerSyncState struct {
	Desired                 ControllerState
	LastSent                ControllerState
	ControllerSequence      uint64
	ActionSequences         [ControllerActionCount]uint64
	InFlightOneShot         InFlightOneShot
	BreakDiggingObserved    bool
	BreakDiggingTarget      BlockPosition
	BreakDiggingCorrelation uint64
}

// Clone returns independent desired and last-sent controller snapshots.
func (s ControllerSyncState) Clone() ControllerSyncState {
	out := s
	out.Desired = s.Desired.Clone()
	out.LastSent = s.LastSent.Clone()
	return out
}

// ControllerSync is the ECS component name retained for generated mirrored views.
type ControllerSync = ControllerSyncState

// ControllerStateDelta carries changed target values and explicit field clears.
// Nil target pointers in a ControllerState mean unchanged only inside a delta;
// removals are represented by ClearFields so they survive protobuf encoding.
type ControllerStateDelta struct {
	State       ControllerState
	ClearFields []ControllerField
}

func (d *ControllerStateDelta) HasAny() bool {
	return d != nil && (d.State.HasAny() || len(d.ClearFields) > 0)
}

func (d *ControllerStateDelta) Clears(field ControllerField) bool {
	return d != nil && slices.Contains(d.ClearFields, field)
}

func EmptyControllerState() ControllerState {
	return ControllerState{}
}

func (s *ControllerState) HasAny() bool {
	return s.GotoTarget != nil || s.BreakTarget != nil || s.AttackTarget != nil ||
		s.CraftTarget != nil || s.EquipTarget != nil || s.ConsumeTarget != nil || s.PlaceTarget != nil
}

func (s *ControllerState) Equal(other *ControllerState) bool {
	if other == nil {
		return !s.HasAny()
	}
	return blockPosEqual(s.GotoTarget, other.GotoTarget) &&
		blockPosEqual(s.BreakTarget, other.BreakTarget) &&
		int32PtrEqual(s.AttackTarget, other.AttackTarget) &&
		craftTargetEqual(s.CraftTarget, other.CraftTarget) &&
		stringPtrEqual(s.EquipTarget, other.EquipTarget) &&
		stringPtrEqual(s.ConsumeTarget, other.ConsumeTarget) &&
		placeTargetEqual(s.PlaceTarget, other.PlaceTarget)
}

func blockPosEqual(a, b *BlockPosition) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func int32PtrEqual(a, b *int32) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func stringPtrEqual(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func craftTargetEqual(a, b *CraftTarget) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.ItemName == b.ItemName && a.Count == b.Count)
}

func placeTargetEqual(a, b *PlaceTarget) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.X == b.X && a.Y == b.Y && a.Z == b.Z &&
		a.FaceX == b.FaceX && a.FaceY == b.FaceY && a.FaceZ == b.FaceZ)
}

// ValidateControllerState normalizes controller targets according to their
// action compatibility matrix without mutating the input.
func ValidateControllerState(cs ControllerState) ControllerState {
	out := cs
	switch {
	case out.BreakTarget != nil:
		out.AttackTarget, out.CraftTarget, out.PlaceTarget, out.ConsumeTarget = nil, nil, nil, nil
	case out.AttackTarget != nil:
		out.BreakTarget, out.CraftTarget, out.PlaceTarget, out.ConsumeTarget = nil, nil, nil, nil
	case out.CraftTarget != nil:
		out.BreakTarget, out.AttackTarget, out.PlaceTarget, out.ConsumeTarget = nil, nil, nil, nil
	case out.PlaceTarget != nil:
		out.BreakTarget, out.AttackTarget, out.CraftTarget, out.ConsumeTarget = nil, nil, nil, nil
	case out.ConsumeTarget != nil:
		out.BreakTarget, out.AttackTarget, out.CraftTarget, out.PlaceTarget = nil, nil, nil, nil
	}
	return out
}

// DiffControllerState returns only changed targets and explicit clears.
func DiffControllerState(last, cur ControllerState) *ControllerStateDelta {
	delta := &ControllerStateDelta{}
	if !blockPosEqual(last.GotoTarget, cur.GotoTarget) {
		if cur.GotoTarget == nil {
			delta.ClearFields = append(delta.ClearFields, ControllerFieldGotoTarget)
		} else {
			delta.State.GotoTarget = cur.GotoTarget
		}
	}
	if !blockPosEqual(last.BreakTarget, cur.BreakTarget) {
		if cur.BreakTarget == nil {
			delta.ClearFields = append(delta.ClearFields, ControllerFieldBreakTarget)
		} else {
			delta.State.BreakTarget = cur.BreakTarget
		}
	}
	if !int32PtrEqual(last.AttackTarget, cur.AttackTarget) {
		if cur.AttackTarget == nil {
			delta.ClearFields = append(delta.ClearFields, ControllerFieldAttackTarget)
		} else {
			delta.State.AttackTarget = cur.AttackTarget
		}
	}
	if !craftTargetEqual(last.CraftTarget, cur.CraftTarget) {
		if cur.CraftTarget == nil {
			delta.ClearFields = append(delta.ClearFields, ControllerFieldCraftTarget)
		} else {
			delta.State.CraftTarget = cur.CraftTarget
		}
	}
	if !stringPtrEqual(last.EquipTarget, cur.EquipTarget) {
		if cur.EquipTarget == nil {
			delta.ClearFields = append(delta.ClearFields, ControllerFieldEquipTarget)
		} else {
			delta.State.EquipTarget = cur.EquipTarget
		}
	}
	if !stringPtrEqual(last.ConsumeTarget, cur.ConsumeTarget) {
		if cur.ConsumeTarget == nil {
			delta.ClearFields = append(delta.ClearFields, ControllerFieldConsumeTarget)
		} else {
			delta.State.ConsumeTarget = cur.ConsumeTarget
		}
	}
	if !placeTargetEqual(last.PlaceTarget, cur.PlaceTarget) {
		if cur.PlaceTarget == nil {
			delta.ClearFields = append(delta.ClearFields, ControllerFieldPlaceTarget)
		} else {
			delta.State.PlaceTarget = cur.PlaceTarget
		}
	}
	if !delta.HasAny() {
		return nil
	}
	return delta
}
