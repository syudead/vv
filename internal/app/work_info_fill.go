package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// A fill searches with one plugin by each selected video's code, or its title without one, fetches the candidate
// that matches and fills only the empty fields, as a link fetch does
// (docs/design-docs/link-fetch.md#a-selection-fills-its-empty-fields-by-search).

// workInfoFillQueueSize holds one full selection of the bulk actions' 20,000 videos.
const workInfoFillQueueSize = 20_000

// WorkInfoFillTargets reads the videos a fill runs for; implemented by store.OverrideStore.
type WorkInfoFillTargets interface {
	// WorkInfoFillTargets returns the first video of each user-data key, in request order, with the key's code.
	WorkInfoFillTargets(ctx context.Context, videoIDs []int64) ([]domain.WorkInfoFillTarget, error)
}

type workInfoFillJob struct {
	target   domain.WorkInfoFillTarget
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

// StartWorkInfoFills queues a fill with pluginID for each user-data key among videoIDs and returns how many it
// queued, at once. The plugin must be an approved work-info plugin that searches and fetches and has its required
// settings, or the error run returns for it comes back; a request the queue has no room for returns
// ErrWorkInfoFillQueueFull. Both queue nothing.
func (p *WorkInfoPlugins) StartWorkInfoFills(ctx context.Context, pluginID int64, videoIDs []int64) (int, error) {
	if p.links == nil || p.fillTargets == nil || p.videos == nil {
		return 0, errors.New("work-info fills are not configured")
	}
	plugin, err := p.store.WorkInfoPlugin(ctx, pluginID)
	if err != nil {
		return 0, err
	}
	if plugin.Kind() != domain.WorkInfoPluginKindWork {
		return 0, domain.ErrWorkInfoPluginKindMismatch
	}
	if _, err := runnableSettings(plugin, domain.WorkInfoSearch); err != nil {
		return 0, err
	}
	if !plugin.Manifest.Supports(domain.WorkInfoFetch) {
		return 0, domain.ErrWorkInfoOperationUnsupported
	}
	targets, err := p.fillTargets.WorkInfoFillTargets(ctx, videoIDs)
	if err != nil {
		return 0, err
	}
	jobs := make([]workInfoFillJob, 0, len(targets))
	for _, target := range targets {
		jobs = append(jobs, workInfoFillJob{target: target, pluginID: pluginID})
	}
	if !p.links.fills.push(jobs) {
		return 0, domain.ErrWorkInfoFillQueueFull
	}
	return len(jobs), nil
}

// runWorkInfoFill records a fill that found no match as the video's failed fetch, since nothing else tells the owner;
// a run failure is recorded by the run itself.
func (p *WorkInfoPlugins) runWorkInfoFill(ctx context.Context, job workInfoFillJob) {
	failure, err := p.fillBySearch(ctx, job)
	if failure != "" {
		err = p.applier.RecordWorkInfoFetch(ctx, job.target.VideoID, failure)
	}
	if err != nil && ctx.Err() == nil && !errors.Is(err, domain.ErrNotFound) {
		p.links.logger.Info("a work-info fill did not apply", "video", job.target.VideoID, "plugin", job.pluginID, "error", err)
	}
}

// fillBySearch returns why the fill found nothing to apply, or the error that stopped it.
func (p *WorkInfoPlugins) fillBySearch(ctx context.Context, job workInfoFillJob) (string, error) {
	video := domain.Video{ID: job.target.VideoID, ContentKey: job.target.ContentKey}
	query := job.target.Code
	if query == "" {
		stored, err := p.videos.GetVideo(ctx, domain.AudienceOwner, video.ID)
		if err != nil {
			return "", err
		}
		query = stored.Title
	}
	query, err := domain.NormalizeWorkInfoSearchQuery(query)
	if err != nil {
		return "The search words must be 1 to 200 characters.", nil
	}
	_, result, err := p.run(ctx, job.pluginID, domain.WorkInfoPluginKindWork,
		domain.WorkInfoRunRequest{Operation: domain.WorkInfoSearch, Query: query})
	if err != nil {
		return "", errors.Join(err, p.recordFetch(ctx, video.ID, err))
	}
	candidate, ok := domain.MatchWorkInfoCandidate(result.Candidates, query)
	if !ok {
		return fmt.Sprintf("No search result matched %q.", query), nil
	}
	id, err := domain.NormalizeWorkInfoSearchQuery(candidate.ID)
	if err != nil {
		return "The matched work's ID must be 1 to 200 characters.", nil
	}
	return "", p.fillWith(ctx, video, job.pluginID, domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch, ID: id})
}
