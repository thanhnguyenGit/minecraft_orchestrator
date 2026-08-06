package ai

import (
	"math"

	"minecraft_orchestrator/internal/engine/model"
)

// FOVStrategy defines how a bot determines whether an entity falls within its
// field of view. Different implementations can model 2D horizontal arcs, full
// 3D cones, or raycast-verified line-of-sight without changing any consumer.
type FOVStrategy interface {
	IsInFOV(botPos model.Position, rot model.Rotation, fovDeg float32, entityPos model.Position) bool
}

// ConeFOV implements a 3D conical field-of-view check using the dot product
// between the bot's forward (look-at) vector and the vector to the entity.
//
// Minecraft yaw / pitch conventions:
//
//	yaw   0° = south (+Z),  90° = west (-X)
//	pitch 0° = horizon,    -90° = straight up
//
// Forward unit vector:
//
//	fx = -sin(yaw) * cos(pitch)
//	fy = -sin(pitch)
//	fz =  cos(yaw) * cos(pitch)
//
// An entity is visible when the angle between forward and (entity − bot) is
// ≤ halfFov, i.e. dot(forward, entityDir) ≥ cos(halfFov). This avoids acos.
type ConeFOV struct{}

func (ConeFOV) IsInFOV(botPos model.Position, rot model.Rotation, fovDeg float32, entityPos model.Position) bool {
	yawRad := float64(rot.Yaw) * (math.Pi / 180)
	pitchRad := float64(rot.Pitch) * (math.Pi / 180)
	cosHalfFov := math.Cos(float64(fovDeg/2) * (math.Pi / 180))

	fx := -math.Sin(yawRad) * math.Cos(pitchRad)
	fy := -math.Sin(pitchRad)
	fz := math.Cos(yawRad) * math.Cos(pitchRad)

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
