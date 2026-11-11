package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/syudead/vv/internal/domain"
)

// BackgroundPauseSettings stores the owner's pause, so a restart or a crash keeps it.
type BackgroundPauseSettings interface {
	SaveBackgroundPaused(ctx context.Context, paused bool) error
}

// BackgroundScheduler is the job scheduler: Hold stops granting and cancels the running jobs without waiting for
// them, and Release grants again.
type BackgroundScheduler interface {
	Hold()
	Release()
}

// PausableWork is background work outside the scheduler that a pause stops after the scheduler is held.
type PausableWork interface {
	PauseBackground(ctx context.Context) error
	ResumeBackground(ctx context.Context) error
}

type BackgroundPauseOptions struct {
	Settings  BackgroundPauseSettings
	Scheduler BackgroundScheduler
	// Nil leaves automatic import running through a pause.
	AutoImport PausableWork
	Scans      PausableWork
	// Nil answers the switch with empty running work and groups.
	Report BackgroundReport
	// Nil disables events.
	Publisher Publisher
	// Paused is the saved state at startup, which the scheduler and the scans were created with.
	Paused bool
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// BackgroundReport is BackgroundStatus: it fills the running work and the groups of a status.
type BackgroundReport interface {
	Read(ctx context.Context, audience domain.Audience, work *domain.BackgroundWork) error
}

// BackgroundPause is the owner's switch over every scan and scheduled job. Requests are applied one at a time, and the
// last one decides the state. See docs/design-docs/background-pause.md.
type BackgroundPause struct {
	settings   BackgroundPauseSettings
	scheduler  BackgroundScheduler
	autoImport PausableWork
	scans      PausableWork
	report     BackgroundReport
	publisher  Publisher
	logger     *slog.Logger

	// apply serializes the requests; paused is read without it, so a status read does not wait for a pause that
	// waits for a scan.
	apply  sync.Mutex
	paused atomic.Bool
}

func NewBackgroundPause(opts BackgroundPauseOptions) *BackgroundPause {
	b := &BackgroundPause{
		settings:   opts.Settings,
		scheduler:  opts.Scheduler,
		autoImport: opts.AutoImport,
		scans:      opts.Scans,
		report:     opts.Report,
		publisher:  opts.Publisher,
		logger:     opts.Logger,
	}
	if b.logger == nil {
		b.logger = slog.Default()
	}
	b.paused.Store(opts.Paused)
	return b
}

// Status answers the switch, the running work and the work left per group as the audience sees them.
func (b *BackgroundPause) Status(ctx context.Context, audience domain.Audience) (domain.BackgroundWork, error) {
	work := domain.BackgroundWork{
		Paused: b.paused.Load(), Running: []domain.BackgroundActivity{}, Groups: domain.EmptyBackgroundGroups(),
	}
	if b.report == nil {
		return work, nil
	}
	if err := b.report.Read(ctx, audience, &work); err != nil {
		return domain.BackgroundWork{}, fmt.Errorf("cannot read the background work status: %w", err)
	}
	return work, nil
}

// SetPaused saves the switch, applies it and answers the status as the audience sees it. A request for the current
// state changes nothing. The request's cancellation does not stop an apply that has begun, so a closed tab never
// leaves the work half paused.
func (b *BackgroundPause) SetPaused(ctx context.Context, audience domain.Audience, paused bool) (domain.BackgroundWork, error) {
	ctx = context.WithoutCancel(ctx)
	b.apply.Lock()
	defer b.apply.Unlock()
	if b.paused.Load() == paused {
		return b.Status(ctx, audience)
	}
	if err := b.settings.SaveBackgroundPaused(ctx, paused); err != nil {
		return domain.BackgroundWork{}, fmt.Errorf("cannot save the background pause: %w", err)
	}
	b.paused.Store(paused)
	if b.publisher != nil {
		b.publisher.Publish(domain.BackgroundChanged{})
	}
	var err error
	if paused {
		err = b.pause(ctx)
	} else {
		err = b.resume(ctx)
	}
	if err != nil {
		return domain.BackgroundWork{}, err
	}
	b.logger.Info("changed the background pause", slog.Bool("paused", paused))
	return b.Status(ctx, audience)
}

// pause holds the scheduler first: stopping automatic import and the scan waits for the file being read, and the
// scheduler must not start jobs during that wait.
func (b *BackgroundPause) pause(ctx context.Context) error {
	b.scheduler.Hold()
	if b.autoImport != nil {
		if err := b.autoImport.PauseBackground(ctx); err != nil {
			return fmt.Errorf("cannot hold automatic import: %w", err)
		}
	}
	if err := b.scans.PauseBackground(ctx); err != nil {
		return fmt.Errorf("cannot pause the scan: %w", err)
	}
	return nil
}

func (b *BackgroundPause) resume(ctx context.Context) error {
	if err := b.scans.ResumeBackground(ctx); err != nil {
		return fmt.Errorf("cannot resume the scan: %w", err)
	}
	if b.autoImport != nil {
		if err := b.autoImport.ResumeBackground(ctx); err != nil {
			return fmt.Errorf("cannot resume automatic import: %w", err)
		}
	}
	b.scheduler.Release()
	return nil
}
