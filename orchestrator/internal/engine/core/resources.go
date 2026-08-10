package core

import "minecraft_orchestrator/internal/engine/model"

type Resources struct {
	worldViews          model.WorldViews
	entityViews         model.EntityViews
	perceptionView      model.PerceptionView
	realityView         model.RealityView
	perceptionBlockView model.PerceptionBlockView
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

func (r *Resources) RealityView() *model.RealityView {
	return &r.realityView
}

func (r *Resources) PerceptionBlockView() *model.PerceptionBlockView {
	return &r.perceptionBlockView
}
