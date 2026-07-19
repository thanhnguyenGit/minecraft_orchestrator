package engine

type Position struct {
	DX, DY, DZ float64
}



type Vital struct {
	Health, Hunger, Oxygen uint32
}

type LifecycleState int

const (
	Spawn LifecycleState = iota 
	Respawn
	Kicked
	End
	Error
)

type LifeCycle struct {
	
}