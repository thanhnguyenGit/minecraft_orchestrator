package core

import "fmt"

type Entity struct {
	Index uint32
	Generation uint32
}

func (e Entity) String() string {
	return fmt.Sprintf("%d:%d", e.Index, e.Generation)
}

func (e Entity) IsZero() bool {
	return e.Index == 0 && e.Generation == 0
}



