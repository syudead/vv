package app

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// pauseSteps records every step a pause or a resume takes, in order, across the fakes it stands in for.
type pauseSteps struct {
	mu    sync.Mutex
	steps []string
	held  bool
	saved bool
}

func (p *pauseSteps) record(step string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.steps = append(p.steps, step)
}

func (p *pauseSteps) recorded() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.steps)
}

func (p *pauseSteps) SaveBackgroundPaused(_ context.Context, paused bool) error {
	p.mu.Lock()
	p.saved = paused
	p.mu.Unlock()
	if paused {
		p.record("save paused")
	} else {
		p.record("save resumed")
	}
	return nil
}

func (p *pauseSteps) Hold() {
	p.mu.Lock()
	p.held = true
	p.mu.Unlock()
	p.record("hold scheduler")
}

func (p *pauseSteps) Release() {
	p.mu.Lock()
	p.held = false
	p.mu.Unlock()
	p.record("release scheduler")
}

// pausableStep is automatic import or the scans, recording into the shared steps.
type pausableStep struct {
	name  string
	steps *pauseSteps
}

func (w pausableStep) PauseBackground(context.Context) error {
	w.steps.record("pause " + w.name)
	return nil
}

func (w pausableStep) ResumeBackground(context.Context) error {
	w.steps.record("resume " + w.name)
	return nil
}

func newTestBackgroundPause() (*BackgroundPause, *pauseSteps, *fakePublisher) {
	steps := &pauseSteps{}
	publisher := &fakePublisher{}
	pause := NewBackgroundPause(BackgroundPauseOptions{
		Settings: steps, Scheduler: steps, Publisher: publisher,
		AutoImport: pausableStep{name: "auto import", steps: steps},
		Scans:      pausableStep{name: "scans", steps: steps},
		Logger:     discardLogger(),
	})
	return pause, steps, publisher
}

func setPaused(t *testing.T, pause *BackgroundPause, paused bool) domain.BackgroundWork {
	t.Helper()
	status, err := pause.SetPaused(context.Background(), paused)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// A crash after any step must restart paused, and the scheduler must not start jobs while automatic import and the
// scan stop, so the save comes first and the hold second.
func TestBackgroundPauseSavesThenHoldsTheSchedulerFirst(t *testing.T) {
	pause, steps, publisher := newTestBackgroundPause()

	if status := setPaused(t, pause, true); !status.Paused {
		t.Fatalf("status = %+v, want paused", status)
	}
	want := []string{"save paused", "hold scheduler", "pause auto import", "pause scans"}
	if got := steps.recorded(); !slices.Equal(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	if got := publisher.published(); !slices.Equal(got, []domain.Event{domain.BackgroundChanged{}}) {
		t.Fatalf("events = %v, want one BackgroundChanged", got)
	}
}

// Pause, resume and pause in a row end in the last state.
func TestBackgroundPauseLastRequestDecides(t *testing.T) {
	pause, steps, _ := newTestBackgroundPause()

	setPaused(t, pause, true)
	setPaused(t, pause, false)
	status := setPaused(t, pause, true)

	if !status.Paused || !steps.saved || !steps.held {
		t.Fatalf("status = %+v, saved = %v, held = %v; want paused everywhere", status, steps.saved, steps.held)
	}
	resume := []string{"save resumed", "resume scans", "resume auto import", "release scheduler"}
	if got := steps.recorded()[4:8]; !slices.Equal(got, resume) {
		t.Fatalf("resume steps = %v, want %v", got, resume)
	}
}

// A request for the current state answers it without saving, holding or notifying again.
func TestBackgroundPauseRepeatedRequestChangesNothing(t *testing.T) {
	pause, steps, publisher := newTestBackgroundPause()
	setPaused(t, pause, true)
	before, events := len(steps.recorded()), len(publisher.published())

	if status := setPaused(t, pause, true); !status.Paused {
		t.Fatalf("status = %+v, want paused", status)
	}
	if got := steps.recorded()[before:]; len(got) != 0 {
		t.Fatalf("steps of a repeated pause = %v, want none", got)
	}
	if got := len(publisher.published()); got != events {
		t.Fatalf("events after a repeated pause = %d, want %d", got, events)
	}
}
