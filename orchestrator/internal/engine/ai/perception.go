package ai

import (
	"math"

	"minecraft_orchestrator/internal/engine/model"
)

// FOVStrategy defines how a bot determines whether an entity falls within its
// field of view.
// Different implementations can model 2D horizontal arcs, full
// 3D cones, or raycast-verified line-of-sight without changing any consumer.
type FOVStrategy interface {
	IsInFOV(
		botPos model.Position,
		rot model.Rotation,
		fovDeg float32,
		entityPos model.Position,
	) bool
}

// ConeFOV implements a 3D conical field-of-view check using the dot product
// between the bot's forward (look-at) vector and the vector to the entity.
//
// Mineflayer yaw / pitch conventions are radians:
//
//	yaw   0 = north (-Z),  PI/2 = west (-X)
//	pitch 0 = horizon,     PI/2 = straight up
//
// Forward unit vector:
//
//	fx = -sin(yaw) * cos(pitch)
//	fy =  sin(pitch)
//	fz = -cos(yaw) * cos(pitch)
//
// An entity is visible when the angle between forward and (entity − bot) is
// ≤ halfFov, i.e. dot(forward, entityDir) ≥ cos(halfFov). This avoids acos.
type ConeFOV struct{}

// HasClearLineOfSight samples the loaded world between an eye position and an
// exposed resource face. Unknown cells intentionally block sight: selecting a
// target is only safe when the orchestrator has a complete local view.
func HasClearLineOfSight(
	from, to model.Position,
	target model.BlockPosition,
	blockState func(model.BlockPosition) (uint32, bool),
) bool {
	dx := to.X - from.X
	dy := to.Y - from.Y
	dz := to.Z - from.Z
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz) // Euclidean distance in 3D formula
	steps := int(math.Ceil(distance / 0.1))
	if steps == 0 {
		return true
	}

	// Raymarching
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		pos := model.BlockPosition{
			X: int32(math.Floor(from.X + dx*t)),
			Y: int32(math.Floor(from.Y + dy*t)),
			Z: int32(math.Floor(from.Z + dz*t)),
		}

		if pos == target {
			continue
		}

		stateID, known := blockState(pos)
		if !known || stateID != 0 {
			return false
		}
	}
	return true
}

func (ConeFOV) IsInFOV(botPos model.Position, rot model.Rotation, fovDeg float32, entityPos model.Position) bool {
	yawRad := float64(rot.Yaw)
	pitchRad := float64(rot.Pitch)
	cosHalfFov := math.Cos(float64(fovDeg/2) * (math.Pi / 180))

	fx := -math.Sin(yawRad) * math.Cos(pitchRad)
	fy := math.Sin(pitchRad)
	fz := -math.Cos(yawRad) * math.Cos(pitchRad)

	tx := entityPos.X - botPos.X
	ty := entityPos.Y - botPos.Y
	tz := entityPos.Z - botPos.Z

	lenSq := tx*tx + ty*ty + tz*tz
	if lenSq == 0 {
		return true
	}

	invLen := 1.0 / math.Sqrt(lenSq)

	dot := (fx*tx + fy*ty + fz*tz) * invLen

	return dot >= cosHalfFov
}
