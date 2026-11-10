package domain

import "fmt"

// ErrJobPaused is the cause a pause of background work cancels a running job with; the job is requeued as a snooze,
// so it keeps its attempts and runs again from the start after the resume. See docs/design-docs/background-pause.md.
var ErrJobPaused = fmt.Errorf("background work is paused: %w", ErrJobSnoozed)

// BackgroundWork is the state of the owner's switch over scans and scheduled jobs.
type BackgroundWork struct {
	Paused bool
}

// BackgroundChanged tells the owner's screens to read the background work status again.
type BackgroundChanged struct{}

func (BackgroundChanged) event() {}
