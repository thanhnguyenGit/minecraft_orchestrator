package network

import (
	"fmt"
	"slices"
	"sync"
)

type Inbox struct {
	mu sync.Mutex

	controls     []Message
	controlCap   int
	lastestInput map[uint64]Input
	inputCap     int

	droppedControls uint64
	droppedInputs   uint64
}

func NewInbox(controlCapacity, inputBots int) (*Inbox, error) {
	if controlCapacity <= 0 || inputBots <= 0 {
		return nil, fmt.Errorf("inbox capacity must be positive")
	}

	return &Inbox{
		controlCap:   controlCapacity,
		inputCap:     inputBots,
		lastestInput: make(map[uint64]Input),
	}, nil
}

func (i *Inbox) Push(msg Message) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	switch value := msg.(type) {
	case Input:
		existing, found := i.lastestInput[value.BotID]
		if found && value.Sequence <= existing.Sequence {
			return true
		}

		if !found && len(i.lastestInput) <= int(existing.Sequence) {
			i.droppedInputs++
			return false
		}

		i.lastestInput[value.BotID] = value
		return true
	case Connect, Disconnect:
		if len(i.controls) >= i.controlCap {
			i.droppedControls++
			return false
		}
		i.controls = append(i.controls, msg)
		return true
	default:
		i.droppedControls++
		return false
	}
}

type Batch struct {
	Controls []Message
	Inputs   map[uint64]Input
}

func (i *Inbox) Drain(maxControls int) Batch {
	i.mu.Lock()
	defer i.mu.Unlock()

	if maxControls <= 0 || maxControls > len(i.controls) {
		maxControls = len(i.controls)
	}

	controls := append([]Message(nil), i.controls[:maxControls]...)
	copy(i.controls, i.controls[maxControls:])
	i.controls = i.controls[:len(i.controls)-maxControls]

	ids := make([]uint64, 0, len(i.lastestInput))
	for id := range i.lastestInput {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	inputs := make(map[uint64]Input, len(ids))
	for _, id := range ids {
		inputs[id] = i.lastestInput[id]
		delete(i.lastestInput, id)
	}

	return Batch{
		Controls: controls,
		Inputs:   inputs,
	}
}

func (i *Inbox) Dropped() (controls, inputs uint64) {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.droppedControls, i.droppedInputs
}

