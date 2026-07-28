package runtime

import (
	"context"
	"errors"
	"time"
)

const defaultStep = 50 * time.Millisecond

type Clock interface {
	Now() time.Time
	Wait(context.Context, time.Duration) error
}

type FixedLoop struct {
	Clock Clock
	Step  time.Duration
}

func (l FixedLoop) Run(ctx context.Context, frame func(tick uint64, delta time.Duration) error) error {
	if ctx == nil {
		return errors.New("fixed loop context is required")
	}
	if frame == nil {
		return errors.New("fixed loop frame callback is required")
	}
	if l.Clock == nil {
		l.Clock = wallClock{}
	}
	if l.Step <= 0 {
		l.Step = defaultStep
	}

	nextStart := l.Clock.Now()
	for tick := uint64(0); ; tick++ {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if until := nextStart.Sub(l.Clock.Now()); until > 0 {
			if err := l.Clock.Wait(ctx, until); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}

		actualStart := l.Clock.Now()
		if err := frame(tick, l.Step); err != nil {
			return err
		}
		// Scheduling is anchored to the actual start. If the frame overran, the
		// next iteration runs immediately, but no historical ticks are replayed.
		nextStart = actualStart.Add(l.Step)
	}
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

func (wallClock) Wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
