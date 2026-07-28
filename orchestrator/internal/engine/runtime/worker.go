package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	"minecraft_orchestrator/internal/mc_protocol/client"
)

const (
	initialRetryDelay = time.Second
	maximumRetryDelay = 30 * time.Second
)

type BotSpec struct {
	ProfileID model.ProfileID
	Username  string
}

type Session interface {
	Start(context.Context) error
	Events() <-chan client.Event
	Wait() error
	Close() error
}

type SessionFactory func(client.Config) (Session, error)
type SleepFunc func(context.Context, time.Duration) error

// Worker owns one bot's sequential session attempts. It has no World access:
// every observed result is published to the network Inbox for ECS to apply.
type Worker struct {
	Bot       BotSpec
	Config    client.Config
	Inbox     *network.Inbox
	Factory   SessionFactory
	Sleep     SleepFunc
	AttemptID uint64
}

func (w Worker) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("worker context is required")
	}
	if w.Bot.ProfileID == (model.ProfileID{}) {
		return errors.New("worker bot profile UUID is required")
	}
	if w.Bot.Username == "" {
		return errors.New("worker bot username is required")
	}
	if w.Inbox == nil {
		return errors.New("worker inbox is required")
	}
	if w.Factory == nil {
		return errors.New("worker session factory is required")
	}
	if w.Sleep == nil {
		w.Sleep = sleep
	}

	delay := initialRetryDelay
	attemptID := w.AttemptID
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		attemptID++
		w.Inbox.Publish(network.Event{
			ProfileID: w.Bot.ProfileID,
			AttemptID: attemptID,
			Kind:      network.EventConnecting,
		})

		cfg := w.Config
		cfg.Username = w.Bot.Username
		session, err := w.Factory(cfg)
		if err != nil {
			if !w.publishClosedAndWait(ctx, attemptID, delay) {
				return nil
			}
			delay = nextRetryDelay(delay)
			continue
		}

		if err := session.Start(ctx); err != nil {
			_ = session.Close()
			if !w.publishClosedAndWait(ctx, attemptID, delay) {
				return nil
			}
			delay = nextRetryDelay(delay)
			continue
		}

		adapter := NewAdapter(w.Bot.ProfileID, attemptID)
		reachedPlay := false
		failed := false
		for event := range session.Events() {
			mapped, err := adapter.Handle(event)
			if err != nil {
				_ = session.Close()
				w.Inbox.Publish(network.Event{
					ProfileID: w.Bot.ProfileID,
					AttemptID: attemptID,
					Kind:      network.EventSessionFailed,
					Failure:   err.Error(),
				})
				failed = true
				break
			}
			for _, mappedEvent := range mapped {
				if mappedEvent.Kind == network.EventPlayReady {
					reachedPlay = true
				}
				w.Inbox.Publish(mappedEvent)
			}
		}
		_ = session.Wait()

		if failed || ctx.Err() != nil {
			return nil
		}
		if reachedPlay {
			delay = initialRetryDelay
		}
		if !w.publishClosedAndWait(ctx, attemptID, delay) {
			return nil
		}
		delay = nextRetryDelay(delay)
	}
}

func (w Worker) publishClosedAndWait(ctx context.Context, attemptID uint64, delay time.Duration) bool {
	w.Inbox.Publish(network.Event{
		ProfileID: w.Bot.ProfileID,
		AttemptID: attemptID,
		Kind:      network.EventSessionClosed,
	})
	return w.Sleep(ctx, delay) == nil
}

func nextRetryDelay(delay time.Duration) time.Duration {
	if delay >= maximumRetryDelay/2 {
		return maximumRetryDelay
	}
	return delay * 2
}

func sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func NewClientSession(cfg client.Config) (Session, error) {
	session, err := client.NewSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("create Minecraft session: %w", err)
	}
	return session, nil
}
