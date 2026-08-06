package core

import "minecraft_orchestrator/internal/engine/model"

type Resources struct {
	worldViews  model.WorldViews
	entityViews model.EntityViews
}

func newResources() Resources {
	return Resources{}
}

func (r *Resources) WorldViews() *model.WorldViews {
	return &r.worldViews
}

func (r *Resources) EntityViews() *model.EntityViews {
	return &r.entityViews
}
