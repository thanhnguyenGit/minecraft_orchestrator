package runtime

import (
	"context"
	"testing"
	"time"
)

func TestFixedLoopWaitsUntilNextDeadlineWhenFramesAreEarly(t *testing.T) {
	clock := &fakeClock{}
	loop := FixedLoop{Clock: clock, Step: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var starts []time.Time
	frames := 0
	err := loop.Run(ctx, func(tick uint64, delta time.Duration) error {
		starts = append(starts, clock.Now())
		if delta != 50*time.Millisecond {
			t.Fatalf("delta = %s, want 50ms", delta)
		}
		clock.Advance(18 * time.Millisecond)
		frames++
		if frames == 3 {
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantStarts := []time.Time{time.Time{}, time.Time{}.Add(50 * time.Millisecond), time.Time{}.Add(100 * time.Millisecond)}
	if !equalTimes(starts, wantStarts) {
		t.Fatalf("starts = %v, want %v", starts, wantStarts)
	}
	if got, want := clock.waits, []time.Duration{32 * time.Millisecond, 32 * time.Millisecond}; !equalDurations(got, want) {
		t.Fatalf("waits = %v, want %v", got, want)
	}
}

func TestFixedLoopRunsOneNextFrameImmediatelyAfterOverrun(t *testing.T) {
	clock := &fakeClock{}
	loop := FixedLoop{Clock: clock, Step: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var starts []time.Time
	frames := 0
	err := loop.Run(ctx, func(uint64, time.Duration) error {
		starts = append(starts, clock.Now())
		if frames == 0 {
			clock.Advance(72 * time.Millisecond)
		} else {
			cancel()
		}
		frames++
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantStarts := []time.Time{time.Time{}, time.Time{}.Add(72 * time.Millisecond)}
	if !equalTimes(starts, wantStarts) {
		t.Fatalf("starts = %v, want %v", starts, wantStarts)
	}
	if len(clock.waits) != 0 {
		t.Fatalf("waits = %v, want none after overrun", clock.waits)
	}
}

type fakeClock struct {
	now   time.Time
	waits []time.Duration
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Wait(_ context.Context, duration time.Duration) error {
	c.waits = append(c.waits, duration)
	c.now = c.now.Add(duration)
	return nil
}

func (c *fakeClock) Advance(duration time.Duration) { c.now = c.now.Add(duration) }

func equalTimes(got, want []time.Time) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if !got[index].Equal(want[index]) {
			return false
		}
	}
	return true
}

func equalDurations(got, want []time.Duration) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
