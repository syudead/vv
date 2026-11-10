package app

import (
	"sync"
	"time"
)

// BackgroundEventInterval is the shortest time between two background events to the screens.
const BackgroundEventInterval = time.Second

// BackgroundEvents throttles the screens' background event: a change sends at once when no event went out within
// the interval, and otherwise one trailing event at the interval's end covers every change before it. A probe
// backlog finishes tens of jobs a second, and each event makes every owner's screen read the status again.
type BackgroundEvents struct {
	send  func()
	clock Clock

	mu    sync.Mutex
	last  time.Time
	timer ClockTimer
}

// NewBackgroundEvents sends through send; a nil clock uses real time.
func NewBackgroundEvents(send func(), clock Clock) *BackgroundEvents {
	if clock == nil {
		clock = systemClock{}
	}
	return &BackgroundEvents{send: send, clock: clock}
}

// Changed reports that the switch, the running work or a count may have changed.
func (e *BackgroundEvents) Changed() {
	e.mu.Lock()
	if e.timer != nil {
		e.mu.Unlock()
		return
	}
	now := e.clock.Now()
	if wait := e.last.Add(BackgroundEventInterval).Sub(now); !e.last.IsZero() && wait > 0 {
		e.timer = e.clock.AfterFunc(wait, e.trail)
		e.mu.Unlock()
		return
	}
	e.last = now
	e.mu.Unlock()
	e.send()
}

func (e *BackgroundEvents) trail() {
	e.mu.Lock()
	e.timer = nil
	e.last = e.clock.Now()
	e.mu.Unlock()
	e.send()
}

// Stop drops a pending trailing event.
func (e *BackgroundEvents) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.timer != nil {
		e.timer.Stop()
		e.timer = nil
	}
}
