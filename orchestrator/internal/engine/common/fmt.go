package common

import (
	"slices"
	"strings"

	"minecraft_orchestrator/internal/engine/model"
)

func FormatMasks(masks []model.Mask) string {
	copy := append([]model.Mask(nil), masks...)
	slices.Sort(copy)
	parts := make([]string, 0, len(copy))

	for _, m := range copy {
		parts = append(parts, m.String())
	}

	return strings.Join(parts, ", ")
}
