package app

import (
	"context"
	"errors"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// A code fill fetches each selected video's work by its code with one plugin and fills only the empty fields, as a
// link fetch does (docs/design-docs/link-fetch.md#a-selection-fills-its-empty-fields-by-code).

// codeFillQueueSize holds one full selection of the bulk actions' 20,000 videos.
const codeFillQueueSize = 20_000

// WorkInfoCodes reads the work codes of selected videos; implemented by store.OverrideStore.
type WorkInfoCodes interface {
	// CodedVideos returns the first video of each user-data key with a code, in request order, and the number of
	// user-data keys the videos have.
	CodedVideos(ctx context.Context, videoIDs []int64) ([]domain.CodedVideo, int, error)
}

type codeFillJob struct {
	video    domain.CodedVideo
	pluginID int64
}

type codeFillQueue struct {
	// mu makes a request's jobs enter the queue all together or not at all.
	mu   sync.Mutex
	jobs chan codeFillJob
}

func (q *codeFillQueue) push(jobs []codeFillJob) bool {
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

// StartCodeFills queues a fill by work code with pluginID for each user-data key among videoIDs that has a code,
// and returns at once. The plugin must be an approved work-info plugin that fetches and has its required settings,
// or the error run returns for it comes back; the bundled link plugin returns ErrWorkInfoOperationUnsupported; a request the queue has no room for returns ErrCodeFillQueueFull. Both
// queue nothing.
func (p *WorkInfoPlugins) StartCodeFills(
	ctx context.Context, pluginID int64, videoIDs []int64,
) (domain.CodeFillsQueued, error) {
	if p.links == nil || p.codes == nil {
		return domain.CodeFillsQueued{}, errors.New("work-info fills are not configured")
	}
	plugin, err := p.store.WorkInfoPlugin(ctx, pluginID)
	if err != nil {
		return domain.CodeFillsQueued{}, err
	}
	if plugin.Kind() != domain.WorkInfoPluginKindWork {
		return domain.CodeFillsQueued{}, domain.ErrWorkInfoPluginKindMismatch
	}
	if _, err := runnableSettings(plugin, domain.WorkInfoFetch); err != nil {
		return domain.CodeFillsQueued{}, err
	}
	// The bundled plugin declares fetch but reads links only, so every fill by code would fail.
	if plugin.Manifest.Name == domain.LinkPluginName {
		return domain.CodeFillsQueued{}, domain.ErrWorkInfoOperationUnsupported
	}
	coded, keys, err := p.codes.CodedVideos(ctx, videoIDs)
	if err != nil {
		return domain.CodeFillsQueued{}, err
	}
	jobs := make([]codeFillJob, 0, len(coded))
	for _, video := range coded {
		// A code over the fetch ID's limit cannot be sent, so it counts as no code.
		if code, err := domain.NormalizeWorkInfoSearchQuery(video.Code); err == nil {
			video.Code = code
			jobs = append(jobs, codeFillJob{video: video, pluginID: pluginID})
		}
	}
	if !p.links.codes.push(jobs) {
		return domain.CodeFillsQueued{}, domain.ErrCodeFillQueueFull
	}
	return domain.CodeFillsQueued{Queued: len(jobs), WithoutCode: keys - len(jobs)}, nil
}

// runCodeFill reports a failure only in the log: a run failure is already recorded as the video's fetch activity.
func (p *WorkInfoPlugins) runCodeFill(ctx context.Context, job codeFillJob) {
	video := domain.Video{ID: job.video.VideoID, ContentKey: job.video.ContentKey}
	err := p.fillWith(ctx, video, job.pluginID, domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch, ID: job.video.Code})
	if err != nil && ctx.Err() == nil && !errors.Is(err, domain.ErrNotFound) {
		p.links.logger.Info("a fill by code did not apply", "video", video.ID, "plugin", job.pluginID, "error", err)
	}
}
