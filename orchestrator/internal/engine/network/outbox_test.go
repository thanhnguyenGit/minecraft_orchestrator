package network

import (
	"reflect"
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestOutboxDrainReturnsSessionIntentsInOrder(t *testing.T) {
	outbox := NewOutbox()
	profileID := model.ProfileID{0x01}
	outbox.Publish(Intent{ProfileID: profileID, Kind: IntentStartHost})
	outbox.Publish(Intent{ProfileID: profileID, Kind: IntentStopHost})

	got := outbox.Drain()
	if len(got) != 2 || got[0].Kind != IntentStartHost || got[1].Kind != IntentStopHost {
		t.Fatalf("Drain() = %#v, want ordered start/stop intents", got)
	}
}

func TestEventContainsOnlyHostObservationFields(t *testing.T) {
	typeOfEvent := reflect.TypeFor[Event]()
	for _, legacy := range []string{"AttemptID", "PlayerEntityID", "Correction", "Patch"} {
		if _, found := typeOfEvent.FieldByName(legacy); found {
			t.Fatalf("Event still exposes direct-client field %s", legacy)
		}
	}
}
