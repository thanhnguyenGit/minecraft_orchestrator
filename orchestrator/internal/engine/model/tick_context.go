package model

// TickContextInventoryCapacity is the largest inventory snapshot a behavior
// can inspect during one tick. Excess slots are intentionally ignored rather
// than retaining an ECS-owned slice.
const TickContextInventoryCapacity = 64

// TickContextEntityCapacity and TickContextBlockCapacity bound perception in a
// behavior snapshot. The producer selects the nearest/relevant observations
// when a view exceeds these limits.
const (
	TickContextEntityCapacity  = 64
	TickContextBlockCapacity   = 128
	TickContextOutcomeCapacity = 16
)

// TickInventorySlot is a value-only inventory entry. HasItem distinguishes an
// empty slot from an item whose zero values happen to be meaningful.
type TickInventorySlot struct {
	Slot    int32
	Item    ItemStack
	HasItem bool
}

// TickInventory is an immutable-by-construction inventory view for one tick.
// It stores no pointers or slices from the ECS component.
type TickInventory struct {
	SelectedHotbarSlot int32
	Slots              [TickContextInventoryCapacity]TickInventorySlot
	Count              uint8
}

// ToInventory returns an independent compatibility value for legacy scoring
// and planning helpers. Changing the returned inventory cannot change the
// tick context.
func (i TickInventory) ToInventory() Inventory {
	count := int(i.Count)
	if count > len(i.Slots) {
		count = len(i.Slots)
	}

	out := Inventory{SelectedHotbarSlot: i.SelectedHotbarSlot, Slots: make([]InventorySlot, 0, count)}
	for _, slot := range i.Slots[:count] {
		outSlot := InventorySlot{Slot: slot.Slot}
		if slot.HasItem {
			item := slot.Item
			outSlot.Item = &item
		}
		out.Slots = append(out.Slots, outSlot)
	}
	return out
}

// TickRealityFeedback snapshots host-controller observations without retaining
// pointers from RealityState.
type TickRealityFeedback struct {
	HasArrivalDistance       bool
	ArrivalDistance          float64
	HasDiggingBlock          bool
	DiggingBlock             BlockPosition
	HasAttackingEntity       bool
	AttackingEntity          int32
	HasEquippedItem          bool
	EquippedItem             string
	HasGotoTarget            bool
	GotoTarget               BlockPosition
	ActionOutcomes           [TickContextOutcomeCapacity]ActionOutcome
	ActionOutcomeCount       uint8
	ActionFailed             bool
	Failure                  string
	ActionFailureCorrelation uint64
}

// TickWorldFacts contains the world/precondition information behavior code may
// need without giving it access to a mutable WorldView.
type TickWorldFacts struct {
	HasNavigableWorld bool
	WanderDestination BlockPosition
	HasWanderTarget   bool
	PreconditionsHash uint64
}

// TickContextInput is the ECS-facing input used to create a defensive,
// per-tick behavior snapshot.
type TickContextInput struct {
	Tick      uint64
	Bot       Bot
	Session   Session
	Position  Position
	Rotation  Rotation
	Health    Health
	Hunger    Hunger
	Inventory Inventory
	Entities  []PerceivedEntity
	Blocks    []PerceptionBlock
	Reality   *RealityState
	World     TickWorldFacts
}

// TickContext is a value-only snapshot used by stateless utility-AI
// behaviors. It deliberately contains fixed arrays rather than ECS slices or
// pointers, so a behavior cannot mutate ECS state through its inputs.
type TickContext struct {
	Tick      uint64
	ProfileID ProfileID
	Username  string
	Session   Session
	Position  Position
	Rotation  Rotation
	Health    Health
	Hunger    Hunger
	Inventory TickInventory

	Entities    [TickContextEntityCapacity]PerceivedEntity
	EntityCount uint8
	Blocks      [TickContextBlockCapacity]PerceptionBlock
	BlockCount  uint8

	Reality            TickRealityFeedback
	World              TickWorldFacts
	RecentHostiles     [RecentHostileMemoryCapacity]HostileMemory
	RecentHostileCount uint8
}

// NewTickContext copies every mutable input into a bounded value snapshot.
func NewTickContext(in TickContextInput) TickContext {
	ctx := TickContext{
		Tick:      in.Tick,
		ProfileID: in.Bot.ProfileID,
		Username:  in.Bot.Username,
		Session:   in.Session,
		Position:  in.Position,
		Rotation:  in.Rotation,
		Health:    in.Health,
		Hunger:    in.Hunger,
		World:     in.World,
		Inventory: TickInventory{SelectedHotbarSlot: in.Inventory.SelectedHotbarSlot},
	}

	for index, slot := range in.Inventory.Slots {
		if index >= len(ctx.Inventory.Slots) {
			break
		}
		ctx.Inventory.Slots[index].Slot = slot.Slot
		if slot.Item != nil {
			ctx.Inventory.Slots[index].Item = *slot.Item
			ctx.Inventory.Slots[index].HasItem = true
		}
		ctx.Inventory.Count++
	}
	for index, entity := range in.Entities {
		if index >= len(ctx.Entities) {
			break
		}
		ctx.Entities[index] = entity
		ctx.EntityCount++
	}
	for index, block := range in.Blocks {
		if index >= len(ctx.Blocks) {
			break
		}
		ctx.Blocks[index] = block
		ctx.BlockCount++
	}
	if in.Reality != nil {
		ctx.Reality = snapshotReality(*in.Reality)
	}

	return ctx
}

func snapshotReality(in RealityState) TickRealityFeedback {
	out := TickRealityFeedback{
		ActionFailed:             in.ActionFailed,
		Failure:                  in.Failure,
		ActionFailureCorrelation: in.ActionFailureCorrelation,
	}
	if in.ArrivalDistance != nil {
		out.HasArrivalDistance = true
		out.ArrivalDistance = *in.ArrivalDistance
	}
	if in.DiggingBlock != nil {
		out.HasDiggingBlock = true
		out.DiggingBlock = *in.DiggingBlock
	}
	if in.AttackingEntity != nil {
		out.HasAttackingEntity = true
		out.AttackingEntity = *in.AttackingEntity
	}
	if in.EquippedItem != nil {
		out.HasEquippedItem = true
		out.EquippedItem = *in.EquippedItem
	}
	if in.GotoTarget != nil {
		out.HasGotoTarget = true
		out.GotoTarget = *in.GotoTarget
	}
	for index, outcome := range in.ActionOutcomes {
		if index >= len(out.ActionOutcomes) {
			break
		}
		out.ActionOutcomes[index] = outcome
		out.ActionOutcomeCount++
	}
	return out
}
