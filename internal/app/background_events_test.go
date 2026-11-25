package app

import (
	"testing"
	"time"
)

// Ten job completions within a second reach the screens as one event at once and one trailing event, so the last
// change is never left unsent.
func TestBackgroundEventsSendOnceAndOnceMoreWithinASecond(t *testing.T) {
	clock := newStepClock()
	var sent int
	events := NewBackgroundEvents(func() { sent++ }, clock)

	for range 10 {
		events.Changed()
		clock.Advance(90 * time.Millisecond)
	}
	if sent != 1 {
		t.Fatalf("sent within the second = %d, want 1", sent)
	}
	clock.Advance(BackgroundEventInterval)
	if sent != 2 {
		t.Fatalf("sent after the second = %d, want 2", sent)
	}
	if pending := clock.pending(); pending != 0 {
		t.Fatalf("pending timers = %d, want none", pending)
	}
}
