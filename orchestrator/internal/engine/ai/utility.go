package ai

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

type CurrentGoal struct {
	GoalType GoalType
}

type Capability struct {
	CombatPower float32
	DefensePower float32
	MiningPower float32
	HuntingPower float32
}

type DecisionResult struct {
	GoalType GoalType
	Score float32
}