package core

import "testing"

func TestBuildSchedulerRunsNetworkApplyBeforeSimulation(t *testing.T) {
	plan, err := BuildScheduler()
	if err != nil {
		t.Fatalf("BuildScheduler() error = %v", err)
	}

	networkNode := -1
	movementNode := -1
	for nodeIndex, node := range plan.Nodes {
		for _, system := range node.Systems {
			switch system.System.ID() {
			case SystemNetworkApply:
				networkNode = nodeIndex
			case SystemMovement:
				movementNode = nodeIndex
			}
		}
	}

	if networkNode < 0 {
		t.Fatal("NetworkApplySystem is not registered")
	}
	if movementNode < 0 {
		t.Fatal("MovementSystem is not registered")
	}
	if networkNode >= movementNode {
		t.Fatalf("NetworkApply node %d must precede Movement node %d", networkNode, movementNode)
	}
}
