package app

import (
	"context"
	"errors"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// A selection fill lists its targets for the owner to search and confirm, then fetches each chosen work and fills
// only the empty fields, as a link fetch does
// (docs/design-docs/link-fetch.md#a-selection-fills-its-empty-fields-after-a-review).

// workInfoFillQueueSize holds one full selection of the bulk actions' 20,000 videos.
const workInfoFillQueueSize = 20_000

// WorkInfoFillTargets reads the videos a fill runs for; implemented by store.OverrideStore.
type WorkInfoFillTargets interface {
	// WorkInfoFillTargets returns the first video of each user-data key, in request order, with the key's code.
	WorkInfoFillTargets(ctx context.Context, videoIDs []int64) ([]domain.WorkInfoFillTarget, error)
}

type workInfoFillJob struct {
	item     domain.WorkInfoFillItem
	pluginID int64
}

type workInfoFillQueue struct {
	// mu makes a request's jobs enter the queue all together or not at all.
	mu   sync.Mutex
	jobs chan workInfoFillJob
}

func (q *workInfoFillQueue) push(jobs []workInfoFillJob) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if cap(q.jobs)-len(q.jobs) < len(jobs) {
		return false
	}
	for _, job := range jobs {
		q.jobs <- job
	}
	return true
}

// WorkInfoFillQueries returns the first video of each user-data key among videoIDs, in request order, with its title
// and the words its search uses: the key's code, or the title without one.
func (p *WorkInfoPlugins) WorkInfoFillQueries(ctx context.Context, videoIDs []int64) ([]domain.WorkInfoFillQuery, error) {
	if p.fillTargets == nil || p.videos == nil {
		return nil, errors.New("work-info fills are not configured")
	}
	targets, err := p.fillTargets.WorkInfoFillTargets(ctx, videoIDs)
	if err != nil {
		return nil, err
	}
	out := make([]domain.WorkInfoFillQuery, 0, len(targets))
	for _, target := range targets {
		video, err := p.videos.GetVideo(ctx, domain.AudienceOwner, target.VideoID)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		query := target.Code
		if query == "" {
			query = video.Title
		}
		out = append(out, domain.WorkInfoFillQuery{VideoID: video.ID, Title: video.Title, Query: query})
	}
	return out, nil
}

// StartWorkInfoFills queues a fill of each item's video from its chosen work with pluginID and returns how many it
// queued, at once. A work ID outside 1–200 characters after trimming returns ErrWorkInfoRunInvalid; the plugin must be
// an approved work-info plugin that fetches and has its required settings, or the error run returns for it comes
// back; a request the queue has no room for returns ErrWorkInfoFillQueueFull. Each queues nothing.
func (p *WorkInfoPlugins) StartWorkInfoFills(ctx context.Context, pluginID int64, items []domain.WorkInfoFillItem) (int, error) {
	if p.links == nil || p.videos == nil {
		return 0, errors.New("work-info fills are not configured")
	}
	jobs := make([]workInfoFillJob, 0, len(items))
	for _, item := range items {
		id, err := domain.NormalizeWorkInfoSearchQuery(item.WorkID)
		if err != nil {
			return 0, err
		}
		jobs = append(jobs, workInfoFillJob{item: domain.WorkInfoFillItem{VideoID: item.VideoID, WorkID: id}, pluginID: pluginID})
	}
	plugin, err := p.store.WorkInfoPlugin(ctx, pluginID)
	if err != nil {
		return 0, err
	}
	if plugin.Kind() != domain.WorkInfoPluginKindWork {
		return 0, domain.ErrWorkInfoPluginKindMismatch
	}
	if _, err := runnableSettings(plugin, domain.WorkInfoFetch); err != nil {
		return 0, err
	}
	if !p.links.fills.push(jobs) {
		return 0, domain.ErrWorkInfoFillQueueFull
	}
	return len(jobs), nil
}

// runWorkInfoFill reads the video's content key at run time, so a video deleted since the request is skipped; a run
// failure is recorded as the video's fetch activity by the run itself.
func (p *WorkInfoPlugins) runWorkInfoFill(ctx context.Context, job workInfoFillJob) {
	video, err := p.videos.GetVideo(ctx, domain.AudienceOwner, job.item.VideoID)
	if err == nil {
		err = p.fillWith(ctx, video, job.pluginID, domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch, ID: job.item.WorkID})
	}
	if err != nil && ctx.Err() == nil && !errors.Is(err, domain.ErrNotFound) {
		p.links.logger.Info("a work-info fill did not apply", "video", job.item.VideoID, "plugin", job.pluginID, "error", err)
	}
}
