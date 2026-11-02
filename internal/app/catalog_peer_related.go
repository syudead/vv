package app

import (
	"context"
	"log/slog"
	"slices"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// MergedRelatedVideos is RelatedVideos over this node and peers, in slot order, for a video of this node: each
// link scores its own videos against this video's clues, and the lists merge in the related order. A link that
// fails is skipped for the rest of the list, and one missing from peers on the first page never joins it.
func (c *Catalog) MergedRelatedVideos(ctx context.Context, audience domain.Audience, video domain.Video, page domain.MergedRelatedPage, peers []domain.RelatedPeer) (domain.MergedRelated, error) {
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
	related, err := c.mergeRelated(ctx, audience, ranking.ranked, clues, page, peers)
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

// RemoteRelatedVideos is the related list of a link's video over this node and peers: this node scores its
// videos against the video's clues and each link its own; the video's link leaves out the video and its group.
// The caller reads the video's group from that link.
func (c *Catalog) RemoteRelatedVideos(ctx context.Context, audience domain.Audience, clues domain.RelatedClues, page domain.MergedRelatedPage, peers []domain.RelatedPeer) (domain.MergedRelated, error) {
	ranked, err := c.rankByClues(ctx, audience, clues, 0, nil)
	if err != nil {
		return domain.MergedRelated{}, err
	}
	return c.mergeRelated(ctx, audience, ranked, &clues, page, peers)
}

// PeerRelated is one page of this node's videos, except exclude, scored against the clues the peer sent.
// openID, when not 0, is the open video in this node's ids; its directory adds the folder points.
func (c *Catalog) PeerRelated(ctx context.Context, audience domain.Audience, clues domain.RelatedClues, openID int64, exclude []int64, after *domain.RelatedCursor, limit int) (domain.ScoredRelated, error) {
	ranked, err := c.rankByClues(ctx, audience, clues, openID, exclude)
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

// rankByClues ranks every visible video of this node except exclude against the clues of the open video.
// openID, when not 0, is the open video in this node's ids, shown by the peer; it supplies the directory the
// clues lack, so this node's videos in it score as in this node's own related list.
func (c *Catalog) rankByClues(ctx context.Context, audience domain.Audience, clues domain.RelatedClues, openID int64, exclude []int64) ([]domain.RankedRelated, error) {
	candidates, open, err := c.clueCandidates(ctx, audience, openID)
	if err != nil {
		return nil, err
	}
	ids, err := c.index.RelatedTagIDs(ctx, clues.TagNames)
	if err != nil {
		return nil, err
	}
	reference := clues.Candidate(ids)
	if open != nil {
		reference.VideoID, reference.Path = open.VideoID, open.Path
	}
	left := make(map[int64]struct{}, len(exclude))
	for _, id := range exclude {
		left[id] = struct{}{}
	}
	candidates = slices.DeleteFunc(candidates, func(candidate domain.RelatedCandidate) bool {
		_, out := left[candidate.VideoID]
		return out
	})
	return domain.RankRelated(c.scorer, reference, candidates), nil
}

// clueCandidates reads this node's candidates and, when the audience can see the video openID, that video.
// An id it cannot see reads as no id, so neither the scores nor the candidates tell that the video exists.
func (c *Catalog) clueCandidates(ctx context.Context, audience domain.Audience, openID int64) ([]domain.RelatedCandidate, *domain.RelatedCandidate, error) {
	if openID > 0 {
		candidates, err := c.index.RelatedCandidates(ctx, audience, openID)
		if err != nil {
			return nil, nil, err
		}
		index := slices.IndexFunc(candidates, func(candidate domain.RelatedCandidate) bool { return candidate.VideoID == openID })
		if index >= 0 {
			open := candidates[index]
			return candidates, &open, nil
		}
	}
	// No video has id 0, so no video is taken as the reference or has its versions left out.
	candidates, err := c.index.RelatedCandidates(ctx, audience, 0)
	return candidates, nil, err
}

// mergeRelated pages this node's ranked list and each link's after the cursor and merges them.
func (c *Catalog) mergeRelated(ctx context.Context, audience domain.Audience, ranked []domain.RankedRelated, clues *domain.RelatedClues, page domain.MergedRelatedPage, peers []domain.RelatedPeer) (domain.MergedRelated, error) {
	at := page.Cursor
	local := domain.RelatedSidePage{}
	if !at.Local.Done {
		shown, next := domain.PageRelated(ranked, at.Local.After, page.Limit)
		local = domain.RelatedSidePage{Items: shown, More: next != nil}
	}
	sides := c.linkRelated(ctx, clues, at, peers, page.Limit)
	if err := ctx.Err(); err != nil {
		return domain.MergedRelated{}, err
	}
	ids, next := domain.MergeRelated(append(sides, local), at, page.Limit)
	localIDs := slices.DeleteFunc(slices.Clone(ids), domain.IsRemoteID)
	videos, err := c.index.VideosByIDs(ctx, audience, localIDs)
	if err != nil {
		return domain.MergedRelated{}, err
	}
	return domain.MergedRelated{IDs: ids, Local: videos, NextCursor: next}, nil
}

// linkRelated reads each link's next page concurrently, so one slow link does not delay the others. A link that
// is done, or fails, gives an empty page, which ends its part of the list; without clues no link is asked.
func (c *Catalog) linkRelated(ctx context.Context, clues *domain.RelatedClues, at domain.MergedRelatedCursor, peers []domain.RelatedPeer, limit int) []domain.RelatedSidePage {
	sides := make([]domain.RelatedSidePage, len(peers))
	var wg sync.WaitGroup
	for i, peer := range peers {
		slot := peer.Slot()
		sides[i] = domain.RelatedSidePage{Slot: slot}
		side := at.Link(slot)
		if side.Done || clues == nil {
			continue
		}
		wg.Go(func() {
			page, err := peer.Related(ctx, *clues, side.After, limit)
			if err != nil {
				c.logger.Info("the related videos leave out a linked VVMDM", slog.Int64("slot", slot), slog.Any("error", err))
				return
			}
			page.Slot = slot
			sides[i] = page
		})
	}
	wg.Wait()
	return sides
}
