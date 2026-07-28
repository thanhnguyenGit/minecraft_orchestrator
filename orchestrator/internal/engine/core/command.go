package core

import (
	"fmt"
	"maps"

	"minecraft_orchestrator/internal/engine/model"
)

type shadowEntity struct {
	alive  bool
	bundle Bundle
}

type shadowState struct {
	entities map[Entity]*shadowEntity
	bots     map[model.ProfileID]Entity
}

func NewShadowState(w *World) *shadowState {
	bots := make(map[model.ProfileID]Entity, len(w.botIndex))
	maps.Copy(bots, w.botIndex)

	return &shadowState{
		entities: make(map[Entity]*shadowEntity),
		bots:     bots,
	}
}

func (s *shadowState) entity(w *World, entity Entity) (*shadowEntity, error) {
	if existing, found := s.entities[entity]; found {
		if !existing.alive {
			return nil, fmt.Errorf("entity %s was already destroyed earlier in batch", entity)
		}

		return existing, nil
	}

	bundle, err := w.bundle(entity)
	if err != nil {
		return nil, err
	}

	state := &shadowEntity{
		alive:  true,
		bundle: bundle,
	}
	s.entities[entity] = state

	return state, nil
}

type validatedCommand interface {
	apply(*World) error
	destinationMask() (model.Mask, bool)
}

type validatedCreate struct {
	bundle Bundle
}

func (v validatedCreate) apply(w *World) error {
	_, err := w.createNow(v.bundle)
	return err
}

func (v validatedCreate) destinationMask() (model.Mask, bool) {
	return v.bundle.Mask, true
}

type validatedDestroy struct {
	entity Entity
}

func (v validatedDestroy) apply(w *World) error {
	return w.destroyNow(v.entity)
}

func (v validatedDestroy) destinationMask() (model.Mask, bool) {
	return 0, false
}

type validatedMigrate struct {
	entity      Entity
	source      model.Mask
	destination Bundle
}

func (v validatedMigrate) apply(w *World) error {
	return w.migrateNow(v.entity, v.source, v.destination)
}

func (v validatedMigrate) destinationMask() (model.Mask, bool) {
	return v.destination.Mask, true
}

type Command interface {
	fmt.Stringer
	DeclaredAffected() []model.Mask
	validate(*World, *shadowState) (validatedCommand, error)
}

type Envelop struct {
	SystemOrder int
	Sequence    uint64
	Command     Command
}

type CommandBuffer struct {
	systemOrder int
	next        uint64
	commands    []Envelop
}

func NewCommandBuffer(systemOrder int) *CommandBuffer {
	return &CommandBuffer{
		systemOrder: systemOrder,
	}
}

func (b *CommandBuffer) Stage(command Command) {
	b.commands = append(b.commands, Envelop{
		SystemOrder: b.systemOrder,
		Sequence:    b.next,
		Command:     command,
	})
	b.next++
}

func (b *CommandBuffer) Len() int {
	return len(b.commands)
}

func (b *CommandBuffer) Envelopes() []Envelop {
	result := make([]Envelop, len(b.commands))
	copy(result, b.commands)
	return result
}

type CreateCommand struct {
	Bundle Bundle
}

func (c CreateCommand) String() string {
	return fmt.Sprintf("create %s", c.Bundle.Mask)
}

func (c CreateCommand) DeclaredAffected() []model.Mask {
	return []model.Mask{
		c.Bundle.Mask,
	}
}

func (c CreateCommand) validate(w *World, shadow *shadowState) (validatedCommand, error) {
	if err := c.Bundle.Validate(); err != nil {
		return nil, fmt.Errorf("validate create: %w", err)
	}

	if c.Bundle.Mask.Has(model.CBot) {
		botData, ok := c.Bundle.Get(model.CBot).(model.Bot)
		if !ok {
			return nil, fmt.Errorf("corrupted bundle: CBot mask set but data is not model.CBot")
		}
		if existing, found := w.botIndex[botData.ProfileID]; found {
			return nil, fmt.Errorf("validate create: bot with profile ID %x already mapped to entity %s", botData.ProfileID, existing)
		}
		if _, exists := shadow.bots[botData.ProfileID]; exists {
			return nil, fmt.Errorf("validate create: bot with profile ID %x already exists in batch", botData.ProfileID)
		}
		shadow.bots[botData.ProfileID] = Entity{} // reserved until commit
	}

	return validatedCreate{
		bundle: c.Bundle,
	}, nil
}

type DestroyCommand struct {
	Entity       Entity
	ExpectedMask model.Mask
}

func (c DestroyCommand) String() string {
	return fmt.Sprintf("destroy %s", c.Entity)
}

func (c DestroyCommand) validate(w *World, shadow *shadowState) (validatedCommand, error) {
	state, err := shadow.entity(w, c.Entity)
	if err != nil {
		return nil, fmt.Errorf("validate destroy: %w", err)
	}

	if c.ExpectedMask != 0 && state.bundle.Mask != c.ExpectedMask {
		return nil, fmt.Errorf("validate destroy: entity %s is in %s, expected %s", c.Entity, state.bundle.Mask, c.ExpectedMask)
	}

	if state.bundle.Mask.Has(model.CBot) {
		botData, ok := state.bundle.Get(model.CBot).(model.Bot)
		if !ok {
			return nil, fmt.Errorf("corrupted bundle: CBot mask set but data is not model.CBot")
		}
		delete(shadow.bots, botData.ProfileID)
	}
	state.alive = false
	return validatedDestroy{
		entity: c.Entity,
	}, nil
}

type DisconnectedCommand struct {
	Entity    Entity
	ClientID  string
	SinceTick uint64
}

func (c DisconnectedCommand) String() string {
	return fmt.Sprintf("disconnect %s", c.Entity)
}

func (c DisconnectedCommand) DeclaredAffected() []model.Mask {
	return []model.Mask{
		model.ConnectedBotMask, model.DisconnectedBotMask,
	}
}

func (c DisconnectedCommand) validate(w *World, shadow *shadowState) (validatedCommand, error) {
	state, err := shadow.entity(w, c.Entity)
	if err != nil {
		return nil, fmt.Errorf("validate disconnect: %w", err)
	}

	if !state.bundle.Mask.Equals(model.ConnectedBotMask) {
		return nil, fmt.Errorf("validate disconnect: entity %s is in %s, expected %s", c.Entity, state.bundle.Mask, model.ConnectedBotMask)
	}

	connectionData, ok := state.bundle.Get(model.CConnection).(model.Connection)
	if !ok {
		return nil, fmt.Errorf("corrupted bundle: CConnection mask set but data is not model.Connection")
	}

	if c.ClientID != "" && connectionData.ClientId != c.ClientID {
		return nil, fmt.Errorf("validate disconnect: client mismatch for %s", c.Entity)
	}

	source := state.bundle.Mask
	state.bundle.Mask &^= model.Bit(model.CConnection)
	state.bundle.Mask |= model.Bit(model.CDisconnected)

	state.bundle.Set(model.CDisconnected, model.Disconnected{
		SinceTick: c.SinceTick,
	})

	return validatedMigrate{
		entity:      c.Entity,
		source:      source,
		destination: state.bundle,
	}, nil
}

type ReconnectedCommand struct {
	Entity   Entity
	ClientID string
}

func (c ReconnectedCommand) String() string {
	return fmt.Sprintf("reconnect %s", c.Entity)
}

func (c ReconnectedCommand) DeclaredAffected() []model.Mask {
	return []model.Mask{
		model.DisconnectedBotMask, model.ConnectedBotMask,
	}
}

func (c ReconnectedCommand) validate(w *World, shadow *shadowState) (validatedCommand, error) {
	if c.ClientID == "" {
		return nil, fmt.Errorf("validate reconnect: client id is empty")
	}

	state, err := shadow.entity(w, c.Entity)
	if err != nil {
		return nil, fmt.Errorf("validate reconnect: %w", err)
	}

	if !state.bundle.Mask.Equals(model.DisconnectedBotMask) {
		return nil, fmt.Errorf("validate reconnect: entity %s is in %s, expected %s", c.Entity, state.bundle.Mask, model.DisconnectedBotMask)
	}

	source := state.bundle.Mask
	state.bundle.Mask &^= model.Bit(model.CDisconnected)
	state.bundle.Mask |= model.Bit(model.CConnection)
	state.bundle.Set(model.CConnection, model.Connection{
		ClientId: c.ClientID,
	})

	return validatedMigrate{
		entity:      c.Entity,
		source:      source,
		destination: state.bundle,
	}, nil
}
