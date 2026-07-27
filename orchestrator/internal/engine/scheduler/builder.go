package scheduler

import "fmt"

type phaseDef struct {
	id      PhaseID
	after   []PhaseID
	order   int
	systems []*systemDef
}

type systemDef struct {
	system System
	phase  PhaseID
	after  []SystemID
	before []SystemID
	order  int
}

// Builder define methods for declaring 
// - which phases exists
// - which systems belong to each phase
// - which systems must run before or after others
type Builder struct {
	phases     map[PhaseID]*phaseDef
	phaseOrder []PhaseID
	systems    map[SystemID]*systemDef
	nextOrder  int
}

func NewBuilder() *Builder {
	return &Builder{
		phases:  make(map[PhaseID]*phaseDef),
		systems: make(map[SystemID]*systemDef),
	}
}

func (b *Builder) AddPhase(id PhaseID, after ...PhaseID) error {
	if id == "" {
		return fmt.Errorf("phase id is empty")
	}

	if _, exists := b.phases[id]; exists {
		return fmt.Errorf("phase %q already registered", id)
	}

	b.phases[id] = &phaseDef{
		id:    id,
		after: append([]PhaseID(nil), after...),
		order: len(b.phaseOrder),
	}

	b.phaseOrder = append(b.phaseOrder, id)

	return nil
}

type SystemOption func(*systemDef)

func After(ids ...SystemID) SystemOption {
	return func(def *systemDef) {
		def.after = append(def.after, ids...)
	}
}

func Before(ids ...SystemID) SystemOption {
	return func(def *systemDef) {
		def.before = append(def.before, ids...)
	}
}

func (b *Builder) AddSystem(phase PhaseID, system System, options ...SystemOption) error {
	phaseDefinition := b.phases[phase]
	if phaseDefinition == nil {
		return fmt.Errorf("phase %q is not registerd", phase)
	}

	if system == nil {
		return fmt.Errorf("system is nil")
	}

	id := system.ID()
	if id == "" {
		return fmt.Errorf("system id is empty")
	}

	if _, exists := b.systems[id]; exists {
		return fmt.Errorf("system %q already registered", id)
	}

	if err := system.Access().Validate(); err != nil {
		return fmt.Errorf("system %q access declarartion: %w", id, err)
	}

	systemDef := &systemDef{
		system: system,
		phase:  phase,
		order:  b.nextOrder,
	}

	b.nextOrder++

	for _, option := range options {
		option(systemDef)
	}

	phaseDefinition.systems = append(phaseDefinition.systems, systemDef)

	return nil
}
