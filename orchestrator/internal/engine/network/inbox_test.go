package network

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestInboxDrainPreservesEventOrder(t *testing.T) {
	inbox := NewInbox()
	profileID := model.ProfileID{0x01}

	inbox.Publish(Event{ProfileID: profileID, AttemptID: 7, Kind: EventConnecting})
	inbox.Publish(Event{ProfileID: profileID, AttemptID: 7, Kind: EventPlayReady, PlayerEntityID: 30011})
	inbox.Publish(Event{ProfileID: profileID, AttemptID: 7, Kind: EventSessionClosed})

	batch := inbox.Drain()
	if len(batch.Events) != 3 {
		t.Fatalf("event count = %d, want 3", len(batch.Events))
	}

	want := []EventKind{EventConnecting, EventPlayReady, EventSessionClosed}
	for index, event := range batch.Events {
		if event.Kind != want[index] {
			t.Fatalf("event %d kind = %v, want %v", index, event.Kind, want[index])
		}
		if event.ProfileID != profileID || event.AttemptID != 7 {
			t.Fatalf("event %d identity = (%x, %d), want (%x, 7)", index, event.ProfileID, event.AttemptID, profileID)
		}
	}

	if got := inbox.Drain(); len(got.Events) != 0 {
		t.Fatalf("second drain returned %d events, want 0", len(got.Events))
	}
}
