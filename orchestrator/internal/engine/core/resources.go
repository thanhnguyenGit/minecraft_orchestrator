package core

import "minecraft_orchestrator/internal/engine/model"

type Resources struct {
	worldViews     model.WorldViews
	entityViews    model.EntityViews
	perceptionView model.PerceptionView
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

func (r *Resources) PerceptionView() *model.PerceptionView {
	return &r.perceptionView
}
