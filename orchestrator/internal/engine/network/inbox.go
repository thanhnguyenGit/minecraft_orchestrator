package network

import "sync"

// Inbox is a thread-safe transport mailbox. It preserves event arrival order
// but intentionally contains no Minecraft replica state or ECS World access.
type Inbox struct {
	mu     sync.Mutex
	events []Event
}

func NewInbox() *Inbox {
	return &Inbox{}
}

func (i *Inbox) Publish(event Event) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.events = append(i.events, event)
}

func (i *Inbox) Drain() Batch {
	i.mu.Lock()
	defer i.mu.Unlock()

	events := i.events
	i.events = nil
	return Batch{Events: events}
}
