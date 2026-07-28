package runtime

import (
	"errors"
	"testing"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/mc_protocol/client"
)

func TestAdapterMapsSelfPlayerPackets(t *testing.T) {
	profileID := model.ProfileID{0x01}
	adapter := NewAdapter(profileID, 7)

	events, err := adapter.Handle(client.Event{Phase: client.PhasePlay, Message: client.PlayLogin{
		EntityID:  30011,
		SpawnInfo: client.SpawnInfo{GameMode: 0},
	}})
	if err != nil {
		t.Fatalf("PlayLogin Handle() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("PlayLogin events = %#v, want ready and game-mode patch", events)
	}
	if ready := events[0]; ready.Kind != network.EventPlayReady || ready.PlayerEntityID != 30011 {
		t.Fatalf("ready event = %#v", ready)
	}
	if mode := events[1].Patch.GameMode; events[1].Kind != network.EventStatePatch || mode == nil || *mode != model.GameModeSurvival {
		t.Fatalf("game mode event = %#v", events[1])
	}

	events, err = adapter.Handle(client.Event{Phase: client.PhasePlay, Message: client.SynchronizePlayerPosition{
		X: 3, Y: 70, Z: 200, DX: 1, DY: 2, DZ: 3, Yaw: 15, Pitch: 5, Flags: 0x09,
	}})
	if err != nil {
		t.Fatalf("position Handle() error = %v", err)
	}
	if len(events) != 1 || events[0].Kind != network.EventPositionCorrection {
		t.Fatalf("position events = %#v", events)
	}
	correction := events[0].Correction
	if correction.Relative != network.RelativePositionX|network.RelativeYaw {
		t.Fatalf("relative flags = %b, want X and yaw", correction.Relative)
	}
	if correction.Position != (model.Position{X: 3, Y: 70, Z: 200}) || correction.Velocity != (model.Velocity{X: 1, Y: 2, Z: 3}) || correction.Rotation != (model.Rotation{Yaw: 15, Pitch: 5}) {
		t.Fatalf("correction = %#v", correction)
	}

	events, err = adapter.Handle(client.Event{Phase: client.PhasePlay, Message: client.SetHealth{Health: 6.5}})
	if err != nil {
		t.Fatalf("health Handle() error = %v", err)
	}
	if got := events[0].Patch.HealthCurrent; len(events) != 1 || got == nil || *got != 6.5 {
		t.Fatalf("health events = %#v", events)
	}

	events, err = adapter.Handle(client.Event{Phase: client.PhasePlay, Message: client.EntityVelocity{EntityID: 30011, X: 8000, Y: -4000, Z: 0}})
	if err != nil {
		t.Fatalf("velocity Handle() error = %v", err)
	}
	if got := events[0].Patch.Velocity; len(events) != 1 || got == nil || *got != (model.Velocity{X: 1, Y: -0.5}) {
		t.Fatalf("velocity events = %#v", events)
	}

	events, err = adapter.Handle(client.Event{Phase: client.PhasePlay, Message: client.EntityVelocity{EntityID: 30012, X: 8000}})
	if err != nil {
		t.Fatalf("other-entity velocity Handle() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("other-entity velocity events = %#v, want none", events)
	}
}

func TestAdapterRejectsMismatchedLoginProfile(t *testing.T) {
	adapter := NewAdapter(model.ProfileID{0x01}, 7)
	_, err := adapter.Handle(client.Event{Phase: client.PhaseLogin, Message: client.LoginSuccess{UUID: [16]byte{0x02}}})
	if !errors.Is(err, ErrProfileMismatch) {
		t.Fatalf("Handle() error = %v, want ErrProfileMismatch", err)
	}
}
