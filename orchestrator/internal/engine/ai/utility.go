package ai

import "minecraft_orchestrator/internal/engine/model"

type GoalType = model.GoalType

const (
	Idle            = model.Idle
	Eat             = model.Eat
	CraftTool       = model.CraftTool
	FindFood        = model.FindFood
	Flee            = model.Flee
	Fight           = model.Fight
	GatherResource  = model.GatherResource
	Hunt            = model.Hunt
	ReturnToShelter = model.ReturnToShelter
)

type CurrentGoal struct {
	GoalType GoalType
}

type Capability struct {
	CombatPower  float32
	DefensePower float32
	MiningPower  float32
	HuntingPower float32
}

type DecisionResult struct {
	GoalType GoalType
	Score    float32
}
