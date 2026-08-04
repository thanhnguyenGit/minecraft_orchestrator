package core

import "minecraft_orchestrator/internal/engine/model"

type Resources struct {
	worldViews model.WorldViews
}

func newResources() Resources {
	return Resources{}
}

func (r *Resources) WorldViews() *model.WorldViews {
	return &r.worldViews
}
