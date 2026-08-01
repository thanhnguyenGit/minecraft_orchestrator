package core

import "testing"

func TestBuildSchedulerContainsOnlyTheECSBootstrapAndHostObservationBoundary(t *testing.T) {
	plan, err := BuildScheduler()
	if err != nil {
		t.Fatalf("BuildScheduler() error = %v", err)
	}

	networkNode := -1
	bootstrapNode := -1
	for nodeIndex, node := range plan.Nodes {
		for _, system := range node.Systems {
			switch system.System.ID() {
			case SystemNetworkApply:
				networkNode = nodeIndex
			case SystemBootstrap:
				bootstrapNode = nodeIndex
			}
		}
	}

	if networkNode < 0 {
		t.Fatal("NetworkApplySystem is not registered")
	}
	if bootstrapNode < 0 {
		t.Fatal("BootstrapSystem is not registered")
	}
	if bootstrapNode >= networkNode {
		t.Fatalf("Bootstrap node %d must precede NetworkApply node %d", bootstrapNode, networkNode)
	}
}
