package app

import (
	"context"
	"errors"

	"github.com/syudead/vv/internal/domain"
)

// A selection fill lists its targets for the owner to search, preview and apply one by one
// (docs/design-docs/link-fetch.md#a-selection-fills-its-empty-fields-after-a-review).

// WorkInfoFillTargets reads the videos a fill runs for; implemented by store.OverrideStore.
type WorkInfoFillTargets interface {
	// WorkInfoFillTargets returns the first video of each user-data key, in request order, with the key's code.
	WorkInfoFillTargets(ctx context.Context, videoIDs []int64) ([]domain.WorkInfoFillTarget, error)
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
