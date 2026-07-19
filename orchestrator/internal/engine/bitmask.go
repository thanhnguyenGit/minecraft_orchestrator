package engine

type Mask uint64
type ComponentMask uint8

func (m Mask) Add(compID ComponentMask) Mask {
	return m | (1 << compID)
}

func (m Mask) Remove(compId ComponentMask) Mask {
	return m &^ (1 << compId)
}

func (m Mask) Contains(other Mask) bool {
	return (m & other) == other
}


