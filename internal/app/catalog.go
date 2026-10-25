package app

import (
	"context"
	"log/slog"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

type CatalogIngestStore interface {
	// Atomically reset missing completed previews to pending and enqueue once; return false if already
	// reset.
	RequeueMissingPreview(ctx context.Context, id int64, contentKey string) (bool, error)
	// Atomically reset missing completed seek previews to pending and enqueue once; return false if
	// already reset.
	RequeueMissingSeekThumbnails(ctx context.Context, id int64, contentKey string) (bool, error)
	RetryProbe(ctx context.Context, id int64) error
}

// Every lookup preserves audience restrictions; guests receive public videos only.
type CatalogIndexStore interface {
	// Include ranking clues for all visible candidates and selfID.
	RelatedCandidates(ctx context.Context, audience domain.Audience, selfID int64) ([]domain.RelatedCandidate, error)
	VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error)
	// Return visible group members in playback order, loading details only within window; contextID
	// zero means no opened group. See docs/design-docs/catalog-organization.md.
	VideoGroup(ctx context.Context, audience domain.Audience, id int64, opened domain.VideoContext, window domain.GroupWindow) (domain.VideoGroup, bool, error)
	// RelatedTagNames returns the names of the tags with ids; an id without a tag is absent.
	RelatedTagNames(ctx context.Context, ids []int64) (map[int64]string, error)
	// RelatedTagIDs resolves names, synonyms included, to tag ids; a name without a tag is absent.
	RelatedTagIDs(ctx context.Context, names []string) (map[string]int64, error)
}

// Hover previews count as available only when manifest and file size agree.
type ArtifactFiles interface {
	PreviewAvailable(contentKey string) bool
	SeekThumbnailsAvailable(contentKey string) bool
}

type CatalogOptions struct {
	Index  CatalogIndexStore
	Ingest CatalogIngestStore
	Files  ArtifactFiles
	// Nil uses domain.DefaultRelatedScorer.
	Scorer domain.RelatedScorer
	// Nil uses slog.Default.
	Logger *slog.Logger
}

type Catalog struct {
	index  CatalogIndexStore
	ingest CatalogIngestStore
	files  ArtifactFiles
	scorer domain.RelatedScorer
	logger *slog.Logger
}

func NewCatalog(opts CatalogOptions) *Catalog {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	scorer := opts.Scorer
	if scorer == nil {
		scorer = domain.DefaultRelatedScorer
	}
	return &Catalog{index: opts.Index, ingest: opts.Ingest, files: opts.Files, scorer: scorer, logger: logger}
}

// PresentVideos preserves order and repairs missing completed previews once through a storage
// transaction; ordinary reads do not write, and terminal failures are not retried.
func (c *Catalog) PresentVideos(ctx context.Context, videos []domain.Video) []domain.VideoView {
	views := make([]domain.VideoView, 0, len(videos))
	for _, video := range videos {
		views = append(views, c.present(ctx, video))
	}
	return views
}

func (c *Catalog) present(ctx context.Context, video domain.Video) domain.VideoView {
	if video.PreviewState != domain.PreviewStateDone {
		return domain.VideoView{Video: video}
	}
	if c.files.PreviewAvailable(video.ContentKey) {
		return domain.VideoView{Video: video, PreviewAvailable: true}
	}
	if video.ContentKey == "" {
		return domain.VideoView{Video: video}
	}
	requeued, err := c.ingest.RequeueMissingPreview(ctx, video.ID, video.ContentKey)
	switch {
	case err != nil:
		// Even if requeueing fails, omit the URL and retry repair on a later read.
		c.logger.Warn("could not queue regeneration of a missing preview",
			slog.Int64("videoId", video.ID), slog.Any("error", err))
	case requeued:
		video.PreviewState = domain.PreviewStatePending
	}
	return domain.VideoView{Video: video}
}

// SeekThumbnailState prefers existing artifacts over a pending state, but a failed rebuild stays failed beside the
// sprite it kept; missing done artifacts appear pending even if repair enqueueing fails, with transactional
// deduplication across readers.
func (c *Catalog) SeekThumbnailState(ctx context.Context, video domain.Video) (domain.SeekThumbnailState, error) {
	if video.SeekThumbnailState == domain.SeekThumbnailFailed {
		return domain.SeekThumbnailFailed, nil
	}
	if c.files.SeekThumbnailsAvailable(video.ContentKey) {
		return domain.SeekThumbnailDone, nil
	}
	if video.SeekThumbnailState != domain.SeekThumbnailDone {
		return video.SeekThumbnailState, nil
	}
	if _, err := c.ingest.RequeueMissingSeekThumbnails(ctx, video.ID, video.ContentKey); err != nil {
		c.logger.Warn("could not queue regeneration of missing seek thumbnails",
			slog.Int64("videoId", video.ID), slog.Any("error", err))
	}
	return domain.SeekThumbnailPending, nil
}

// VideoGroup preserves audience and opened context, loading only the requested detail window; see
// docs/design-docs/catalog-organization.md.
func (c *Catalog) VideoGroup(ctx context.Context, audience domain.Audience, video domain.Video, opened domain.VideoContext, window domain.GroupWindow) (domain.VideoGroup, bool, error) {
	return c.index.VideoGroup(ctx, audience, video.ID, opened, window)
}

// RelatedVideos ranks audience-visible candidates, excluding context-group members before
// pagination; missing self yields no results. Only the first page includes group navigation, while
// ungrouped navigation uses the top related video and leaves previous navigation to the UI.
func (c *Catalog) RelatedVideos(ctx context.Context, audience domain.Audience, video domain.Video, page domain.RelatedPage) (domain.RelatedVideos, error) {
	first := page.After == nil
	ranking, err := c.rankRelated(ctx, audience, video, page.Context(), first)
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	shown, next := domain.PageRelated(ranking.ranked, page.After, page.Limit)
	ids := make([]int64, 0, len(shown))
	for _, item := range shown {
		ids = append(ids, item.VideoID)
	}
	items, err := c.index.VideosByIDs(ctx, audience, ids)
	if err != nil {
		return domain.RelatedVideos{}, err
	}
	related := domain.RelatedVideos{Items: items, NextCursor: next}
	if !first {
		return related, nil
	}
	if ranking.grouped {
		related.PrevID, related.NextID = ranking.group.Neighbors(video.ID)
		related.Group = &ranking.group
	} else if len(ranking.ranked) > 0 {
		related.NextID = ranking.ranked[0].VideoID
	}
	return related, nil
}

// relatedRanking is the open video's context group and its ranked candidates; self is nil when the
// audience cannot see the video.
type relatedRanking struct {
	ranked  []domain.RankedRelated
	self    *domain.RelatedCandidate
	group   domain.VideoGroup
	grouped bool
}

// rankRelated loads the group window only on the first page, since later pages show no group.
func (c *Catalog) rankRelated(ctx context.Context, audience domain.Audience, video domain.Video, opened domain.VideoContext, first bool) (relatedRanking, error) {
	window := domain.GroupWindow{}
	if first {
		window = domain.GroupWindow{Limit: domain.GroupMemberWindow, Around: true}
	}
	var ranking relatedRanking
	var err error
	ranking.group, ranking.grouped, err = c.index.VideoGroup(ctx, audience, video.ID, opened, window)
	if err != nil {
		return relatedRanking{}, err
	}
	members := map[int64]struct{}{}
	if ranking.grouped {
		for _, member := range ranking.group.MemberIDs {
			members[member] = struct{}{}
		}
	}

	candidates, err := c.index.RelatedCandidates(ctx, audience, video.ID)
	if err != nil {
		return relatedRanking{}, err
	}
	selfIndex := slices.IndexFunc(candidates, func(candidate domain.RelatedCandidate) bool {
		return candidate.VideoID == video.ID
	})
	if selfIndex >= 0 {
		self := candidates[selfIndex]
		others := slices.DeleteFunc(slices.Clone(candidates), func(candidate domain.RelatedCandidate) bool {
			_, member := members[candidate.VideoID]
			return member
		})
		ranking.ranked = domain.RankRelated(c.scorer, self, others)
		ranking.self = &self
	}
	return ranking, nil
}

// RetryProbe atomically resets failed state and queues work; nonfailed or missing videos return
// ErrProbeNotFailed or ErrNotFound.
func (c *Catalog) RetryProbe(ctx context.Context, video domain.Video) error {
	return c.ingest.RetryProbe(ctx, video.ID)
}
