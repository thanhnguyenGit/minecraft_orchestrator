package network

import (
	"sync"

	"minecraft_orchestrator/internal/engine/model"
)

type IntentKind uint8

const (
	IntentStartHost IntentKind = iota
	IntentStopHost
	// Deprecated compatibility names for the inactive direct-client runner.
	IntentStartSession = IntentStartHost
	IntentStopSession  = IntentStopHost
)

type Intent struct {
	ProfileID model.ProfileID
	Kind      IntentKind
}

// Outbox transports ECS lifecycle intent to the runtime. It has no socket or
// World access; the Mineflayer host supervisor consumes drained intents after a frame.
type Outbox struct {
	mu      sync.Mutex
	intents []Intent
}

func NewOutbox() *Outbox {
	return &Outbox{}
}

func (o *Outbox) Publish(intent Intent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.intents = append(o.intents, intent)
}

func (o *Outbox) Drain() []Intent {
	o.mu.Lock()
	defer o.mu.Unlock()
	intents := o.intents
	o.intents = nil
	return intents
}
