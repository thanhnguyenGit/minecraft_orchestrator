package network

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestInboxDrainPreservesEventOrder(t *testing.T) {
	inbox := NewInbox()
	profileID := model.ProfileID{0x01}

	inbox.Publish(Event{ProfileID: profileID, Kind: EventHostStatus, RemoteSessionID: "host-a", Sequence: 1})
	inbox.Publish(Event{ProfileID: profileID, Kind: EventHostVitals, RemoteSessionID: "host-a", Sequence: 2})
	inbox.Publish(Event{ProfileID: profileID, Kind: EventHostEffects, RemoteSessionID: "host-a", Sequence: 3})

	batch := inbox.Drain()
	if len(batch.Events) != 3 {
		t.Fatalf("event count = %d, want 3", len(batch.Events))
	}

	want := []EventKind{EventHostStatus, EventHostVitals, EventHostEffects}
	for index, event := range batch.Events {
		if event.Kind != want[index] {
			t.Fatalf("event %d kind = %v, want %v", index, event.Kind, want[index])
		}
		if event.ProfileID != profileID || event.RemoteSessionID != "host-a" || event.Sequence != uint64(index+1) {
			t.Fatalf("event %d identity = (%x, %q, %d)", index, event.ProfileID, event.RemoteSessionID, event.Sequence)
		}
	}

	if got := inbox.Drain(); len(got.Events) != 0 {
		t.Fatalf("second drain returned %d events, want 0", len(got.Events))
	}
}
