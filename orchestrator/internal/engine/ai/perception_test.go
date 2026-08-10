package ai

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestConeFOVUsesMineflayerRadians(t *testing.T) {
	fov := ConeFOV{}
	bot := model.Position{}
	rotation := model.Rotation{Yaw: 0, Pitch: 0}

	if !fov.IsInFOV(bot, rotation, 120, model.Position{Z: -5}) {
		t.Fatal("yaw=0 should face north (-Z) for Mineflayer rotations")
	}
	if fov.IsInFOV(bot, rotation, 120, model.Position{Z: 5}) {
		t.Fatal("yaw=0 should not include south (+Z) in a 120 degree cone")
	}
}

func TestHasClearLineOfSightRejectsOccludingBlock(t *testing.T) {
	target := model.BlockPosition{
		X: 0,
		Y: 64,
		Z: -4,
	}

	blockState := func(pos model.BlockPosition) (uint32, bool) {
		if pos == (model.BlockPosition{
			X: 0,
			Y: 64,
			Z: -2,
		}) {
			return 1, true
		}

		return 0, true
	}

	if HasClearLineOfSight(
		model.Position{
			X: 0.5,
			Y: 65.62,
			Z: 0.5,
		},
		model.Position{
			X: 0.5,
			Y: 64.5,
			Z: -3.501,
		},
		target,
		blockState,
	) {
		t.Fatal("line of sight passed through an opaque block")
	}
}
