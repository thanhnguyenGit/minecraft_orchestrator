package scheduler

import (
	"fmt"
	"strings"

	"minecraft_orchestrator/internal/engine/model"
)

type NodeKind uint8

const (
	NodeWave NodeKind = iota
	NodeSync
)

type CompiledSystem struct {
	System System
	Order  int
}

type PlanNode struct {
	Kind       NodeKind
	Phase      PhaseID
	Wave       int
	Systems    []CompiledSystem
	Reason     string
	DirtyAfter []model.Mask
}

type ExecutionPlan struct {
	Nodes []PlanNode
}

func (e *ExecutionPlan) String() string {
	var builder strings.Builder
	var current PhaseID

	for _, node := range e.Nodes {
		if node.Phase != current {
			current = node.Phase
			fmt.Fprintf(&builder, "%s\n", strings.ToUpper(string(current)))
		}

		switch node.Kind {
		case NodeWave:
			fmt.Fprintf(&builder, "  wave %d: ", node.Wave)
			for i, system := range node.Systems {
				if i > 0 {
					builder.WriteString(", ")
				}

				builder.WriteString(string(system.System.ID()))
			}

			builder.WriteByte('\n')
		case NodeSync:
			fmt.Fprintf(&builder, " sync: %s\n", node.Reason)
		}
	}

	return builder.String()
}
