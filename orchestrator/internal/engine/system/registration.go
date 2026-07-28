package core

import "minecraft_orchestrator/internal/engine/scheduler"

const (
	PhaseInput      scheduler.PhaseID = "input"
	PhaseSimulation scheduler.PhaseID = "simulation"
)

func BuildScheduler() (*scheduler.ExecutionPlan, error) {
	builder := scheduler.NewBuilder()
	if err := builder.AddPhase(PhaseInput); err != nil {
		return nil, err
	}

	if err := builder.AddPhase(PhaseSimulation, PhaseInput); err != nil {
		return nil, err
	}

	if err := builder.AddSystem(PhaseInput, BootstrapSystem{}); err != nil {
		return nil, err
	}

	if err := builder.AddSystem(PhaseInput, NetworkApplySystem{}, scheduler.After(SystemBootstrap)); err != nil {
		return nil, err
	}

	if err := builder.AddSystem(PhaseSimulation, MovementSystem{}); err != nil {
		return nil, err
	}

	if err := builder.AddSystem(PhaseSimulation, DisconnectedCleanUpSystem{}); err != nil {
		return nil, err
	}

	return builder.Compile()
}
