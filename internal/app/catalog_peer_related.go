package app

import (
	"context"
	"log/slog"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// MergedRelatedVideos is RelatedVideos over both nodes for a video of this node: the peer scores its own
// videos against this video's clues, and the two lists merge in the related order. A peer that fails is
// skipped for the rest of the list; a nil peer is skipped from the start.
func (c *Catalog) MergedRelatedVideos(ctx context.Context, audience domain.Audience, video domain.Video, page domain.MergedRelatedPage, peer domain.RelatedPeer) (domain.MergedRelated, error) {
	first := page.Cursor.Start()
	ranking, err := c.rankRelated(ctx, audience, video, page.Context(), first)
	if err != nil {
		return domain.MergedRelated{}, err
	}
	var clues *domain.RelatedClues
	if ranking.self != nil {
		names, err := c.index.RelatedTagNames(ctx, ranking.self.TagIDs)
		if err != nil {
			return domain.MergedRelated{}, err
		}
		own := domain.CluesOf(*ranking.self, names)
		clues = &own
	}
	related, err := c.mergeRelated(ctx, audience, ranking.ranked, clues, nil, page, peer)
	if err != nil || !first {
		return related, err
	}
	if ranking.grouped {
		related.PrevID, related.NextID = ranking.group.Neighbors(video.ID)
		related.Group = &ranking.group
	} else if len(related.IDs) > 0 {
		related.NextID = related.IDs[0]
	}
	return related, nil
}

// RemoteRelatedVideos is the related list of a peer video over both nodes: this node scores its videos
// against the video's clues and the peer its own, except the peer ids in exclude. The caller reads the
// video's group from the peer.
func (c *Catalog) RemoteRelatedVideos(ctx context.Context, audience domain.Audience, clues domain.RelatedClues, exclude []int64, page domain.MergedRelatedPage, peer domain.RelatedPeer) (domain.MergedRelated, error) {
	ranked, err := c.rankByClues(ctx, audience, clues, nil)
	if err != nil {
		return domain.MergedRelated{}, err
	}
	return c.mergeRelated(ctx, audience, ranked, &clues, exclude, page, peer)
}

// PeerRelated is one page of this node's videos, except exclude, scored against the clues the peer sent.
func (c *Catalog) PeerRelated(ctx context.Context, audience domain.Audience, clues domain.RelatedClues, exclude []int64, after *domain.RelatedCursor, limit int) (domain.ScoredRelated, error) {
	ranked, err := c.rankByClues(ctx, audience, clues, exclude)
	if err != nil {
		return domain.ScoredRelated{}, err
	}
	shown, next := domain.PageRelated(ranked, after, limit)
	scores := make(map[int64]domain.RelatedScore, len(shown))
	ids := make([]int64, 0, len(shown))
	for _, item := range shown {
		scores[item.VideoID] = item.Score
		ids = append(ids, item.VideoID)
	}
	videos, err := c.index.VideosByIDs(ctx, audience, ids)
	if err != nil {
		return domain.ScoredRelated{}, err
	}
	page := domain.ScoredRelated{Items: make([]domain.ScoredVideo, 0, len(videos)), NextCursor: next}
	for _, video := range videos {
		page.Items = append(page.Items, domain.ScoredVideo{Score: scores[video.ID], Video: video})
	}
	return page, nil
}

// RelatedClues are the clues of the video with id; false when the audience cannot see it.
func (c *Catalog) RelatedClues(ctx context.Context, audience domain.Audience, id int64) (domain.RelatedClues, bool, error) {
	candidates, err := c.index.RelatedCandidates(ctx, audience, id)
	if err != nil {
		return domain.RelatedClues{}, false, err
	}
	index := slices.IndexFunc(candidates, func(candidate domain.RelatedCandidate) bool { return candidate.VideoID == id })
	if index < 0 {
		return domain.RelatedClues{}, false, nil
	}
	names, err := c.index.RelatedTagNames(ctx, candidates[index].TagIDs)
	if err != nil {
		return domain.RelatedClues{}, false, err
	}
	return domain.CluesOf(candidates[index], names), true, nil
}

// rankByClues ranks every visible video of this node except exclude against clues of a video it does not hold.
func (c *Catalog) rankByClues(ctx context.Context, audience domain.Audience, clues domain.RelatedClues, exclude []int64) ([]domain.RankedRelated, error) {
	// No video has id 0, so no video is taken as the reference or has its versions left out.
	candidates, err := c.index.RelatedCandidates(ctx, audience, 0)
	if err != nil {
		return nil, err
	}
	ids, err := c.index.RelatedTagIDs(ctx, clues.TagNames)
	if err != nil {
		return nil, err
	}
	left := make(map[int64]struct{}, len(exclude))
	for _, id := range exclude {
		left[id] = struct{}{}
	}
	candidates = slices.DeleteFunc(candidates, func(candidate domain.RelatedCandidate) bool {
		_, out := left[candidate.VideoID]
		return out
	})
	return domain.RankRelated(c.scorer, clues.Candidate(ids), candidates), nil
}

// mergeRelated pages this node's ranked list and the peer's after the cursor and merges them.
func (c *Catalog) mergeRelated(ctx context.Context, audience domain.Audience, ranked []domain.RankedRelated, clues *domain.RelatedClues, exclude []int64, page domain.MergedRelatedPage, peer domain.RelatedPeer) (domain.MergedRelated, error) {
	at := page.Cursor
	var local domain.RelatedSidePage
	if !at.Local.Done {
		shown, next := domain.PageRelated(ranked, at.Local.After, page.Limit)
		local = domain.RelatedSidePage{Items: shown, More: next != nil}
	}
	var remote domain.RelatedSidePage
	if !at.Peer.Done && (peer == nil || clues == nil) {
		at.Peer.Done = true
	}
	if !at.Peer.Done {
		var err error
		remote, err = peer.Related(ctx, *clues, exclude, at.Peer.After, page.Limit)
		if err != nil {
			if ctx.Err() != nil {
				return domain.MergedRelated{}, ctx.Err()
			}
			// An unpaired node lands here on every request, so this stays out of the default log.
			c.logger.Debug("the related videos of the other VVMDM are skipped", slog.Any("error", err))
			at.Peer.Done, remote = true, domain.RelatedSidePage{}
		}
	}
	ids, next := domain.MergeRelated(local, remote, at, page.Limit)
	localIDs := slices.DeleteFunc(slices.Clone(ids), domain.IsRemoteID)
	videos, err := c.index.VideosByIDs(ctx, audience, localIDs)
	if err != nil {
		return domain.MergedRelated{}, err
	}
	return domain.MergedRelated{IDs: ids, Local: videos, NextCursor: next}, nil
}
