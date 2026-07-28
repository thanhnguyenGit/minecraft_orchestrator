package network

import (
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func TestOutboxDrainReturnsSessionIntentsInOrder(t *testing.T) {
	outbox := NewOutbox()
	profileID := model.ProfileID{0x01}
	outbox.Publish(Intent{ProfileID: profileID, Kind: IntentStartSession})
	outbox.Publish(Intent{ProfileID: profileID, Kind: IntentStopSession})

	got := outbox.Drain()
	if len(got) != 2 || got[0].Kind != IntentStartSession || got[1].Kind != IntentStopSession {
		t.Fatalf("Drain() = %#v, want ordered start/stop intents", got)
	}
}
