package bus_test

import (
	"testing"

	"minecraft_orchestrator/internal/bus"
)

func TestStreamNamesAreDerivedFromBotID(t *testing.T) {
	if got, want := bus.CommandStream("king_crimson"), "mc:bot:king_crimson:commands"; got != want {
		t.Fatalf("CommandStream() = %q, want %q", got, want)
	}

	if got, want := bus.EventStream(), "mc:events"; got != want {
		t.Fatalf("EventStream() = %q, want %q", got, want)
	}
}
