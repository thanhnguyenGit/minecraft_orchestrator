package engine

type Table struct {
	Mask Mask

	Entities []Entity
	Positions []Position
	Vitals []Vital
}

type TableHandler struct {
	Archetype Mask
	Row int
}

