package main

import (
	"slices"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/eventbus"
)

type waker interface {
	Wake()
}

// Publishers do not depend on subscribers.
type eventSubscribers struct {
	// Screen maps overrides and bundle changes to video events, and activity changes to owner-only
	// activity events; overrides do not wake workers.
	Screen           func(domain.Event)
	Workers          map[domain.JobKind]waker
	ReleaseArtifacts func(domain.ContentUnreferenced)
	// Seek completion follows thumbnail completion, so available images are ready for automatic
	// tagging; see docs/design-docs/auto-tagging.md.
	ImagesFinished func(videoID int64)
	// Wake on processing or candidate changes; see docs/design-docs/duplicate-judging.md.
	VersionJudge waker
}

type eventSubscriptions struct {
	// Unsubscribe before closing screen connections.
	StopScreen func()
	// Unsubscribe before stopping workers.
	StopWorkers func()
}

// Register subscribers here without changing publishers. Artifact cleanup stays subscribed until
// bus.Close drains pending events to avoid leaking artifacts.
func subscribeEvents(bus *eventbus.Bus, s eventSubscribers) eventSubscriptions {
	var stops eventSubscriptions
	stops.StopScreen = func() {}
	if s.Screen != nil {
		stops.StopScreen = bus.Subscribe("screen notifications", s.Screen)
	}

	var stopWorkers []func()
	for kind, worker := range s.Workers {
		// Workers wake on events instead of polling queues.
		stopWorkers = append(stopWorkers, eventbus.On(bus, "worker wake-up ("+string(kind)+")",
			func(event domain.JobsQueued) {
				if slices.Contains(event.Kinds, kind) {
					worker.Wake()
				}
			}))
	}
	// Thumbnails wait for probe results because extraction position depends on duration.
	if thumbnail, ok := s.Workers[domain.JobThumbnail]; ok {
		stopWorkers = append(stopWorkers, eventbus.On(bus, "thumbnail after probe",
			func(event domain.VideoIngestChanged) {
				if event.Stage == domain.JobProbe {
					thumbnail.Wake()
				}
			}))
	}
	// Wake seek workers when probing, thumbnail completion, or deletion can satisfy
	// domain.ClaimConditionFor; these conditions do not cancel running jobs.
	if seek, ok := s.Workers[domain.JobSeekThumbnail]; ok {
		stopWorkers = append(stopWorkers, eventbus.On(bus, "seek thumbnail after thumbnail",
			func(event domain.VideoIngestChanged) {
				switch event.Stage {
				case domain.JobProbe, domain.JobThumbnail, "":
					seek.Wake()
				}
			}))
	}
	// Fingerprint jobs arrive through JobsQueued when sprites finish; they need no extra wakeup.
	// Automatic tagging also accepts failed seek generation, using any remaining images and metadata.
	if s.ImagesFinished != nil {
		finished := s.ImagesFinished
		stopWorkers = append(stopWorkers, eventbus.On(bus, "auto-tagging after seek thumbnails",
			func(event domain.VideoIngestChanged) {
				if event.Stage == domain.JobSeekThumbnail {
					finished(event.VideoID)
				}
			}))
	}
	if judge := s.VersionJudge; judge != nil {
		stopWorkers = append(stopWorkers,
			eventbus.On(bus, "duplicate judging after jobs", func(domain.ProcessingChanged) { judge.Wake() }),
			eventbus.On(bus, "duplicate judging after candidates", func(domain.VersionCandidatesChanged) { judge.Wake() }))
	}
	stops.StopWorkers = func() {
		for _, stop := range stopWorkers {
			stop()
		}
	}

	if s.ReleaseArtifacts != nil {
		eventbus.On(bus, "artifact release", s.ReleaseArtifacts)
	}
	return stops
}
