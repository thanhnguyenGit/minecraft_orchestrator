package bus

import (
	"context"
	"fmt"
)

const eventStream = "mc:events"

func CommandStream(botID string) string {
	return fmt.Sprintf("mc:bot:%s:commands", botID)
}

func EventStream() string {
	return eventStream
}

type EventHandler[T any] interface {
	HandleEvent(event T) error
}

func ConsumeEvent[T any](ctx context.Context, events <-chan T, handler EventHandler[T]) error {
	select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-events:
			if !ok {
				return nil
			}

			if err := handler.HandleEvent(event); err != nil {
				return fmt.Errorf("handler failed: %w", err)
			}
	}
}