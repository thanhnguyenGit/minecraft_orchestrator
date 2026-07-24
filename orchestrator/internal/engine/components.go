package engine

import "fmt"

type Component uint8

const (
	CPosition Component = iota
	CVelocity
	CHealth
	CConnection
	CDisconnection
	componentCount
)

var componentNames = [...]string {
	"Position",
	"Velocity",
	"Health",
	"Connection",
	"Disconnection",
}

func (c Component) String() string {
	if c >= componentCount {
		return fmt.Sprintf("Component(%d)", c)
	}

	return componentNames[c]
}