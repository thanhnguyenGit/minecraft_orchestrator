package bus_test

import (
	"testing"

	"minecraft_orchestrator/internal/bus"
)

func TestEventStreamName(t *testing.T) {
	if got, want := bus.EventStream(), "mc:events"; got != want {
		t.Fatalf("EventStream() = %q, want %q", got, want)
	}
}
