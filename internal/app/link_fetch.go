package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/syudead/vv/internal/domain"
)

// A virtual video created from a link gets its empty fields filled by the plugins that handle the link, in the
// owner's link order, one fetch at a time behind the request that created it (docs/design-docs/link-fetch.md).

// linkFetchQueueSize bounds the fetches waiting behind the running one; a fetch past it fails at once as busy.
const linkFetchQueueSize = 64

// LinkFetchStore records how a virtual video's link fetch ended; implemented by store.VirtualVideoStore.
type LinkFetchStore interface {
	// FinishLinkFetch writes the outcome only while the fetch is running and the video exists; empty failure is done.
	FinishLinkFetch(ctx context.Context, key, plugin, failure string) error
}

type linkFetchJob struct {
	video domain.Video
	link  string
}

type linkFetches struct {
	store  LinkFetchStore
	logger *slog.Logger
	queue  chan linkFetchJob
	stop   context.CancelFunc
	done   chan struct{}
}

// StartLinkFetches starts the worker that runs the queued fetches until Close or the end of ctx; without it,
// StartLinkFetch does nothing. Call it once.
func (p *WorkInfoPlugins) StartLinkFetches(ctx context.Context, store LinkFetchStore, logger *slog.Logger) {
	ctx, stop := context.WithCancel(ctx)
	f := &linkFetches{
		store: store, logger: logger, queue: make(chan linkFetchJob, linkFetchQueueSize), stop: stop,
		done: make(chan struct{}),
	}
	p.links = f
	go func() {
		defer close(f.done)
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-f.queue:
				p.runLinkFetch(ctx, job)
			}
		}
	}()
}

// Close stops the link fetch worker; a fetch it interrupts stays running until the next start records it as
// interrupted.
func (p *WorkInfoPlugins) Close() {
	if p.links != nil {
		p.links.stop()
		<-p.links.done
	}
}

// StartLinkFetch queues the fill of a virtual video just created from link and returns at once.
func (p *WorkInfoPlugins) StartLinkFetch(video domain.Video, link string) {
	if p.links == nil {
		return
	}
	select {
	case p.links.queue <- linkFetchJob{video: video, link: link}:
	default:
		p.finishLinkFetch(context.Background(), video, "", "Too many links are being read; try Fetch info… later.")
	}
}

func (p *WorkInfoPlugins) runLinkFetch(ctx context.Context, job linkFetchJob) {
	plugin, failure := p.fillFromLink(ctx, job.video, job.link)
	if ctx.Err() != nil {
		return
	}
	p.finishLinkFetch(ctx, job.video, plugin, failure)
}

func (p *WorkInfoPlugins) finishLinkFetch(ctx context.Context, video domain.Video, plugin, failure string) {
	if err := p.links.store.FinishLinkFetch(context.WithoutCancel(ctx), video.ContentKey, plugin, failure); err != nil {
		p.links.logger.Warn("the link fetch's outcome was not saved", "video", video.ID, "error", err)
	}
}

// fillFromLink tries the plugins for link in order until one fetches, and fills the video's empty fields from it;
// it returns the plugin that ran last and, when none succeeded, the first failure.
func (p *WorkInfoPlugins) fillFromLink(ctx context.Context, video domain.Video, link string) (string, string) {
	plugins, err := p.store.ListWorkInfoPlugins(ctx)
	if err != nil {
		return "", "Could not read the plugins."
	}
	candidates := domain.LinkPluginsFor(plugins, link)
	if len(candidates) == 0 {
		return "", "No approved plugin reads links from " + linkHost(link) + "."
	}
	first, name := "", ""
	for _, plugin := range candidates {
		name = plugin.Manifest.Name
		err := p.fillWith(ctx, video, plugin.ID, link)
		if err == nil || errors.Is(err, domain.ErrNotFound) || ctx.Err() != nil {
			// A video deleted or merged since is not the fetch's to report on.
			return name, ""
		}
		if first == "" {
			first = linkFetchFailure(name, err)
		}
	}
	return name, first
}

// fillWith runs one plugin by link and applies its result to the empty fields only, then sets the first cover when
// the video still has no image.
func (p *WorkInfoPlugins) fillWith(ctx context.Context, video domain.Video, pluginID int64, link string) error {
	plugin, result, err := p.run(ctx, pluginID, domain.WorkInfoPluginKindWork,
		domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch, URL: link})
	if recordErr := p.recordFetch(ctx, video.ID, err); recordErr != nil {
		return recordErr
	}
	if err != nil {
		return err
	}
	keepPersonImages(result.WorkInfo.People, result.PersonImages)
	details := domain.OrderFetchedWorkDetails(result.WorkInfo)
	apply := domain.FetchedWorkInfoApply{
		VideoID: video.ID, ContentKey: video.ContentKey, PluginID: plugin.ID, Digest: plugin.Digest,
		Generation: plugin.Generation, Details: details, DetailFields: domain.FetchedDetailFields(details),
		FillEmpty: true,
	}
	if title := domain.KeptWorkInfoTitle(result.WorkInfo.Title); title != "" {
		apply.Title = &title
	}
	apply.Tags, _ = splitTagNames(result.WorkInfo.Tags)
	applied, err := p.applier.ApplyFetchedWorkInfo(ctx, apply)
	if err != nil {
		return err
	}
	p.applyPersonImages(ctx, applied.Video, appliedFaces(details, applied))
	covers, _ := keptCovers(result.WorkInfo.CoverURLs, result.CoverImages)
	p.fillCover(ctx, video, covers)
	return nil
}

// fillCover uploads the first cover only while the video has no image, so an image the owner set during the fetch
// stays.
func (p *WorkInfoPlugins) fillCover(ctx context.Context, video domain.Video, covers []domain.WorkInfoCover) {
	if p.covers == nil || len(covers) == 0 {
		return
	}
	if _, err := p.covers.FillThumbnailImage(ctx, video.ID, video.ContentKey, covers[0].Data); err != nil {
		p.links.logger.Info("the link's image was not applied", "video", video.ID, "error", err)
	}
}

// linkFetchFailure tells the owner why plugin could not read the link.
func linkFetchFailure(plugin string, err error) string {
	var runErr *domain.WorkInfoRunError
	var missing *domain.WorkInfoSettingsMissingError
	switch {
	case errors.As(err, &runErr) && runErr.PluginMessage != "":
		return fmt.Sprintf("%s: %s", plugin, runErr.PluginMessage)
	case errors.As(err, &runErr) && runErr.Host != "":
		return fmt.Sprintf("%s could not read the link (%s, %s).", plugin, runErr.Failure, runErr.Host)
	case errors.As(err, &runErr):
		return fmt.Sprintf("%s could not read the link (%s).", plugin, runErr.Failure)
	case errors.As(err, &missing):
		return plugin + " needs its settings filled in."
	case errors.Is(err, domain.ErrWorkInfoFetchStale), errors.Is(err, domain.ErrWorkInfoPluginNotApproved):
		return plugin + " changed while it read the link."
	}
	return plugin + " could not read the link."
}

func linkHost(link string) string {
	if parsed, err := url.Parse(link); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return "this site"
}
